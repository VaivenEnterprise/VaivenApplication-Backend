package mongo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// MetaCollection guarda el modelo vigente, el candado del recálculo y las
// fechas de borrado de historial (plan P-04 y P-09). Toda lectura es por _id.
const MetaCollection = "reco_states"

const (
	modelMetaID = "model"
	resetPrefix = "reset:"
)

type modelMetaDoc struct {
	ID          string     `bson:"_id"`
	BuildID     string     `bson:"buildId"`
	BuiltAt     *time.Time `bson:"builtAt"`
	LockedUntil time.Time  `bson:"lockedUntil"`
	LockOwner   string     `bson:"lockOwner"`
	CreatedAt   time.Time  `bson:"createdAt"`
	UpdatedAt   time.Time  `bson:"updatedAt"`
}

type resetDoc struct {
	ID      string    `bson:"_id"`
	ResetAt time.Time `bson:"resetAt"`
}

// MetaRepo es el estado del modelo y el candado del recálculo. El candado vive
// en MongoDB, no en Redis ni en memoria (Art. XIV.1 y XIV.2): sirve aunque haya
// varias réplicas y vence solo si el proceso muere.
type MetaRepo struct {
	col *drv.Collection
}

// NewMetaRepo crea el repositorio sobre db.
func NewMetaRepo(db *drv.Database) *MetaRepo {
	return &MetaRepo{col: db.Collection(MetaCollection)}
}

// TryLock intenta tomar el candado del recálculo para owner hasta now +
// domain.LockTTL. Devuelve false, sin error, si otro lo tiene vigente.
//
// Es atómico: el filtro solo coincide si el candado venció. Si está vigente,
// el upsert intenta insertar otro documento con _id "model" y MongoDB lo
// rechaza por clave duplicada; eso significa «ocupado».
func (r *MetaRepo) TryLock(ctx context.Context, owner string, now time.Time) (bool, error) {
	now = now.UTC()
	filter := bson.D{{Key: "_id", Value: modelMetaID}, {Key: "lockedUntil", Value: bson.D{{Key: "$lte", Value: now}}}}
	update := bson.D{
		{Key: "$set", Value: bson.D{{Key: "lockedUntil", Value: now.Add(domain.LockTTL)}, {Key: "lockOwner", Value: owner}, {Key: "updatedAt", Value: now}}},
		{Key: "$setOnInsert", Value: bson.D{{Key: "buildId", Value: ""}, {Key: "builtAt", Value: nil}, {Key: "createdAt", Value: now}}},
	}
	err := r.col.FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetUpsert(true)).Err()
	switch {
	case err == nil, errors.Is(err, drv.ErrNoDocuments):
		return true, nil
	case drv.IsDuplicateKeyError(err):
		return false, nil
	default:
		return false, fmt.Errorf("reco_states: tomar candado: %w", err)
	}
}

// Publish vuelve vigente el modelo buildID y suelta el candado, solo si owner
// todavía lo tiene. Devuelve false si el candado ya era de otro (venció y otro
// proceso lo tomó): entonces el modelo no se publica.
func (r *MetaRepo) Publish(ctx context.Context, owner, buildID string, builtAt time.Time) (bool, error) {
	builtAt = builtAt.UTC()
	res, err := r.col.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: modelMetaID}, {Key: "lockOwner", Value: owner}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "buildId", Value: buildID}, {Key: "builtAt", Value: builtAt},
			{Key: "lockedUntil", Value: time.Time{}}, {Key: "lockOwner", Value: ""}, {Key: "updatedAt", Value: builtAt},
		}}})
	if err != nil {
		return false, fmt.Errorf("reco_states: publicar modelo: %w", err)
	}
	return res.MatchedCount == 1, nil
}

// Unlock suelta el candado de owner sin publicar (el recálculo falló).
func (r *MetaRepo) Unlock(ctx context.Context, owner string) error {
	_, err := r.col.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: modelMetaID}, {Key: "lockOwner", Value: owner}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "lockedUntil", Value: time.Time{}}, {Key: "lockOwner", Value: ""}}}})
	if err != nil {
		return fmt.Errorf("reco_states: soltar candado: %w", err)
	}
	return nil
}

// Current devuelve el modelo vigente. ok es false si todavía no hay ninguno.
func (r *MetaRepo) Current(ctx context.Context) (buildID string, builtAt time.Time, ok bool, err error) {
	var d modelMetaDoc
	err = r.col.FindOne(ctx, bson.D{{Key: "_id", Value: modelMetaID}}).Decode(&d)
	switch {
	case errors.Is(err, drv.ErrNoDocuments):
		return "", time.Time{}, false, nil
	case err != nil:
		return "", time.Time{}, false, fmt.Errorf("reco_states: leer modelo vigente: %w", err)
	case d.BuildID == "" || d.BuiltAt == nil:
		return "", time.Time{}, false, nil
	}
	return d.BuildID, d.BuiltAt.UTC(), true, nil
}

// SetReset guarda que userID borró su historial en at: sus solicitudes
// anteriores dejan de contar (plan P-09).
func (r *MetaRepo) SetReset(ctx context.Context, userID string, at time.Time) error {
	at = at.UTC()
	_, err := r.col.UpdateOne(ctx, bson.D{{Key: "_id", Value: resetPrefix + userID}},
		bson.D{
			{Key: "$set", Value: bson.D{{Key: "resetAt", Value: at}, {Key: "updatedAt", Value: at}}},
			{Key: "$setOnInsert", Value: bson.D{{Key: "createdAt", Value: at}}},
		}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("reco_states: guardar borrado de historial: %w", err)
	}
	return nil
}

// Resets devuelve la fecha de borrado de historial de cada persona que lo
// borró. La consulta usa el índice de _id con un prefijo fijo.
func (r *MetaRepo) Resets(ctx context.Context) (map[string]time.Time, error) {
	cur, err := r.col.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$regex", Value: "^" + resetPrefix}}}})
	if err != nil {
		return nil, fmt.Errorf("reco_states: leer borrados de historial: %w", err)
	}
	var docs []resetDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("reco_states: leer borrados de historial: %w", err)
	}
	out := make(map[string]time.Time, len(docs))
	for _, d := range docs {
		out[strings.TrimPrefix(d.ID, resetPrefix)] = d.ResetAt.UTC()
	}
	return out, nil
}

// ResetAt devuelve la fecha de borrado de historial de userID, si la hay.
func (r *MetaRepo) ResetAt(ctx context.Context, userID string) (time.Time, bool, error) {
	var d resetDoc
	err := r.col.FindOne(ctx, bson.D{{Key: "_id", Value: resetPrefix + userID}}).Decode(&d)
	switch {
	case errors.Is(err, drv.ErrNoDocuments):
		return time.Time{}, false, nil
	case err != nil:
		return time.Time{}, false, fmt.Errorf("reco_states: leer borrado de historial: %w", err)
	}
	return d.ResetAt.UTC(), true, nil
}
