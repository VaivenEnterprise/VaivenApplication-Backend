// Package mongo implementa el repositorio de pagos y retrasos con MongoDB
// (spec 009, T010–T011; plan P-09). Guarda dentro de los documentos de
// bookings: solo hace $set de sus campos con filtro por estado, nunca
// reemplaza el documento completo, y sube version para que un guardado
// concurrente de la spec 007 detecte el cambio en vez de pisarlo.
package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/usecase"
)

// Repo guarda los pagos y las marcas de retraso dentro de la colección bookings.
type Repo struct {
	bookings *drv.Collection
}

var _ usecase.Repository = (*Repo)(nil)

// NewRepo crea el repositorio sobre db.
func NewRepo(db *drv.Database) *Repo {
	return &Repo{bookings: db.Collection(bookingsmongo.BookingsCollection)}
}

// PaymentIndexes declara los índices de bookingsmongo.BookingsCollection: es la fuente única
// de verdad que consumen EnsureIndexes y el catálogo de `admin indexes`
// (spec 012, T004). Solo datos: no abre conexiones.
func PaymentIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(bookingsmongo.BookingsCollection,
		drv.IndexModel{
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "lateAt", Value: 1}},
			Options: options.Index().SetName("status_lateAt"),
		},
	)
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *Repo) Indexes() []adaptermongo.IndexSpec { return PaymentIndexes() }

// EnsureIndexes crea el índice {status, lateAt} del detector de retrasos (plan
// 009 §3). Es idempotente.
func (r *Repo) EnsureIndexes(ctx context.Context) error {
	_, err := r.bookings.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes()))
	if err != nil {
		return fmt.Errorf("payments: crear índice: %w", err)
	}
	return nil
}

// accepting son los estados de reserva que admiten pagos.
func accepting() bson.A {
	return bson.A{string(bookings.StatusAccepted), string(bookings.StatusInProgress), string(bookings.StatusCompleted)}
}

func objectID(id string) (bson.ObjectID, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.ObjectID{}, bookings.ErrNotFound
	}
	return oid, nil
}

// apply ejecuta un UpdateOne y traduce «no coincidió» a usecase.ErrNoMatch.
func (r *Repo) apply(ctx context.Context, what string, filter, update bson.D) error {
	res, err := r.bookings.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("payments: %s: %w", what, err)
	}
	if res.MatchedCount == 0 {
		return usecase.ErrNoMatch
	}
	return nil
}

// ChangePayment pasa el pago de la etapa de c.From a c.To. El filtro exige que
// la reserva admita pagos y que el elemento de payments siga en c.From, así que
// de dos peticiones iguales a la vez solo una coincide (plan P-08, P-09).
func (r *Repo) ChangePayment(ctx context.Context, bookingID string, c domain.Change) error {
	oid, err := objectID(bookingID)
	if err != nil {
		return err
	}
	at := c.At.UTC()
	set := bson.D{{Key: "payments.$.status", Value: string(c.To)}, {Key: "updatedAt", Value: at}}
	switch c.To {
	case bookings.PaymentReported:
		set = append(set,
			bson.E{Key: "payments.$.reportedAt", Value: at},
			bson.E{Key: "payments.$.reportedNote", Value: c.Note})
	case bookings.PaymentConfirmed:
		set = append(set, bson.E{Key: "payments.$.confirmedAt", Value: at})
	}
	return r.apply(ctx, "cambiar pago",
		bson.D{
			{Key: "_id", Value: oid},
			{Key: "status", Value: bson.D{{Key: "$in", Value: accepting()}}},
			{Key: "payments", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
				{Key: "stage", Value: string(c.Stage)},
				{Key: "status", Value: string(c.From)},
			}}}},
		},
		bson.D{
			{Key: "$set", Value: set},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		})
}

// lateDoc son los campos que necesita el detector de retrasos.
type lateDoc struct {
	ID          bson.ObjectID `bson:"_id"`
	ItemID      string        `bson:"itemId"`
	OwnerID     string        `bson:"ownerId"`
	RequesterID string        `bson:"requesterId"`
	EndDate     string        `bson:"endDate"`
}

