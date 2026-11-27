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
// 021, D-4). No existe mientras ninguna cuenta implicada se haya dado de baja.
const PurgeAtField = "purgeAt"

// Eraser es la parte de reportes de la baja de cuenta (spec 021, D-4; Art. XII):
// cumple erasure.Eraser sobre reports, y solo sobre ella. Se llama DENTRO de la
// transacción de la baja, con su ctx.
//
// Los reportes que la persona hizo, o que recibió como persona reportada
// (targetType = user), se conservan 12 meses y luego un índice TTL los borra
// solos: el eraser solo fija purgeAt (con $min) y deja el texto SIN cambios
// (N-3). Un reporte sobre un objeto o mensaje de la persona no la involucra por
// sí mismo (el objeto se borra con items; la moderación tolera not_found).
type Eraser struct {
	col *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{col: db.Collection(ReportsCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "reports" }

// Erase fija purgeAt = at + 12 meses en los reportes de userID. Es idempotente.
func (e *Eraser) Erase(ctx context.Context, userID string, at time.Time) (erasure.EraseResult, error) {
	res := erasure.EraseResult{Counts: map[string]int64{}}
	purge := erasure.PurgeAt(at)

	// Índices reporter_target_unique (prefijo reporterId) y target.
	filter := bson.D{
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "reporterId", Value: userID}},
			bson.D{{Key: "targetType", Value: "user"}, {Key: "targetId", Value: userID}},
		}},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: PurgeAtField, Value: bson.D{{Key: "$exists", Value: false}}}},
			bson.D{{Key: PurgeAtField, Value: bson.D{{Key: "$gt", Value: purge}}}},
		}},
	}
	u, err := e.col.UpdateMany(ctx, filter, bson.D{{Key: "$min", Value: bson.D{{Key: PurgeAtField, Value: purge}}}})
	if err != nil {
		return res, fmt.Errorf("reports: fijar la fecha de purga: %w", err)
	}
	res.Counts["reports.purgeScheduled"] = u.ModifiedCount
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
