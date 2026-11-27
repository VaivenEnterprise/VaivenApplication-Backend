package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

// Eraser es la parte del asistente de la baja de cuenta (spec 021, §5.3; Art.
// XII): cumple erasure.Eraser sobre ai_conversations, y solo sobre ella. Se
// llama DENTRO de la transacción de la baja, con su ctx. Borra las
// conversaciones de la persona con el asistente (el texto que escribió). Las de
// otras personas no se tocan.
type Eraser struct {
	col *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{col: db.Collection(ConversationsCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "assistant" }

// Erase borra las conversaciones de userID. La colección no tiene índice por
// userId a propósito: sus documentos viven 24 h (TTL de updatedAt), así que el
// recorrido es pequeño. Es idempotente.
func (e *Eraser) Erase(ctx context.Context, userID string, _ time.Time) (erasure.EraseResult, error) {
	res := erasure.EraseResult{Counts: map[string]int64{}}
	d, err := e.col.DeleteMany(ctx, bson.D{{Key: "userId", Value: userID}})
	if err != nil {
		return res, fmt.Errorf("assistant: borrar conversaciones: %w", err)
	}
	res.Counts["ai_conversations.deleted"] = d.DeletedCount
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
