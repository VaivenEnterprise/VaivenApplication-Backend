package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// Tokens son las credenciales que recibe el cliente (plan §2).
type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
}

// AuthResult es la respuesta de registro e inicio de sesión.
type AuthResult struct {
	User   domain.User
	Tokens Tokens
}

// Deps son las dependencias del Service.
type Deps struct {
	Users       Users
	Refresh     RefreshTokens
	Hasher      PasswordHasher
	Access      AccessTokens
	NewRefresh  RefreshGenerator
	HashRefresh RefreshHasher
	Bookings    ActiveBookingChecker // nil = NoActiveBookings
	RefreshTTL  time.Duration
	Now         func() time.Time // nil = time.Now
	NewFamilyID func() (string, error)
}

// Service implementa los casos de uso de la cuenta.
type Service struct {
	d Deps
}

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Users == nil || d.Refresh == nil || d.Hasher == nil || d.Access == nil ||
		d.NewRefresh == nil || d.HashRefresh == nil || d.RefreshTTL <= 0 {
		return nil, errors.New("auth: faltan dependencias del servicio")
	}
	if d.Bookings == nil {
		d.Bookings = NoActiveBookings{}
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.NewFamilyID == nil {
		d.NewFamilyID = randomID
	}
	// El señuelo lo calcula el Hasher al crearse (fuera de cualquier
	// petición); sin él no se puede igualar el tiempo del login.
	if d.Hasher.DecoyHash() == "" {
		return nil, errors.New("auth: el hasher no tiene hash señuelo")
	}
	return &Service{d: d}, nil
}

// now devuelve la hora en UTC truncada a milisegundos (precisión de MongoDB).
func (s *Service) now() time.Time {
	return s.d.Now().UTC().Truncate(time.Millisecond)
}

// issueSession emite un token de acceso y un refresh token de la familia
// indicada (vacía = familia nueva, es decir, un inicio de sesión nuevo).
func (s *Service) issueSession(ctx context.Context, u domain.User, familyID string) (Tokens, error) {
	if familyID == "" {
		id, err := s.d.NewFamilyID()
		if err != nil {
			return Tokens{}, fmt.Errorf("auth: familia de sesión: %w", err)
		}
		familyID = id
	}
	access, err := s.d.Access.Issue(u.ID, string(u.Role))
	if err != nil {
		return Tokens{}, fmt.Errorf("auth: emitir token de acceso: %w", err)
	}
	refresh, hash, err := s.d.NewRefresh()
	if err != nil {
		return Tokens{}, fmt.Errorf("auth: emitir refresh token: %w", err)
	}
	now := s.now()
	if err := s.d.Refresh.Create(ctx, domain.RefreshToken{
		UserID:    u.ID,
		FamilyID:  familyID,
		TokenHash: hash,
		ExpiresAt: now.Add(s.d.RefreshTTL),
		CreatedAt: now,
	}); err != nil {
		return Tokens{}, fmt.Errorf("auth: guardar refresh token: %w", err)
	}
	return Tokens{AccessToken: access, RefreshToken: refresh, ExpiresIn: s.d.Access.TTL()}, nil
}

// checkPassword verifica la contraseña de u. Si u es nil (el correo no
// existe) verifica contra el hash señuelo del Hasher: el mismo cálculo, el
// mismo turno y los mismos errores que con una cuenta real, y nunca coincide.
// Así el tiempo y la respuesta no revelan si el correo tiene cuenta (RF-04).
// El señuelo se calculó al arrancar, no depende del contexto de la petición, y
// una petición cancelada no deja ningún estado.
func (s *Service) checkPassword(ctx context.Context, u *domain.User, password string) (bool, error) {
	encoded := s.d.Hasher.DecoyHash()
	if u != nil {
		encoded = u.PasswordHash
	}
	ok, err := s.d.Hasher.Verify(ctx, password, encoded)
	if err != nil {
		return false, fmt.Errorf("auth: verificar contraseña: %w", err)
	}
	return ok && u != nil, nil
}

// randomID devuelve 16 bytes aleatorios en hexadecimal.
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
