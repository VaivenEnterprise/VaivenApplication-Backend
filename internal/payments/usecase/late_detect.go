package usecase

import (
	"context"
	"errors"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
)

// DetectLate marca las devoluciones tardías y las reservas afectadas (RF-09,
// plan P-05 y P-07). Devuelve cuántas reservas marcó como tardías. Es
// idempotente y segura con varias réplicas: cada marca es atómica y una que otra
// réplica ya hizo se omite, así que cada aviso sale una sola vez.
//
//   - Una reserva in_progress cuyo lateAt ya pasó queda isLate y se avisa a las
//     dos partes (booking_late).
//   - Otra reserva aceptada del mismo objeto que debía empezar después de la
//     devolución prevista y cuyo inicio ya llegó queda afectada (affectedBy) y se
//     avisa a su solicitante (booking_affected).
//
// El ajuste económico no se calcula aquí: se registra al confirmar la devolución
// (payments/domain.Lateness.Adjustment, que recibe el módulo de entrega).
func (s *Service) DetectLate(ctx context.Context) (int, error) {
	now := s.now()
	due, err := s.d.Repo.DueLate(ctx, now, detectBatch)
	if err != nil {
		return 0, err
	}
	marked := 0
	for _, late := range due {
		// Primero las reservas afectadas y después la marca de tardía: si el
		// proceso se interrumpe entre las dos, la siguiente pasada retoma la
		// reserva (sigue sin marcar) y no se pierde ninguna. Cada aviso solo sale
		// cuando su marca atómica tuvo éxito, así que no se repite.
		if err := s.markAffected(ctx, late, now); err != nil {
			return marked, err
		}
		if err := s.d.Repo.MarkLate(ctx, late.ID, now); err != nil {
			if errors.Is(err, ErrNoMatch) {
				continue // otra réplica la marcó o ya se devolvió
			}
			return marked, err
		}
		marked++
		s.notifyUser(ctx, late.OwnerID, "booking_late", "Devolución tardía",
			"La devolución del objeto ya pasó la fecha acordada.", late.ID, late.ItemID)
		s.notifyUser(ctx, late.RequesterID, "booking_late", "Devolución tardía",
			"Ya pasó la fecha de devolución del objeto. Devuélvelo lo antes posible.", late.ID, late.ItemID)
	}
	return marked, nil
}

func (s *Service) markAffected(ctx context.Context, late LateBooking, now time.Time) error {
	cands, err := s.d.Repo.AffectedCandidates(ctx, late, bookings.Today(now, s.d.Location))
	if err != nil {
		return err
	}
	for _, other := range cands {
		if err := s.d.Repo.MarkAffected(ctx, other.ID, late.ID, now); err != nil {
			if errors.Is(err, ErrNoMatch) {
				continue
			}
			return err
		}
		s.notifyUser(ctx, other.RequesterID, "booking_affected", "Tu reserva se retrasa",
			"La persona anterior aún no devuelve el objeto. Puedes esperar o cancelar sin penalización.", other.ID, other.ItemID)
	}
	return nil
}
