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
	s.upgradeHash(ctx, found, in.Password)
	return AuthResult{User: *found, Tokens: tokens}, nil
}

// upgradeHash recalcula con los parámetros actuales el hash de una cuenta que
// acaba de iniciar sesión con un hash antiguo (auditoría 2026-11-07, N2). Solo
// ocurre con la contraseña correcta, así que no añade un canal de tiempo para
// intentos fallidos. Es una mejora oportunista: si falla (sin turno, base de
// datos), el login ya fue correcto y se reintenta en el próximo inicio de
// sesión; el hash anterior sigue siendo válido.
func (s *Service) upgradeHash(ctx context.Context, u *domain.User, password string) {
	if !s.d.Hasher.NeedsRehash(u.PasswordHash) {
		return
	}
	hash, err := s.d.Hasher.Hash(ctx, password)
	if err != nil {
		return
	}
	if err := s.d.Users.UpdatePasswordHash(ctx, u.ID, hash, s.now()); err != nil {
		return
	}
	u.PasswordHash = hash
}
