package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
)

// ListMine devuelve las incidencias de userID (como quien reporta o como parte
// reportada), las más recientes primero (RF-07). status "" = todas. Siempre es
// la vista de las partes, también para un administrador que consulta las suyas.
func (s *Service) ListMine(ctx context.Context, userID string, status domain.Status, after *domain.Cursor, limit int) (Page, error) {
	limit = clampLimit(limit)
	rows, err := s.d.Repo.ListByParty(ctx, userID, status, after, limit+1)
	if err != nil {
		return Page{}, err
	}
	return s.page(ctx, rows, limit, false)
}

// page recorta la fila extra (que indica que hay otra página) y arma las vistas.
func (s *Service) page(ctx context.Context, rows []domain.Incident, limit int, adminView bool) (Page, error) {
	var next *domain.Cursor
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = &domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	items, err := s.views(ctx, rows, adminView)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: items, Next: next}, nil
}
