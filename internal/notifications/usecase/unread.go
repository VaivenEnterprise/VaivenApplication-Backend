package usecase

import "context"

// UnreadCount devuelve cuántos avisos sin leer tiene userID (RF-03).
func (s *Service) UnreadCount(ctx context.Context, userID string) (int64, error) {
	return s.d.Repo.CountUnread(ctx, userID)
}