func (d lateDoc) toLate() usecase.LateBooking {
	return usecase.LateBooking{
		ID: d.ID.Hex(), ItemID: d.ItemID, OwnerID: d.OwnerID, RequesterID: d.RequesterID,
		End: bookings.MustDate(d.EndDate),
	}
}

func (r *Repo) findLate(ctx context.Context, what string, filter bson.D, sort bson.D, limit int64) ([]usecase.LateBooking, error) {
	cur, err := r.bookings.Find(ctx, filter, options.Find().
		SetProjection(bson.D{{Key: "itemId", Value: 1}, {Key: "ownerId", Value: 1}, {Key: "requesterId", Value: 1}, {Key: "endDate", Value: 1}}).
		SetSort(sort).SetLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("payments: %s: %w", what, err)
	}
	var docs []lateDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("payments: leer %s: %w", what, err)
	}
	out := make([]usecase.LateBooking, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toLate())
	}
	return out, nil
}

// DueLate devuelve hasta limit reservas in_progress sin marcar cuyo lateAt ya
// pasó (usa el índice {status, lateAt}).
func (r *Repo) DueLate(ctx context.Context, now time.Time, limit int) ([]usecase.LateBooking, error) {
	return r.findLate(ctx, "buscar retrasos",
		bson.D{
			{Key: "status", Value: string(bookings.StatusInProgress)},
			{Key: "lateAt", Value: bson.D{{Key: "$lt", Value: now.UTC()}}},
			{Key: "isLate", Value: bson.D{{Key: "$ne", Value: true}}},
		},
		bson.D{{Key: "lateAt", Value: 1}}, int64(limit))
}

// MarkLate marca isLate si la reserva sigue in_progress y sin marcar.
func (r *Repo) MarkLate(ctx context.Context, bookingID string, at time.Time) error {
	oid, err := objectID(bookingID)
	if err != nil {
		return err
	}
	return r.apply(ctx, "marcar retraso",
		bson.D{
			{Key: "_id", Value: oid},
			{Key: "status", Value: string(bookings.StatusInProgress)},
			{Key: "isLate", Value: bson.D{{Key: "$ne", Value: true}}},
		},
		bson.D{
			{Key: "$set", Value: bson.D{{Key: "isLate", Value: true}, {Key: "updatedAt", Value: at.UTC()}}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		})
}

// AffectedCandidates devuelve las reservas aceptadas del mismo objeto que
// empiezan después de late.End y a más tardar hoy, sin marcar aún (plan P-07).
// Las fechas son cadenas YYYY-MM-DD, así que comparan en orden.
func (r *Repo) AffectedCandidates(ctx context.Context, late usecase.LateBooking, today bookings.Date) ([]usecase.LateBooking, error) {
	return r.findLate(ctx, "buscar afectadas",
		bson.D{
			{Key: "itemId", Value: late.ItemID},
			{Key: "status", Value: string(bookings.StatusAccepted)},
			{Key: "startDate", Value: bson.D{{Key: "$gt", Value: late.End.String()}, {Key: "$lte", Value: today.String()}}},
			{Key: "affectedBy", Value: nil},
		},
		bson.D{{Key: "startDate", Value: 1}}, 20)
}

// MarkAffected marca affectedBy si la reserva sigue aceptada y sin marcar.
func (r *Repo) MarkAffected(ctx context.Context, bookingID, lateBookingID string, at time.Time) error {
	oid, err := objectID(bookingID)
	if err != nil {
		return err
	}
	return r.apply(ctx, "marcar afectada",
		bson.D{
			{Key: "_id", Value: oid},
			{Key: "status", Value: string(bookings.StatusAccepted)},
			{Key: "affectedBy", Value: nil},
		},
		bson.D{
			{Key: "$set", Value: bson.D{{Key: "affectedBy", Value: lateBookingID}, {Key: "updatedAt", Value: at.UTC()}}},
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		})
}
