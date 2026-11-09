package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
)

// AdminList devuelve la cola de incidencias para administración, las más
// antiguas primero (RF-08). status "" equivale a las abiertas. El rol admin lo
// exige la capa HTTP (RF-11).
func (s *Service) AdminList(ctx context.Context, status domain.Status, after *domain.Cursor, limit int) (Page, error) {
	if status == "" {
		status = domain.StatusOpen
	}
	limit = clampLimit(limit)
	rows, err := s.d.Repo.ListAll(ctx, status, after, limit+1)
	if err != nil {
		return Page{}, err
	}
	return s.page(ctx, rows, limit, true)
}
