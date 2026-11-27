package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

// Eraser es la parte de reservas de la baja de cuenta (spec 021, §5.3; Art. XII):
// cumple erasure.Eraser sobre las colecciones de bookings, y solo sobre
// ellas. Se llama DENTRO de la transacción de la baja, con su ctx.
//
// Qué hace:
//   - cierra las solicitudes pendientes de la persona (cancelled si ella las
//     envió, rejected si es la dueña) con el motivo «cuenta eliminada» y
//     devuelve un aviso por contraparte; los avisos salen DESPUÉS del commit;
//   - desprende itemSnapshot.firstPhoto de las reservas de los objetos de la
//     persona (R-02): sus fotos se borran con el objeto (D-2) y no debe quedar
//     una imagen rota en el historial de la contraparte.
//
// Las reservas aceptadas o en curso no se tocan: la baja ya se rechazó con 409
// antes (HasActiveBookings de UserActivity, con el ctx de la transacción). Las
// reservas pasadas conservan ids seudónimos e itemSnapshot (POL-17).
type Eraser struct {
	bookings *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{bookings: db.Collection(BookingsCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "bookings" }

// Erase aplica la baja de userID a las reservas. Es idempotente: una segunda
// ejecución no encuentra pendientes ni fotos y no cambia nada.
func (e *Eraser) Erase(ctx context.Context, userID string, at time.Time) (erasure.EraseResult, error) {
	at = at.UTC()
	res := erasure.EraseResult{Counts: map[string]int64{}}

	cur, err := e.bookings.Find(ctx, bson.D{
		{Key: "status", Value: string(domain.StatusPending)},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "ownerId", Value: userID}},
			bson.D{{Key: "requesterId", Value: userID}},
		}},
	})
	if err != nil {
		return res, fmt.Errorf("bookings: buscar pendientes: %w", err)
	}
	var docs []bookingDoc
	if err := cur.All(ctx, &docs); err != nil {
		return res, fmt.Errorf("bookings: leer pendientes: %w", err)
	}
	for _, d := range docs {
		b := d.toDomain()
		n, err := domain.CloseOnAccountDeletion(&b, userID, at)
		if errors.Is(err, domain.ErrInvalidTransition) {
			continue // ya venció: la tarea periódica la cierra como expirada
		}
		if err != nil {
			return res, fmt.Errorf("bookings: cerrar pendiente: %w", err)
		}
		nd, err := toDoc(b)
		if err != nil {
			return res, fmt.Errorf("bookings: cerrar pendiente: %w", err)
		}
		expected := nd.Version
		nd.Version = expected + 1
		r, err := e.bookings.ReplaceOne(ctx, bson.D{{Key: "_id", Value: nd.ID}, {Key: "version", Value: expected}}, nd)
		if err != nil {
			return res, fmt.Errorf("bookings: cerrar pendiente: %w", err)
		}
		if r.MatchedCount == 0 {
			return res, fmt.Errorf("bookings: cerrar pendiente: %w", domain.ErrVersionConflict)
		}
		res.Counts["bookings.closed"]++
		res.Notices = append(res.Notices, erasure.Notice{
			UserID: n.UserID, Type: n.Type, Title: n.Title, Body: n.Body, Data: n.Data,
		})
	}

	// R-02: la foto del objeto borrado ya no existe. Se sube la versión para que
	// un reemplazo concurrente con la foto vieja falle por conflicto de versión.
	up, err := e.bookings.UpdateMany(ctx,
		bson.D{{Key: "ownerId", Value: userID}, {Key: "itemSnapshot.firstPhoto", Value: bson.D{{Key: "$exists", Value: true}}}},
		bson.D{
			{Key: "$unset", Value: bson.D{{Key: "itemSnapshot.firstPhoto", Value: ""}}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		})
	if err != nil {
		return res, fmt.Errorf("bookings: quitar foto del objeto: %w", err)
	}
	res.Counts["bookings.photosUnset"] = up.ModifiedCount
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
