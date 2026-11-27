package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

// Eraser es la parte de avisos de la baja de cuenta (spec 021, §5.3, POL-17;
// Art. XII): cumple erasure.Eraser sobre notifications y push_tokens, y
// solo sobre ellas. Se llama DENTRO de la transacción de la baja, con su ctx.
//
// Borra los avisos dirigidos a la persona (llevan texto con contenido
// personal) y sus tokens de push (identifican sus dispositivos). Los de otras
// personas no se tocan; los avisos que la baja genera para las contrapartes
// los envía el worker de bajas DESPUÉS del commit. Un push pendiente de una
// cuenta sin tokens no envía nada (el worker de push lo ignora).
type Eraser struct {
	notifications *drv.Collection
	tokens        *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{notifications: db.Collection(NotificationsCollection), tokens: db.Collection(PushTokensCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "notifications" }

// Erase borra avisos y tokens de userID (índices userId). Es idempotente.
func (e *Eraser) Erase(ctx context.Context, userID string, _ time.Time) (erasure.EraseResult, error) {
	res := erasure.EraseResult{Counts: map[string]int64{}}
	filter := bson.D{{Key: "userId", Value: userID}}
	n, err := e.notifications.DeleteMany(ctx, filter)
	if err != nil {
		return res, fmt.Errorf("notifications: borrar avisos: %w", err)
	}
	res.Counts["notifications.deleted"] = n.DeletedCount
	t, err := e.tokens.DeleteMany(ctx, filter)
	if err != nil {
		return res, fmt.Errorf("notifications: borrar tokens de push: %w", err)
	}
	res.Counts["push_tokens.deleted"] = t.DeletedCount
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
