package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

// Eraser es la parte de recomendaciones de la baja de cuenta (spec 021, §5.3;
// Art. XII): cumple erasure.Eraser sobre reco_interactions y reco_states, y
// solo sobre ellas. Se llama DENTRO de la transacción de la baja, con su ctx.
//
// Borra los eventos de interacción de la persona (hoy solo los purgaba el
// recálculo, después) y su marca de «borrar mi historial»
// (reco_states con _id = "reset:<userId>"; se borra por _id, no por userId). El
// modelo ya calculado (reco_neighbors, reco_popular_lists) es por objeto, no por
// persona, y el siguiente recálculo lo rehace. Ese recálculo y la purga de
// cuentas dadas de baja (DeleteUsers) no fallan si ya no hay datos.
type Eraser struct {
	interactions *drv.Collection
	states       *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{interactions: db.Collection(InteractionsCollection), states: db.Collection(MetaCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "reco" }

// Erase borra los datos de recomendación de userID. Es idempotente.
func (e *Eraser) Erase(ctx context.Context, userID string, _ time.Time) (erasure.EraseResult, error) {
	res := erasure.EraseResult{Counts: map[string]int64{}}
	i, err := e.interactions.DeleteMany(ctx, bson.D{{Key: "userId", Value: userID}})
	if err != nil {
		return res, fmt.Errorf("reco: borrar interacciones: %w", err)
	}
	res.Counts["reco_interactions.deleted"] = i.DeletedCount
	s, err := e.states.DeleteOne(ctx, bson.D{{Key: "_id", Value: resetPrefix + userID}})
	if err != nil {
		return res, fmt.Errorf("reco: borrar marca de reinicio: %w", err)
	}
	res.Counts["reco_states.deleted"] = s.DeletedCount
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
