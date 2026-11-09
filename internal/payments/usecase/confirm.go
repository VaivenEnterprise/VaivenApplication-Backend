package usecase

import (
	"context"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/domain"
)

// Confirm registra que el propietario recibió el pago reportado: reported →
// confirmed (RF-07, CA-06). Sin reporte previo es 409. Avisa al solicitante.
func (s *Service) Confirm(ctx context.Context, v Viewer, bookingID string, stage bookings.Stage) error {
	if !domain.ValidStage(stage) {
		return bookings.ErrNotFound
	}
	b, err := s.load(ctx, v, bookingID)
	if err != nil {
		return err
	}
	c, err := domain.PlanConfirm(b, v.ID, stage, s.now())
	if err != nil {
		return err
	}
	if err := s.apply(ctx, v, b.ID, c, func(fresh bookings.Booking) error {
		_, err := domain.PlanConfirm(fresh, v.ID, stage, c.At)
		return err
	}); err != nil {
		return err
	}
	s.notifyUser(ctx, b.RequesterID, "payment_confirmed", "Pago confirmado",
		"El propietario confirmó que recibió tu pago.", b.ID, b.ItemID)
	return nil
}
