package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

// PurgeAtField es el campo con la fecha en que un TTL borra el documento (spec
// 021, D-4). No existe mientras la cuenta sancionada no se haya dado de baja.
const PurgeAtField = "purgeAt"

// Eraser es la parte de sanciones de la baja de cuenta (spec 021, D-4; Art.
// XII): cumple erasure.Eraser sobre sanctions, y solo sobre ella. Se llama
// DENTRO de la transacción de la baja, con su ctx.
//
// Las sanciones de la persona se conservan 12 meses y luego un índice TTL las
// borra solas: el eraser solo fija purgeAt (con $min) y deja el texto SIN
// cambios (N-3).
type Eraser struct {
	col *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{col: db.Collection(SanctionsCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "sanctions" }

// Erase fija purgeAt = at + 12 meses en las sanciones de userID. Es idempotente.
func (e *Eraser) Erase(ctx context.Context, userID string, at time.Time) (erasure.EraseResult, error) {
	res := erasure.EraseResult{Counts: map[string]int64{}}
	purge := erasure.PurgeAt(at)

	// Índice user_createdAt (prefijo userId).
	filter := bson.D{
		{Key: "userId", Value: userID},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: PurgeAtField, Value: bson.D{{Key: "$exists", Value: false}}}},
			bson.D{{Key: PurgeAtField, Value: bson.D{{Key: "$gt", Value: purge}}}},
		}},
	}
	u, err := e.col.UpdateMany(ctx, filter, bson.D{{Key: "$min", Value: bson.D{{Key: PurgeAtField, Value: purge}}}})
	if err != nil {
		return res, fmt.Errorf("sanctions: fijar la fecha de purga: %w", err)
	}
	res.Counts["sanctions.purgeScheduled"] = u.ModifiedCount
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
