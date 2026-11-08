package usecase

import (
	"context"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// LoginInput son las credenciales de inicio de sesión.
type LoginInput struct {
	Email    string
	Password string
}

// Login abre una sesión nueva (RF-04). Correo inexistente, contraseña
// incorrecta o cuenta dada de baja devuelven el mismo ErrInvalidCredentials,
// y en tiempo parecido, para no revelar si el correo tiene cuenta.
func (s *Service) Login(ctx context.Context, in LoginInput) (AuthResult, error) {
	email := domain.NormalizeEmail(in.Email)
	if email == "" || in.Password == "" || len(in.Password) > 4*domain.MaxPasswordLen {
		return AuthResult{}, domain.ErrInvalidCredentials
	}

	var found *domain.User
	u, err := s.d.Users.ByEmail(ctx, email)
	switch {
	case err == nil:
		found = &u
	case domain.Is(err, domain.ErrUserNotFound):
	default:
		return AuthResult{}, fmt.Errorf("auth: login: %w", err)
	}

	ok, err := s.checkPassword(ctx, found, in.Password)
	if err != nil {
		return AuthResult{}, fmt.Errorf("auth: login: %w", err)
	}
	if !ok || !found.IsActive() {
		return AuthResult{}, domain.ErrInvalidCredentials
	}

	tokens, err := s.issueSession(ctx, *found, "")
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{User: *found, Tokens: tokens}, nil
}
