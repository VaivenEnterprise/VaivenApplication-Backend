package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

// Eraser es la parte de calificaciones de la baja de cuenta (spec 021, D-3;
// Art. XII): cumple erasure.Eraser sobre ratings, y solo sobre ella. Se
// llama DENTRO de la transacción de la baja, con su ctx.
//
// Quita el comentario de las calificaciones que escribió la persona
// ($unset: el campo desaparece, nunca queda null). Las estrellas (score) se
// conservan para que los promedios ajenos no cambien (CA-08). Las
// calificaciones que la persona RECIBIÓ no se tocan: son de quien las dio.
type Eraser struct {
	col *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{col: db.Collection(RatingsCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "ratings" }

// Erase quita los comentarios de userID (índice raterId). Es idempotente: solo
// filtra las que aún tienen comentario.
func (e *Eraser) Erase(ctx context.Context, userID string, at time.Time) (erasure.EraseResult, error) {
	res := erasure.EraseResult{Counts: map[string]int64{}}
	up, err := e.col.UpdateMany(ctx,
		bson.D{{Key: "raterId", Value: userID}, {Key: "comment", Value: bson.D{{Key: "$exists", Value: true}}}},
		bson.D{
			{Key: "$unset", Value: bson.D{{Key: "comment", Value: ""}}},
			{Key: "$set", Value: bson.D{{Key: "updatedAt", Value: at.UTC()}}},
		})
	if err != nil {
		return res, fmt.Errorf("ratings: quitar comentarios: %w", err)
	}
	res.Counts["ratings.commentsRemoved"] = up.ModifiedCount
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
