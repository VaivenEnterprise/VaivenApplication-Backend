package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

// Get devuelve el detalle de un objeto si viewer lo puede ver (RF-06):
// active cualquiera; paused y blocked solo el dueño y admin; removed nadie.
// Si no lo puede ver responde como si no existiera (404), para no revelar
// objetos pausados (CA-06).
func (s *Service) Get(ctx context.Context, viewer Viewer, id string) (ItemView, error) {
	it, err := s.d.Items.ByID(ctx, id)
	if err != nil {
		return ItemView{}, err
	}
	if !it.VisibleTo(viewer.ID, viewer.Admin) {
		return ItemView{}, domain.ErrNotFound
	}
	return s.view(ctx, it, viewer)
}

// Categories devuelve las categorías de POL-14 (RF-11).
func (s *Service) Categories() []domain.Category { return domain.Categories() }
