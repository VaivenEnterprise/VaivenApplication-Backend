package usecase

import (
	"context"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// maxRefreshTokenLen acota el token recibido antes de calcular su hash.
const maxRefreshTokenLen = 128

// Refresh canjea un refresh token por un par nuevo de la misma familia (RF-05,
// plan P-04). El token canjeado queda usado; si vuelve a llegar, se revoca
// toda la familia y se responde ErrInvalidSession (CA-03).
func (s *Service) Refresh(ctx context.Context, token string) (Tokens, error) {
	rt, err := s.findRefresh(ctx, token)
	if err != nil {
		return Tokens{}, err
	}
	now := s.now()

	if rt.Reused() {
		return Tokens{}, s.revokeFamily(ctx, rt.FamilyID)
	}
	if !rt.Usable(now) {
		return Tokens{}, domain.ErrInvalidSession
	}
	ok, err := s.d.Refresh.MarkUsed(ctx, rt.ID, now)
	if err != nil {
		return Tokens{}, fmt.Errorf("auth: refresh: %w", err)
	}
	if !ok {
		// Otra petición lo canjeó al mismo tiempo: se trata como reutilización.
		return Tokens{}, s.revokeFamily(ctx, rt.FamilyID)
	}

	u, err := s.d.Users.ByID(ctx, rt.UserID)
	if err != nil && !domain.Is(err, domain.ErrUserNotFound) {
		return Tokens{}, fmt.Errorf("auth: refresh: %w", err)
	}
	if err != nil || !u.IsActive() {
		return Tokens{}, s.revokeFamily(ctx, rt.FamilyID)
	}
	return s.issueSession(ctx, u, rt.FamilyID)
}

// findRefresh busca un refresh token por su hash. Un token vacío, demasiado
// largo o desconocido es ErrInvalidSession.
func (s *Service) findRefresh(ctx context.Context, token string) (domain.RefreshToken, error) {
	if token == "" || len(token) > maxRefreshTokenLen {
		return domain.RefreshToken{}, domain.ErrInvalidSession
	}
	rt, err := s.d.Refresh.ByHash(ctx, s.d.HashRefresh(token))
	if err != nil {
		if domain.Is(err, domain.ErrInvalidSession) {
			return domain.RefreshToken{}, domain.ErrInvalidSession
		}
		return domain.RefreshToken{}, fmt.Errorf("auth: buscar refresh token: %w", err)
	}
	return rt, nil
}

// revokeFamily revoca la familia y devuelve ErrInvalidSession, o el error de
// la revocación si falló (para no ocultar que la familia sigue abierta).
func (s *Service) revokeFamily(ctx context.Context, familyID string) error {
	if err := s.d.Refresh.RevokeFamily(ctx, familyID, s.now()); err != nil {
		return fmt.Errorf("auth: revocar familia: %w", err)
	}
	return domain.ErrInvalidSession
}
