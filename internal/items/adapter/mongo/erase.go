package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

// Eraser es la parte de items de la baja de cuenta (spec 021, D-2; Art. XII):
// cumple erasure.Eraser sobre items, y solo sobre ella. Se llama DENTRO de la
// transacción de la baja, con su ctx.
//
// Borra físicamente los objetos de la persona, en cualquier estado, y devuelve
// sus fotos para que la baja las ponga en la bandeja de salida (en la misma
// transacción) y el worker las destruya en Cloudinary con reintento. Las
// reservas pasadas conservan su itemSnapshot, así que el historial ajeno no se
// rompe; el objeto deja de existir para la búsqueda y las recomendaciones, que
// leen de items.
type Eraser struct {
	col *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{col: db.Collection(ItemsCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "items" }

// Erase borra los objetos de userID. Es idempotente: sin objetos no hace nada
// y no devuelve fotos. Usa el índice owner_status_createdAt (prefijo ownerId).
func (e *Eraser) Erase(ctx context.Context, userID string, _ time.Time) (erasure.EraseResult, error) {
	res := erasure.EraseResult{Counts: map[string]int64{}}

	cur, err := e.col.Find(ctx, bson.D{{Key: "ownerId", Value: userID}},
		options.Find().SetProjection(bson.D{{Key: "photos", Value: 1}}))
	if err != nil {
		return res, fmt.Errorf("items: buscar objetos: %w", err)
	}
	var found []struct {
		ID     bson.ObjectID `bson:"_id"`
		Photos []imageDoc    `bson:"photos"`
	}
	if err := cur.All(ctx, &found); err != nil {
		return res, fmt.Errorf("items: leer objetos: %w", err)
	}
	if len(found) == 0 {
		return res, nil
	}
	ids := make(bson.A, 0, len(found))
	for _, it := range found {
		ids = append(ids, it.ID)
		for _, p := range it.Photos {
			if p.PublicID == "" {
				continue
			}
			res.Images = append(res.Images, erasure.ImageRef{
				PublicID: p.PublicID,
				Delivery: images.Delivery(p.Delivery).CloudinaryType(),
			})
		}
	}
	// Se borra por _id (los encontrados con este mismo ctx): lo que otra
	// escritura de la transacción haya cambiado ya está reflejado en la lectura.
	d, err := e.col.DeleteMany(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}})
	if err != nil {
		return res, fmt.Errorf("items: borrar objetos: %w", err)
	}
	res.Counts["items.deleted"] = d.DeletedCount
	res.Counts["items.photosQueued"] = int64(len(res.Images))
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
