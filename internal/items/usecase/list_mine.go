package usecase

import (
	"context"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

// ListQuery son los filtros de GET /me/items (RF-10).
type ListQuery struct {
	// Status nil = todos menos removed.
	Status *domain.Status
	After  *domain.Cursor
	// Limit ≤ 0 usa DefaultPageSize; se acota a MaxPageSize.
	Limit int
}

// ListMine devuelve los objetos de viewer, más recientes primero, sin los
// removed (RF-10).
func (s *Service) ListMine(ctx context.Context, viewer Viewer, q ListQuery) (Page, error) {
	statuses := []domain.Status{domain.StatusActive, domain.StatusPaused, domain.StatusBlocked}
	if q.Status != nil {
		if !domain.ListableStatus(*q.Status) {
			return Page{}, shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: "status", Issue: "no es una opción válida"})
		}
		statuses = []domain.Status{*q.Status}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultPageSize
	}
	limit = min(limit, MaxPageSize)

	// Se pide uno de más para saber si hay otra página.
	items, err := s.d.Items.ListByOwner(ctx, viewer.ID, statuses, q.After, limit+1)
	if err != nil {
		return Page{}, err
	}
	var next *domain.Cursor
	if len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		next = &domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	page := Page{Items: make([]ItemView, 0, len(items)), Next: next}
	if len(items) == 0 {
		return page, nil
	}
	owners, err := s.d.Users.PublicUsers(ctx, []string{viewer.ID})
	if err != nil {
		return Page{}, err
	}
	for _, it := range items {
		v, err := s.viewWith(it, viewer, owners)
		if err != nil {
			return Page{}, err
		}
		page.Items = append(page.Items, v)
	}
	return page, nil
}
