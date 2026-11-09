package usecase

import (
	"context"
	"errors"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// RecordEvidence registra o reemplaza el estado y las fotos de la etapa (RF-01,
// RF-04). Reemplazar sube evidenceVersion e invalida el QR vigente. Después de
// confirmada la etapa ya no se puede cambiar (RF-13).
func (s *Service) RecordEvidence(ctx context.Context, v Viewer, bookingID string, k domain.Kind, notes string, refs []images.Ref) (StageView, error) {
	if err := checkKind(k); err != nil {
		return StageView{}, err
	}
	b, err := s.load(ctx, v, bookingID)
	if err != nil {
		return StageView{}, err
	}
	if err := domain.CanRecord(b, k, v.ID); err != nil {
		return StageView{}, err
	}
	text, err := domain.ValidateNotes(notes)
	if err != nil {
		return StageView{}, err
	}
	if err := domain.ValidatePhotoCount(len(refs)); err != nil {
		return StageView{}, err
	}
	imgs, err := s.d.Images.Resolve(ctx, v.ID, images.PurposeEvidence, "photos", refs)
	if err != nil {
		return StageView{}, err
	}
	h := domain.Record(domain.Of(b, k), v.ID, text, imgs, s.now())
	if err := s.d.Repo.SaveEvidence(ctx, b.ID, k, h); err != nil {
		return StageView{}, s.explain(ctx, v, b.ID, k, err, domain.CanRecord)
	}
	return s.stageView(k, &h, s.now())
}

// explain traduce un ErrNoMatch del repositorio: vuelve a leer la reserva y
// aplica la misma regla para devolver el error preciso; si la regla ya pasa,
// otra petición cambió la reserva a la vez y se responde ErrWrongStatus.
func (s *Service) explain(ctx context.Context, v Viewer, id string, k domain.Kind, err error,
	check func(bookings.Booking, domain.Kind, string) error) error {
	if !errors.Is(err, ErrNoMatch) {
		return err
	}
	fresh, loadErr := s.load(ctx, v, id)
	if loadErr != nil {
		return loadErr
	}
	if ruleErr := check(fresh, k, v.ID); ruleErr != nil {
		return ruleErr
	}
	return domain.ErrWrongStatus
}
