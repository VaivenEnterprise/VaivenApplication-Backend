package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

func TestNormalize(t *testing.T) {
	if got := NormalizeEmail("  Ana.Lopez@Example.COM \t"); got != "ana.lopez@example.com" {
		t.Errorf("NormalizeEmail = %q", got)
	}
	if got := NormalizeDisplayName("  Ana  "); got != "Ana" {
		t.Errorf("NormalizeDisplayName = %q", got)
	}
	if got := AnonymizedEmail("abc"); got != "deleted+abc@vaiven.invalid" || ValidateEmail(got) != nil {
		t.Errorf("AnonymizedEmail = %q (debe ser un correo válido y no enrutable)", got)
	}
}

func TestValidateEmail(t *testing.T) {
	ok := []string{"ana@example.com", "a.b+c@sub.example.mx"}
	bad := []string{"", "ana", "ana@", "@example.com", "ana@example", "Ana <ana@example.com>", "a b@example.com", strings.Repeat("a", 250) + "@x.mx"}
	for _, e := range ok {
		if d := ValidateEmail(e); d != nil {
			t.Errorf("ValidateEmail(%q) = %v, want nil", e, d)
		}
	}
	for _, e := range bad {
		if d := ValidateEmail(e); len(d) != 1 || d[0].Field != "email" {
			t.Errorf("ValidateEmail(%q) = %v, want un detalle de email", e, d)
		}
	}
}

func TestValidateLengths(t *testing.T) {
	tests := []struct {
		name  string
		check func(string) []shared.Detail
		value string
		ok    bool
	}{
		{"password 7", ValidatePassword, "1234567", false},
		{"password 8", ValidatePassword, "12345678", true},
		{"password 72", ValidatePassword, strings.Repeat("a", 72), true},
		{"password 73", ValidatePassword, strings.Repeat("a", 73), false},
		{"password con acentos cuenta runas", ValidatePassword, "ñandúñandú", true},
		{"nombre 1", ValidateDisplayName, "A", false},
		{"nombre 2", ValidateDisplayName, "Al", true},
		{"nombre 40", ValidateDisplayName, strings.Repeat("ñ", 40), true},
		{"nombre 41", ValidateDisplayName, strings.Repeat("a", 41), false},
		{"teléfono 10", ValidatePhone, "5512345678", true},
		{"teléfono con +", ValidatePhone, "+525512345678", true},
		{"teléfono corto", ValidatePhone, "551234", false},
		{"teléfono con letras", ValidatePhone, "55-1234-5678", false},
		{"teléfono 16", ValidatePhone, "1234567890123456", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.check(tt.value) == nil; got != tt.ok {
				t.Errorf("válido = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestUserState(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	later, earlier := now.Add(time.Hour), now.Add(-time.Hour)
	u := User{ID: "u1", DisplayName: "Ana", Status: StatusActive, CreatedAt: earlier}

	if !u.IsActive() || u.IsSuspended(now) || u.RatingAverage() != nil {
		t.Errorf("cuenta nueva: activa=%v suspendida=%v promedio=%v", u.IsActive(), u.IsSuspended(now), u.RatingAverage())
	}
	u.SuspendedUntil = &later
	if !u.IsSuspended(now) {
		t.Error("suspendida hasta más tarde debe estar suspendida")
	}
	u.SuspendedUntil = &earlier
	if u.IsSuspended(now) {
		t.Error("una suspensión vencida no cuenta")
	}

	u.RatingSum, u.RatingCount = 9, 2
	p := u.Public()
	if p.ID != "u1" || p.DisplayName != "Ana" || p.RatingAverage == nil || *p.RatingAverage != 4.5 ||
		p.RatingCount != 2 || !p.MemberSince.Equal(earlier) {
		t.Errorf("Public = %+v", p)
	}

	u.Status = StatusDeactivated
	if u.IsActive() || u.Public().DisplayName != DeletedDisplayName {
		t.Errorf("baja: activa=%v nombre=%q", u.IsActive(), u.Public().DisplayName)
	}
}

func TestIs(t *testing.T) {
	if !Is(ErrEmailTaken, ErrEmailTaken) {
		t.Error("Is con el mismo error")
	}
	if !Is(ErrEmailTaken.Wrap(errors.New("E11000")), ErrEmailTaken) {
		t.Error("Is debe reconocer el error con causa (Wrap)")
	}
	if !Is(fmt.Errorf("repo: %w", ErrUserNotFound), ErrUserNotFound) {
		t.Error("Is debe reconocer el error envuelto con %w")
	}
	if Is(ErrInvalidCredentials, ErrInvalidSession) {
		t.Error("mismo código y distinto mensaje no son el mismo error")
	}
	if Is(errors.New("otro"), ErrEmailTaken) || Is(nil, ErrEmailTaken) {
		t.Error("un error ajeno no es de dominio")
	}
	// Todos los errores tienen un código del Art. IV y mensaje en español.
	for _, e := range []*shared.Error{ErrEmailTaken, ErrInvalidCredentials, ErrInvalidSession, ErrUserNotFound,
		ErrWrongPassword, ErrActiveBookings, ErrSuspended, ErrForbiddenRole} {
		if e.Code == "" || e.Message == "" {
			t.Errorf("error incompleto: %+v", e)
		}
	}
}
