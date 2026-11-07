// Package usecase contiene los casos de uso de la cuenta (spec 003): registro,
// inicio y cierre de sesión, renovación, perfil y baja. Declara los puertos que
// necesita (Art. III.3); los adaptadores de MongoDB y security los implementan.
package usecase

import (
	"context"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// Users guarda y lee cuentas.
type Users interface {
	Create(ctx context.Context, u domain.User) (domain.User, error)
	ByEmail(ctx context.Context, email string) (domain.User, error)
	ByID(ctx context.Context, id string) (domain.User, error)
	// UpdateProfile aplica los cambios no nulos y devuelve la cuenta nueva.
	UpdateProfile(ctx context.Context, id string, p ProfileChanges, now time.Time) (domain.User, error)
	// Deactivate da de baja la cuenta y anonimiza correo, nombre y teléfono.
	Deactivate(ctx context.Context, id, anonymizedEmail string, now time.Time) error
}

// ProfileChanges son los cambios de PATCH /me. Un campo nil no cambia;
// ClearPhone borra el teléfono.
type ProfileChanges struct {
	DisplayName *string
	Phone       *string
	ClearPhone  bool
}

// RefreshTokens guarda los refresh tokens (solo su hash).
type RefreshTokens interface {
	Create(ctx context.Context, t domain.RefreshToken) error
	ByHash(ctx context.Context, hash string) (domain.RefreshToken, error)
	// MarkUsed marca el token como usado de forma atómica; false si ya estaba
	// usado o revocado.
	MarkUsed(ctx context.Context, id string, at time.Time) (bool, error)
	RevokeFamily(ctx context.Context, familyID string, at time.Time) error
	RevokeAllForUser(ctx context.Context, userID string, at time.Time) error
}

// PasswordHasher calcula y verifica hashes de contraseña (argon2id).
type PasswordHasher interface {
	Hash(ctx context.Context, password string) (string, error)
	// Verify devuelve false, nil si la contraseña no coincide.
	Verify(ctx context.Context, password, encoded string) (bool, error)
	// DecoyHash es un hash válido de un valor desconocido, con los parámetros
	// actuales, calculado al crear el hasher (no por petición). Login verifica
	// contra él cuando el correo no existe.
	DecoyHash() string
}

// AccessTokens emite tokens de acceso (JWT).
type AccessTokens interface {
	Issue(userID, role string) (string, error)
	TTL() time.Duration
}

// RefreshGenerator genera un refresh token opaco y su hash.
type RefreshGenerator func() (token, hash string, err error)

// RefreshHasher calcula el hash con el que se guarda un refresh token.
type RefreshHasher func(token string) string

// ActiveBookingChecker indica si la persona tiene reservas activas (plan P-10).
// Hasta la spec 007 se usa NoActiveBookings.
type ActiveBookingChecker interface {
	HasActiveBookings(ctx context.Context, userID string) (bool, error)
}

// NoActiveBookings es la implementación vacía: nunca hay reservas activas.
type NoActiveBookings struct{}

// HasActiveBookings siempre devuelve false.
func (NoActiveBookings) HasActiveBookings(context.Context, string) (bool, error) {
	return false, nil
}
