// Package mongo implementa los repositorios de las recomendaciones (spec 015,
// T003; plan §2) y las lecturas en solo lectura de bookings, items y users
// (plan P-05). Es el único paquete del módulo reco que importa el driver.
//
// Colecciones propias:
//
//	reco_interactions { userId, itemId, type, weight, bucket, demo, createdAt, updatedAt }
//	reco_neighbors        { _id: "<buildId>:<itemId>", buildId, itemId, neighbors[≤20], createdAt, updatedAt }
//	reco_popular_lists      { _id: "<buildId>:<clave>", buildId, key, items[≤50], createdAt, updatedAt }
//	reco_states         { _id: "model", buildId, builtAt, lockedUntil, lockOwner, ... } y { _id: "reset:<userId>", resetAt, ... }
//
// Los errores nunca incluyen identificadores de objetos ni eventos (RF-09):
// solo nombran la operación.
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
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// InteractionsCollection es la colección de eventos de interacción.
const InteractionsCollection = "reco_interactions"

// interactionTTLSeconds es la vida de un evento (90 días, RF-09).
const interactionTTLSeconds = int32(domain.Window / time.Second)

// InteractionIndexes declara los índices de reco_interactions: único por
// persona, objeto, tipo y bloque de 10 min (límite de vistas, RF-01), historial
// por persona (consulta y borrado) y TTL de 90 días.
func InteractionIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(InteractionsCollection,
		drv.IndexModel{
			Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "itemId", Value: 1}, {Key: "type", Value: 1}, {Key: "bucket", Value: 1}},
			Options: options.Index().SetName("user_item_type_bucket_unique").SetUnique(true),
		},
		drv.IndexModel{
			Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("user_createdAt"),
		},
		drv.IndexModel{
			Keys:    bson.D{{Key: "createdAt", Value: 1}},
			Options: options.Index().SetName("created_ttl").SetExpireAfterSeconds(interactionTTLSeconds),
		},
	)
}

type interactionDoc struct {
	UserID    string    `bson:"userId"`
	ItemID    string    `bson:"itemId"`
	Type      string    `bson:"type"`
	Weight    float64   `bson:"weight"`
	Bucket    time.Time `bson:"bucket"`
	Demo      bool      `bson:"demo"`
	CreatedAt time.Time `bson:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"`
}

func toInteractionDoc(e domain.Event, demo bool) interactionDoc {
	at := e.At.UTC()
	return interactionDoc{
		UserID: e.UserID, ItemID: e.ItemID, Type: string(e.Type), Weight: e.Weight,
		Bucket: e.Bucket(), Demo: demo, CreatedAt: at, UpdatedAt: at,
	}
}

func (d interactionDoc) toDomain() domain.Event {
	return domain.Event{UserID: d.UserID, ItemID: d.ItemID, Type: domain.EventType(d.Type), Weight: d.Weight, At: d.CreatedAt.UTC()}
}

// InteractionRepo guarda los eventos de interacción.
type InteractionRepo struct {
	col *drv.Collection
}

// NewInteractionRepo crea el repositorio sobre db.
func NewInteractionRepo(db *drv.Database) *InteractionRepo {
	return &InteractionRepo{col: db.Collection(InteractionsCollection)}
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *InteractionRepo) Indexes() []adaptermongo.IndexSpec { return InteractionIndexes() }

// EnsureIndexes crea los índices de forma idempotente.
func (r *InteractionRepo) EnsureIndexes(ctx context.Context) error {
	if _, err := r.col.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes())); err != nil {
		return fmt.Errorf("reco_interactions: crear índices: %w", err)
	}
	return nil
}

// Record guarda un evento. Devuelve false, sin error, si ya había uno de la
// misma persona, objeto y tipo en el mismo bloque de 10 min (el índice único
// lo rechaza): esa vista no cuenta otra vez.
func (r *InteractionRepo) Record(ctx context.Context, e domain.Event) (bool, error) {
	_, err := r.col.InsertOne(ctx, toInteractionDoc(e, false))
	switch {
	case err == nil:
		return true, nil
	case drv.IsDuplicateKeyError(err):
		return false, nil
	default:
		return false, fmt.Errorf("reco_interactions: guardar evento: %w", err)
	}
}

// Recent devuelve los limit eventos más recientes de userID, del más nuevo al
// más viejo.
func (r *InteractionRepo) Recent(ctx context.Context, userID string, limit int) ([]domain.Event, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(int64(limit))
	return r.find(ctx, bson.D{{Key: "userId", Value: userID}}, opts, "historial")
}

// Since devuelve hasta limit eventos creados desde since, los más recientes
// primero (recálculo del modelo; plan P-11).
func (r *InteractionRepo) Since(ctx context.Context, since time.Time, limit int) ([]domain.Event, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(int64(limit))
	return r.find(ctx, bson.D{{Key: "createdAt", Value: bson.D{{Key: "$gte", Value: since.UTC()}}}}, opts, "ventana")
}

func (r *InteractionRepo) find(ctx context.Context, filter bson.D, opts *options.FindOptionsBuilder, what string) ([]domain.Event, error) {
	cur, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("reco_interactions: leer %s: %w", what, err)
	}
	var docs []interactionDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("reco_interactions: leer %s: %w", what, err)
	}
	out := make([]domain.Event, len(docs))
	for i, d := range docs {
		out[i] = d.toDomain()
	}
	return out, nil
}

// DeleteUser borra todos los eventos de userID (borrar historial, RF-09).
func (r *InteractionRepo) DeleteUser(ctx context.Context, userID string) error {
	if _, err := r.col.DeleteMany(ctx, bson.D{{Key: "userId", Value: userID}}); err != nil {
		return fmt.Errorf("reco_interactions: borrar historial: %w", err)
	}
	return nil
}

// DeleteUsers borra los eventos de varias personas (cuentas dadas de baja,
// RF-06). Sin personas no hace nada.
func (r *InteractionRepo) DeleteUsers(ctx context.Context, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}
	if _, err := r.col.DeleteMany(ctx, bson.D{{Key: "userId", Value: bson.D{{Key: "$in", Value: userIDs}}}}); err != nil {
		return fmt.Errorf("reco_interactions: purgar cuentas dadas de baja: %w", err)
	}
	return nil
}

// InsertDemo guarda eventos sintéticos marcados con demo: true (RF-08) y
// devuelve cuántos guardó. Los repetidos en el mismo bloque se ignoran.
func (r *InteractionRepo) InsertDemo(ctx context.Context, events []domain.Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	docs := make([]any, len(events))
	for i, e := range events {
		docs[i] = toInteractionDoc(e, true)
	}
	res, err := r.col.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if err == nil {
		return len(res.InsertedIDs), nil
	}
	var bwe drv.BulkWriteException
	if errors.As(err, &bwe) && onlyDuplicates(bwe) {
		return len(events) - len(bwe.WriteErrors), nil
	}
	return 0, fmt.Errorf("reco_interactions: guardar demostración: %w", err)
}

func onlyDuplicates(bwe drv.BulkWriteException) bool {
	if bwe.WriteConcernError != nil {
		return false
	}
	for _, we := range bwe.WriteErrors {
		if !we.HasErrorCode(11000) {
			return false
		}
	}
	return true
}

// PurgeDemo borra los eventos de demostración y devuelve cuántos borró.
func (r *InteractionRepo) PurgeDemo(ctx context.Context) (int64, error) {
	res, err := r.col.DeleteMany(ctx, bson.D{{Key: "demo", Value: true}})
	if err != nil {
		return 0, fmt.Errorf("reco_interactions: borrar demostración: %w", err)
	}
	return res.DeletedCount, nil
}
