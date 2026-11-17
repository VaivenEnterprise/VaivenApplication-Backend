package usecase

import (
	"context"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// Deactivate da de baja la cuenta (RF-08, POL-17) tras confirmar la contraseña:
// anonimiza correo, nombre y teléfono, revoca todas las sesiones y la cuenta
// deja de iniciar sesión. El correo original queda libre. Con reservas activas
// devuelve ErrActiveBookings (CA-05).
func (s *Service) Deactivate(ctx context.Context, userID, password string) error {
	u, err := s.Me(ctx, userID)
	if err != nil {
		return err
	}
	ok, err := s.checkPassword(ctx, &u, password)
	if err != nil {
		return fmt.Errorf("auth: baja: %w", err)
	}
	if !ok {
		return domain.ErrWrongPassword
	}
	active, err := s.d.Bookings.HasActiveBookings(ctx, u.ID)
	if err != nil {
		return fmt.Errorf("auth: baja: reservas activas: %w", err)
	}
	if active {
		return domain.ErrActiveBookings
	}

	now := s.now()
	if err := s.d.Users.Deactivate(ctx, u.ID, domain.AnonymizedEmail(u.ID), now); err != nil {
		return fmt.Errorf("auth: baja: %w", err)
	}
	if err := s.d.Refresh.RevokeAllForUser(ctx, u.ID, now); err != nil {
		return fmt.Errorf("auth: baja: revocar sesiones: %w", err)
	}
	return nil
}
