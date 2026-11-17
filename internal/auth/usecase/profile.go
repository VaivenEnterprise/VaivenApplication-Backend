package usecase

import (
	"context"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Me devuelve la cuenta de la persona autenticada. Si la cuenta ya no existe
// o se dio de baja, el token de acceso ya no sirve: ErrInvalidSession.
func (s *Service) Me(ctx context.Context, userID string) (domain.User, error) {
	u, err := s.d.Users.ByID(ctx, userID)
	if err != nil {
		if domain.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.ErrInvalidSession
		}
		return domain.User{}, fmt.Errorf("auth: me: %w", err)
	}
	if !u.IsActive() {
		return domain.User{}, domain.ErrInvalidSession
	}
	return u, nil
}

// UpdateProfile cambia el nombre y el teléfono (RF-08). Solo aplica los campos
// presentes; ClearPhone borra el teléfono.
func (s *Service) UpdateProfile(ctx context.Context, userID string, in ProfileChanges) (domain.User, error) {
	var details []shared.Detail
	if in.DisplayName == nil && in.Phone == nil && !in.ClearPhone {
		details = append(details, shared.Detail{Field: "body", Issue: "envía displayName o phone"})
	}
	if in.DisplayName != nil {
		name := domain.NormalizeDisplayName(*in.DisplayName)
		in.DisplayName = &name
		details = append(details, domain.ValidateDisplayName(name)...)
	}
	if in.Phone != nil && !in.ClearPhone {
		details = append(details, domain.ValidatePhone(*in.Phone)...)
	}
	if len(details) > 0 {
		return domain.User{}, shared.NewValidation(messageInvalidData, details...)
	}

	u, err := s.d.Users.UpdateProfile(ctx, userID, in, s.now())
	if err != nil {
		if domain.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.ErrInvalidSession
		}
		return domain.User{}, fmt.Errorf("auth: actualizar perfil: %w", err)
	}
	return u, nil
}

// PublicProfile devuelve el perfil público de cualquier cuenta, también de una
// dada de baja («Cuenta eliminada», RF-09).
func (s *Service) PublicProfile(ctx context.Context, id string) (domain.PublicUser, error) {
	u, err := s.d.Users.ByID(ctx, id)
	if err != nil {
		if domain.Is(err, domain.ErrUserNotFound) {
			return domain.PublicUser{}, domain.ErrUserNotFound
		}
		return domain.PublicUser{}, fmt.Errorf("auth: perfil público: %w", err)
	}
	return u.Public(), nil
}
