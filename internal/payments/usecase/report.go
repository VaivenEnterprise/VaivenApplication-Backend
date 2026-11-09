package usecase

import (
	"context"
	"errors"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/domain"
)

// Report registra que el solicitante pagó la etapa: pending → reported (RF-07).
// Avisa al propietario. El QR de entrega o devolución no interviene.
func (s *Service) Report(ctx context.Context, v Viewer, bookingID string, stage bookings.Stage, note *string) error {
	if !domain.ValidStage(stage) {
		return bookings.ErrNotFound
	}
	b, err := s.load(ctx, v, bookingID)
	if err != nil {
		return err
	}
	c, err := domain.PlanReport(b, v.ID, stage, note, s.now())
	if err != nil {
		return err
	}
	if err := s.apply(ctx, v, b.ID, c, func(fresh bookings.Booking) error {
		_, err := domain.PlanReport(fresh, v.ID, stage, note, c.At)
		return err
	}); err != nil {
		return err
	}
	s.notifyUser(ctx, b.OwnerID, "payment_reported", "Pago reportado",
		"El solicitante informó que realizó un pago. Confírmalo cuando lo recibas.", b.ID, b.ItemID)
	return nil
}

// apply guarda el cambio de forma atómica. Si otra petición cambió el pago a la
// vez, vuelve a leer la reserva y devuelve el error preciso de la misma regla.
func (s *Service) apply(ctx context.Context, v Viewer, bookingID string, c domain.Change, recheck func(bookings.Booking) error) error {
	err := s.d.Repo.ChangePayment(ctx, bookingID, c)
	if err == nil || !errors.Is(err, ErrNoMatch) {
		return err
	}
	fresh, loadErr := s.load(ctx, v, bookingID)
	if loadErr != nil {
		return loadErr
	}
	if ruleErr := recheck(fresh); ruleErr != nil {
		return ruleErr
	}
	return domain.ErrWrongBookingStatus
}
