package usecase

import (
	"context"
	"errors"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
)

// Confirm confirma la etapa con el token del QR (RF-03, RF-04): la entrega la
// confirma el solicitante y deja la reserva en in_progress; la devolución, el
// propietario y la deja en completed. La reserva, el QR y el cambio de estado
// se guardan en una sola operación atómica: con el mismo QR dos veces a la vez
// exactamente una tiene éxito (RNF-02). El QR no toca los pagos (RF-07).
func (s *Service) Confirm(ctx context.Context, v Viewer, bookingID string, k domain.Kind, token string) error {
	if err := checkKind(k); err != nil {
		return err
	}
	b, err := s.load(ctx, v, bookingID)
	if err != nil {
		return err
	}
	c, err := domain.PlanConfirm(b, k, v.ID, token, s.now(), s.d.Location, s.d.Lateness)
	if err != nil {
		return err
	}
	if err := s.d.Repo.Confirm(ctx, b.ID, c, domain.HashToken(token)); err != nil {
		if !errors.Is(err, ErrNoMatch) {
			return err
		}
		// Perdió la carrera (o el QR cambió): se explica con la reserva actual.
		fresh, loadErr := s.load(ctx, v, bookingID)
		if loadErr != nil {
			return loadErr
		}
		if _, planErr := domain.PlanConfirm(fresh, k, v.ID, token, s.now(), s.d.Location, s.d.Lateness); planErr != nil {
			return planErr
		}
		return domain.ErrQRUsed
	}
	title, body := "Entrega confirmada", "La entrega del objeto quedó confirmada."
	typ := "delivery_confirmed"
	if k == domain.KindReturn {
		title, body, typ = "Devolución confirmada", "La devolución del objeto quedó confirmada.", "return_confirmed"
	}
	s.notifyOther(ctx, b, v.ID, typ, title, body)
	return nil
}
