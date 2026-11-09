package usecase

import (
	"context"
	"errors"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Get devuelve una reserva a sus partes o a admin (RF-10).
func (s *Service) Get(ctx context.Context, viewer Viewer, id string) (BookingView, error) {
	b, role, err := s.load(ctx, viewer, id, true)
	if err != nil {
		return BookingView{}, err
	}
	return s.view(ctx, b, role)
}

// ListQuery son los filtros de GET /me/bookings.
type ListQuery struct {
	Role   domain.Role
	Status *domain.Status
	After  *domain.Cursor
	Limit  int
}

// List devuelve las reservas de viewer por rol y estado (RF-10).
func (s *Service) List(ctx context.Context, viewer Viewer, q ListQuery) (Page, error) {
	if q.Role != domain.RoleOwner && q.Role != domain.RoleRequester {
		return Page{}, shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: "role", Issue: "debe ser owner o requester"})
	}
	if q.Status != nil && !q.Status.Valid() {
		return Page{}, shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: "status", Issue: "no es una opción válida"})
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultPageSize
	}
	limit = min(limit, MaxPageSize)
	items, err := s.d.Repo.ListByParty(ctx, viewer.ID, q.Role, q.Status, q.After, limit+1, s.now())
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: make([]BookingView, 0, min(len(items), limit))}
	if len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		page.Next = &domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	if len(items) == 0 {
		return page, nil
	}
	ids := make([]string, 0, 2*len(items))
	for _, b := range items {
		ids = append(ids, b.OwnerID, b.RequesterID)
	}
	profiles, err := s.d.Users.PublicUsers(ctx, ids)
	if err != nil {
		return Page{}, err
	}
	for _, b := range items {
		v, err := s.viewWith(b, q.Role, profiles)
		if err != nil {
			return Page{}, err
		}
		page.Items = append(page.Items, v)
	}
	return page, nil
}

// ExpireDue pasa a expired las solicitudes pendientes vencidas (RF-06, plan
// P-04) y avisa a quien solicitó. Devuelve cuántas venció. Es idempotente y
// segura con varias réplicas: una que otra réplica ya cambió se omite.
func (s *Service) ExpireDue(ctx context.Context) (int, error) {
	now := s.now()
	due, err := s.d.Repo.DuePending(ctx, now, expireBatch)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range due {
		if err := b.Transition(domain.StatusExpired, domain.RoleSystem, now); err != nil {
			continue
		}
		saved, err := s.d.Repo.Update(ctx, b)
		if errors.Is(err, domain.ErrVersionConflict) {
			continue
		}
		if err != nil {
			return n, err
		}
		n++
		s.notify(ctx, saved.RequesterID, "booking.expired", "Solicitud vencida", "Tu solicitud de "+saved.Item.Title+" venció sin respuesta.", saved)
	}
	return n, nil
}
