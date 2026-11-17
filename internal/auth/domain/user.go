// Package domain contiene la entidad User y las reglas de la cuenta (spec 003).
// Solo importa la biblioteca estándar y los errores de dominio compartidos
// (Art. III).
package domain

import (
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Role es el rol de una cuenta (RF-10).
type Role string

// Roles válidos.
const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Status es el estado de una cuenta.
type Status string

// Estados de una cuenta. Una cuenta dada de baja no inicia sesión (RF-08).
const (
	StatusActive      Status = "active"
	StatusDeactivated Status = "deactivated"
)

// DeletedDisplayName es el nombre que muestra una cuenta dada de baja (RF-09).
const DeletedDisplayName = "Cuenta eliminada"

// Límites de los campos (RF-03, RF-08).
const (
	MaxEmailLen       = 254
	MinPasswordLen    = 8
	MaxPasswordLen    = 72
	MinDisplayNameLen = 2
	MaxDisplayNameLen = 40
)

// User es una cuenta de VAIVÉN. Una misma cuenta puede publicar y solicitar.
type User struct {
	ID              string
	Email           string // normalizado; anonimizado tras la baja
	PasswordHash    string // argon2id en formato PHC; nunca sale del backend
	DisplayName     string
	Phone           *string
	Role            Role
	Status          Status
	AcceptedTermsAt time.Time
	RatingSum       int64
	RatingCount     int64
	SuspendedUntil  *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int64
	DeactivatedAt   *time.Time
}

// IsActive indica si la cuenta puede iniciar sesión.
func (u User) IsActive() bool { return u.Status == StatusActive }

// IsSuspended indica si la cuenta está suspendida en el instante now (RF-13).
func (u User) IsSuspended(now time.Time) bool {
	return u.SuspendedUntil != nil && now.Before(*u.SuspendedUntil)
}

// RatingAverage es el promedio de calificaciones, o nil si no tiene (RF-09).
func (u User) RatingAverage() *float64 {
	if u.RatingCount <= 0 {
		return nil
	}
	avg := float64(u.RatingSum) / float64(u.RatingCount)
	return &avg
}

// PublicUser es el perfil que ve cualquier persona autenticada (RF-09). Nunca
// lleva correo ni teléfono (Art. VI.4).
type PublicUser struct {
	ID            string
	DisplayName   string
	RatingAverage *float64
	RatingCount   int64
	MemberSince   time.Time
}

// Public devuelve el perfil público; una cuenta dada de baja se muestra como
// «Cuenta eliminada».
func (u User) Public() PublicUser {
	name := u.DisplayName
	if !u.IsActive() {
		name = DeletedDisplayName
	}
	return PublicUser{
		ID:            u.ID,
		DisplayName:   name,
		RatingAverage: u.RatingAverage(),
		RatingCount:   u.RatingCount,
		MemberSince:   u.CreatedAt,
	}
}

// NormalizeEmail quita espacios y pasa a minúsculas (RF-02).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NormalizeDisplayName quita espacios al inicio y al final.
func NormalizeDisplayName(name string) string {
	return strings.TrimSpace(name)
}

// AnonymizedEmail es el correo que reemplaza al original tras la baja: único,
// no enrutable (dominio .invalid, RFC 2606) y sin datos de la persona. Libera
// el correo original para una cuenta nueva.
func AnonymizedEmail(userID string) string {
	return "deleted+" + userID + "@vaiven.invalid"
}

var phonePattern = regexp.MustCompile(`^\+?[0-9]{10,15}$`)

// ValidateEmail revisa un correo ya normalizado.
func ValidateEmail(email string) []shared.Detail {
	if email == "" {
		return []shared.Detail{{Field: "email", Issue: "es obligatorio"}}
	}
	if len(email) > MaxEmailLen {
		return []shared.Detail{{Field: "email", Issue: "es demasiado largo"}}
	}
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || !strings.Contains(email[strings.LastIndex(email, "@"):], ".") {
		return []shared.Detail{{Field: "email", Issue: "formato inválido"}}
	}
	return nil
}

// ValidatePassword exige de 8 a 72 caracteres (RF-03).
func ValidatePassword(password string) []shared.Detail {
	n := utf8.RuneCountInString(password)
	if n < MinPasswordLen || n > MaxPasswordLen {
		return []shared.Detail{{Field: "password", Issue: "debe tener de 8 a 72 caracteres"}}
	}
	return nil
}

// ValidateDisplayName exige de 2 a 40 caracteres (RF-08), ya sin espacios en
// los extremos.
func ValidateDisplayName(name string) []shared.Detail {
	n := utf8.RuneCountInString(name)
	if n < MinDisplayNameLen || n > MaxDisplayNameLen {
		return []shared.Detail{{Field: "displayName", Issue: "debe tener de 2 a 40 caracteres"}}
	}
	return nil
}

// ValidatePhone acepta de 10 a 15 dígitos con «+» opcional.
func ValidatePhone(phone string) []shared.Detail {
	if !phonePattern.MatchString(phone) {
		return []shared.Detail{{Field: "phone", Issue: "debe tener de 10 a 15 dígitos"}}
	}
	return nil
}
