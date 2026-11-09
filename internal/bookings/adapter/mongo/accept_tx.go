package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
)

// Accept guarda la aceptación de b (ya en estado accepted por el dominio, con
// la versión leída) de forma atómica (RF-07, RNF-01, plan P-03), dentro de una
// transacción:
//
//  1. $inc en items.reservationSeq y items.version: dos aceptaciones del mismo
//     objeto chocan (WriteConflict) y el driver reintenta la segunda, que ya ve
//     la primera; además, un DELETE /items concurrente (005) falla por versión
//     y no puede dar de baja un objeto con una reserva recién aceptada. El
//     objeto debe estar active o paused.
//  2. Si otra reserva accepted/in_progress se cruza → domain.ErrPeriodTaken.
//  3. Reemplaza la solicitud solo si sigue pending con la misma versión.
//  4. Las demás pendientes que se cruzan pasan a rejected con motivo
//     no_disponible y se devuelven para notificarlas.
func (r *Repo) Accept(ctx context.Context, b domain.Booking, now time.Time) (domain.Booking, []domain.Booking, error) {
	itemOID, err := bson.ObjectIDFromHex(b.ItemID)
	if err != nil {
		return domain.Booking{}, nil, domain.ErrItemNotFound
	}
	sess, err := r.client.StartSession()
	if err != nil {
		return domain.Booking{}, nil, fmt.Errorf("bookings: iniciar sesión de MongoDB: %w", err)
	}
	defer sess.EndSession(ctx)

	var accepted domain.Booking
	var rejected []domain.Booking
	_, err = sess.WithTransaction(ctx, func(tx context.Context) (any, error) {
		rejected = nil // el driver puede repetir la función
		res, err := r.items.UpdateOne(tx,
			bson.D{{Key: "_id", Value: itemOID}, {Key: "status", Value: bson.D{{Key: "$in", Value: bson.A{"active", "paused"}}}}},
			bson.D{{Key: "$inc", Value: bson.D{{Key: "reservationSeq", Value: 1}, {Key: "version", Value: 1}}}})
		if err != nil {
			return nil, err
		}
		if res.MatchedCount == 0 {
			return nil, domain.ErrItemUnavailable
		}

		taken, err := r.bookings.CountDocuments(tx, append(overlapFilter(b.ItemID, b.Start, b.End),
			bson.E{Key: "status", Value: bson.D{{Key: "$in", Value: blocking()}}}))
		if err != nil {
			return nil, err
		}
		if taken > 0 {
			return nil, domain.ErrPeriodTaken
		}

		d, err := toDoc(b)
		if err != nil {
			return nil, err
		}
		expected := d.Version
		d.Version = expected + 1
		rep, err := r.bookings.ReplaceOne(tx, bson.D{
			{Key: "_id", Value: d.ID}, {Key: "version", Value: expected}, {Key: "status", Value: string(domain.StatusPending)},
			{Key: "expiresAt", Value: bson.D{{Key: "$gt", Value: now.UTC()}}},
		}, d)
		if err != nil {
			return nil, err
		}
		if rep.MatchedCount == 0 {
			return nil, domain.ErrVersionConflict
		}
		accepted = d.toDomain()

		rejected, err = r.rejectOverlappingPending(tx, accepted, now)
		return nil, err
	})
	if err != nil {
		return domain.Booking{}, nil, err
	}
	return accepted, rejected, nil
}

// rejectOverlappingPending rechaza (motivo no_disponible) las pendientes del
// mismo objeto que se cruzan con la aceptada. Las ya vencidas se dejan para el
// vencimiento.
func (r *Repo) rejectOverlappingPending(tx context.Context, acc domain.Booking, now time.Time) ([]domain.Booking, error) {
	oid, _ := bson.ObjectIDFromHex(acc.ID)
	cur, err := r.bookings.Find(tx, append(overlapFilter(acc.ItemID, acc.Start, acc.End),
		bson.E{Key: "status", Value: string(domain.StatusPending)},
		bson.E{Key: "_id", Value: bson.D{{Key: "$ne", Value: oid}}}))
	if err != nil {
		return nil, err
	}
	var docs []bookingDoc
	if err := cur.All(tx, &docs); err != nil {
		return nil, err
	}
	reason := domain.RejectReasonUnavailable
	var out []domain.Booking
	for _, d := range docs {
		other := d.toDomain()
		if other.Transition(domain.StatusRejected, domain.RoleOwner, now) != nil {
			continue
		}
		other.RejectReason = &reason
		saved, err := r.replace(tx, r.bookings, other)
		if err != nil {
			return nil, err
		}
		out = append(out, saved)
	}
	return out, nil
}
