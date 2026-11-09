package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
)

// GetStage devuelve la etapa a las dos partes de la reserva, con URLs firmadas
// de 10 minutos (RF-05). Un tercero recibe not_found (CA-09).
func (s *Service) GetStage(ctx context.Context, v Viewer, bookingID string, k domain.Kind) (StageView, error) {
	if err := checkKind(k); err != nil {
		return StageView{}, err
	}
	b, err := s.load(ctx, v, bookingID)
	if err != nil {
		return StageView{}, err
	}
	return s.stageView(k, domain.Of(b, k), s.now())
}
