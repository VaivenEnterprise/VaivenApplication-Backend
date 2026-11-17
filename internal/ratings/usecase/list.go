package usecase

import (
	"context"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
)

// List devuelve las calificaciones recibidas por rateeID, las más recientes
// primero (RF-04). limit 0 usa el valor por omisión; after es el id de la
// última calificación de la página anterior ("" = desde el inicio). Una
// persona que no existe responde 404.
func (s *Service) List(ctx context.Context, rateeID, after string, limit int) (Page, error) {
	if limit <= 0 {
		limit = DefaultPage
	}
	if limit > MaxPage {
		limit = MaxPage
	}
	profile, err := s.d.Users.PublicUsers(ctx, []string{rateeID})
	if err != nil {
		return Page{}, fmt.Errorf("ratings: leer perfil: %w", err)
	}
	if _, ok := profile[rateeID]; !ok {
		return Page{}, domain.ErrUserNotFound
	}
	// Se pide uno de más para saber si hay otra página.
	rows, err := s.d.Repo.ListByRatee(ctx, rateeID, after, limit+1)
	if err != nil {
		return Page{}, err
	}
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		id := rows[len(rows)-1].ID
		next = &id
	}
	names, err := s.names(ctx, rows)
	if err != nil {
		return Page{}, err
	}
	out := make([]RatingView, 0, len(rows))
	for _, r := range rows {
		name := DeletedName
		if n, ok := names[r.RaterID]; ok {
			name = n
		}
		out = append(out, RatingView{Rating: r, RaterName: name})
	}
	return Page{Items: out, Next: next}, nil
}

// names lee en un solo lote los nombres visibles de quienes calificaron.
func (s *Service) names(ctx context.Context, rows []domain.Rating) (map[string]string, error) {
	seen := map[string]bool{}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		if !seen[r.RaterID] {
			seen[r.RaterID] = true
			ids = append(ids, r.RaterID)
		}
	}
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	m, err := s.d.Users.PublicUsers(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("ratings: leer perfiles: %w", err)
	}
	for id, u := range m {
		if u.DisplayName != "" {
			out[id] = u.DisplayName
		}
	}
	return out, nil
}
