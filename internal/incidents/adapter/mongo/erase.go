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

// PurgeAtField es el campo con la fecha en que un TTL borra el documento (spec
// 021, D-4). No existe mientras ninguna cuenta implicada se haya dado de baja.
const PurgeAtField = "purgeAt"

// Eraser es la parte de incidencias de la baja de cuenta (spec 021, D-4;
// Art. XII): cumple erasure.Eraser sobre incidents, y solo sobre ella. Se llama
// DENTRO de la transacción de la baja, con su ctx.
//
// Las incidencias donde la persona es quien reporta o la reportada se conservan
// 12 meses y luego un índice TTL las borra solas: el eraser solo fija purgeAt
// (con $min: si las dos partes se dan de baja gana la fecha más temprana) y
// deja el texto SIN cambios (N-3). Las evidencias (imágenes en Cloudinary) se
// devuelven con NotBefore = purgeAt, así el borrado de las fotos coincide con
// el del documento. Las que ya tenían una fecha igual o anterior no se
// vuelven a encolar.
type Eraser struct {
	col *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{col: db.Collection(IncidentsCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "incidents" }

// Erase fija purgeAt = at + 12 meses en las incidencias de userID. Es
// idempotente: repetirla no cambia nada ni encola evidencias otra vez.
func (e *Eraser) Erase(ctx context.Context, userID string, at time.Time) (erasure.EraseResult, error) {
	res := erasure.EraseResult{Counts: map[string]int64{}}
	purge := erasure.PurgeAt(at)

	// Involucra a la persona (índices reporter_createdAt y reported_createdAt) y
	// aún no tiene una fecha igual o anterior.
	filter := bson.D{
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "reporterId", Value: userID}},
			bson.D{{Key: "reportedId", Value: userID}},
		}},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: PurgeAtField, Value: bson.D{{Key: "$exists", Value: false}}}},
			bson.D{{Key: PurgeAtField, Value: bson.D{{Key: "$gt", Value: purge}}}},
		}},
	}
	cur, err := e.col.Find(ctx, filter, options.Find().SetProjection(bson.D{{Key: "evidence", Value: 1}}))
	if err != nil {
		return res, fmt.Errorf("incidents: buscar incidencias: %w", err)
	}
	var found []struct {
		Evidence []imageDoc `bson:"evidence"`
	}
	if err := cur.All(ctx, &found); err != nil {
		return res, fmt.Errorf("incidents: leer incidencias: %w", err)
	}
	if len(found) == 0 {
		return res, nil
	}
	for _, inc := range found {
		for _, ev := range inc.Evidence {
			if ev.PublicID == "" {
				continue
			}
			res.Images = append(res.Images, erasure.ImageRef{
				PublicID:  ev.PublicID,
				Delivery:  images.Delivery(ev.Delivery).CloudinaryType(),
				NotBefore: purge,
			})
		}
	}
	u, err := e.col.UpdateMany(ctx, filter, bson.D{{Key: "$min", Value: bson.D{{Key: PurgeAtField, Value: purge}}}})
	if err != nil {
		return res, fmt.Errorf("incidents: fijar la fecha de purga: %w", err)
	}
	res.Counts["incidents.purgeScheduled"] = u.ModifiedCount
	res.Counts["incidents.evidenceQueued"] = int64(len(res.Images))
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
