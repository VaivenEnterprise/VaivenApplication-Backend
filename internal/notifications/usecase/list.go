package usecase

import "context"

// List devuelve los avisos de userID, los más recientes primero (RF-03). limit
// 0 usa el valor por omisión; after es el id del último aviso de la página
// anterior ("" = desde el inicio).
func (s *Service) List(ctx context.Context, userID, after string, limit int) (Page, error) {
	if limit <= 0 {
		limit = DefaultPage
	}
	if limit > MaxPage {
		limit = MaxPage
	}
	// Se pide uno de más para saber si hay otra página.
	rows, err := s.d.Repo.List(ctx, userID, after, limit+1)
	if err != nil {
		return Page{}, err
	}
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		id := rows[len(rows)-1].ID
		next = &id
	}
	return Page{Items: rows, Next: next}, nil
}
