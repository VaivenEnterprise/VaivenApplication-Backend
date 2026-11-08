package usecase

import (
	"context"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// Logout revoca la familia del refresh token (RF-06). Es idempotente: un token
// desconocido, vencido o ya revocado no es un error.
func (s *Service) Logout(ctx context.Context, token string) error {
	rt, err := s.findRefresh(ctx, token)
	if err != nil {
		if domain.Is(err, domain.ErrInvalidSession) {
			return nil
		}
		return err
	}
	if err := s.d.Refresh.RevokeFamily(ctx, rt.FamilyID, s.now()); err != nil {
		return fmt.Errorf("auth: logout: %w", err)
	}
	return nil
}
