package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
)

// RegisterToken registra el token de Expo de userID o lo reasigna si estaba a
// nombre de otra persona (RF-04): un dispositivo recibe los avisos de quien
// tiene la sesión abierta.
func (s *Service) RegisterToken(ctx context.Context, userID, token, platform string) error {
	t, err := domain.NormalizeToken(token)
	if err != nil {
		return err
	}
	if err := domain.ValidatePlatform(platform); err != nil {
		return err
	}
	now := s.now()
	return s.d.Tokens.Upsert(ctx, domain.PushToken{Token: t, UserID: userID, Platform: platform, CreatedAt: now, LastSeenAt: now})
}

// UnregisterToken elimina el token de userID (al cerrar sesión, RF-04). Es
// idempotente y no toca tokens de otras personas.
func (s *Service) UnregisterToken(ctx context.Context, userID, token string) error {
	t, err := domain.NormalizeToken(token)
	if err != nil {
		return err
	}
	return s.d.Tokens.Delete(ctx, userID, t)
}
