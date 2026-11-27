package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

// Eraser es la parte de uso de IA de la baja de cuenta (spec 021, §5.3; Art.
// XII): cumple erasure.Eraser sobre ai_usage, y solo sobre ella. Se llama
// DENTRO de la transacción de la baja, con su ctx. Borra los contadores de uso
// de la persona (sin base para conservarlos; ya vencen a los 3 días). Los de
// otras personas no se tocan.
type Eraser struct {
	col *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{col: db.Collection(UsageCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "ai" }

// Erase borra los contadores de userID (índice user_feature_day_unique, cuyo
// prefijo es userId). Es idempotente.
func (e *Eraser) Erase(ctx context.Context, userID string, _ time.Time) (erasure.EraseResult, error) {
	res := erasure.EraseResult{Counts: map[string]int64{}}
	d, err := e.col.DeleteMany(ctx, bson.D{{Key: "userId", Value: userID}})
	if err != nil {
		return res, fmt.Errorf("ai: borrar uso de IA: %w", err)
	}
	res.Counts["ai_usage.deleted"] = d.DeletedCount
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
