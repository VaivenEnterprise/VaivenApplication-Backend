// Package mongo guarda el contador de uso diario de la IA (spec 014, RF-05 y
// RF-15; plan P-04). Un documento por persona, función y día:
//
//	{ _id, userId, feature, day, count, createdAt, updatedAt }
//
// El límite se aplica de forma atómica con un solo FindOneAndUpdate: dos
// peticiones simultáneas nunca pasan del límite (prueba de concurrencia).
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
)

// UsageCollection es la colección del contador.
const UsageCollection = "ai_usage"

// UsageRetentionSeconds es la vida de un contador: se borra a los 3 días (plan P-04).
const UsageRetentionSeconds = 3 * 24 * 60 * 60

// UsageIndexes declara los índices de ai_usage: único por persona, función y
// día (la base del límite atómico) y TTL sobre createdAt.
func UsageIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(UsageCollection, []drv.IndexModel{
		{
			Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "feature", Value: 1}, {Key: "day", Value: 1}},
			Options: options.Index().SetName("user_feature_day_unique").SetUnique(true),
		},
		{Keys: bson.D{{Key: "createdAt", Value: 1}}, Options: options.Index().SetName("created_ttl").SetExpireAfterSeconds(UsageRetentionSeconds)},
	}...)
}

// UsageRepo es el contador de uso diario.
type UsageRepo struct {
	col *drv.Collection
}

// NewUsageRepo crea el repositorio sobre db.
func NewUsageRepo(db *drv.Database) *UsageRepo {
	return &UsageRepo{col: db.Collection(UsageCollection)}
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *UsageRepo) Indexes() []adaptermongo.IndexSpec { return UsageIndexes() }

// EnsureIndexes crea los índices de forma idempotente.
func (r *UsageRepo) EnsureIndexes(ctx context.Context) error {
	if _, err := r.col.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes())); err != nil {
		return fmt.Errorf("ai_usage: crear índices: %w", err)
	}
	return nil
}

// Take consume una unidad del límite diario de userID para feature en day
// (fecha de calendario YYYY-MM-DD que decide el caso de uso). Devuelve false,
// sin error, si ya se alcanzó limit.
//
// Es atómico: el filtro solo coincide si count < limit. Si ya se llegó al
// límite, el upsert intenta insertar otro documento con la misma clave y el
// índice único lo rechaza; ese error de clave duplicada significa «sin cupo».
func (r *UsageRepo) Take(ctx context.Context, userID, feature, day string, limit int, now time.Time) (bool, error) {
	if limit <= 0 {
		return false, nil
	}
	now = now.UTC()
	filter := bson.D{
		{Key: "userId", Value: userID}, {Key: "feature", Value: feature}, {Key: "day", Value: day},
		{Key: "count", Value: bson.D{{Key: "$lt", Value: limit}}},
	}
	update := bson.D{
		{Key: "$inc", Value: bson.D{{Key: "count", Value: 1}}},
		{Key: "$set", Value: bson.D{{Key: "updatedAt", Value: now}}},
		{Key: "$setOnInsert", Value: bson.D{{Key: "createdAt", Value: now}}},
	}
	err := r.col.FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetUpsert(true)).Err()
	switch {
	case err == nil, errors.Is(err, drv.ErrNoDocuments):
		// Con upsert, ErrNoDocuments solo indica que no había documento previo.
		return true, nil
	case drv.IsDuplicateKeyError(err):
		return false, nil
	default:
		return false, fmt.Errorf("ai_usage: consumir cupo: %w", err)
	}
}
