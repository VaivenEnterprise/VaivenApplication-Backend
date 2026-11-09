package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

const (
	ownerID     = "6730f1c2a9b3e4d5f6a7b8c1"
	requesterID = "6730f1c2a9b3e4d5f6a7b8c2"
	strangerID  = "6730f1c2a9b3e4d5f6a7b8c3"
)

var done = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

func completedBooking() bookings.Booking {
	return bookings.Booking{
		ID: "6731a0000000000000000001", OwnerID: ownerID, RequesterID: requesterID,
		Status: bookings.StatusCompleted,
		History: []bookings.HistoryEntry{
			{Status: bookings.StatusPending, At: done.Add(-96 * time.Hour), By: bookings.RoleRequester},
			{Status: bookings.StatusAccepted, At: done.Add(-90 * time.Hour), By: bookings.RoleOwner},
			{Status: bookings.StatusInProgress, At: done.Add(-48 * time.Hour), By: bookings.RoleSystem},
			{Status: bookings.StatusCompleted, At: done, By: bookings.RoleSystem},
		},
	}
}

func str(s string) *string { return &s }

func TestValidateScore(t *testing.T) {
	for _, ok := range []int{1, 2, 3, 4, 5} {
		if err := ValidateScore(ok); err != nil {
			t.Errorf("ValidateScore(%d) = %v", ok, err)
		}
	}
	for _, bad := range []int{-1, 0, 6, 100} {
		err := ValidateScore(bad)
		var de *shared.Error
		if !errors.As(err, &de) || de.Code != shared.CodeValidation || de.Details[0].Field != "score" {
			t.Errorf("ValidateScore(%d) = %v, quería validation_failed sobre score", bad, err)
		}
	}
}

func TestNormalizeComment(t *testing.T) {
	long := strings.Repeat("á", 300)
	tests := []struct {
		name string
		in   *string
		want *string
		fail bool
	}{
		{"nil", nil, nil, false},
		{"vacío", str("   "), nil, false},
		{"recorta", str("  hola  "), str("hola"), false},
		{"300 caracteres con acentos", str(long), str(long), false},
		{"301 caracteres", str(long + "a"), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeComment(tt.in)
			if (err != nil) != tt.fail {
				t.Fatalf("error = %v, quería fallo=%v", err, tt.fail)
			}
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAverage(t *testing.T) {
	if Average(0, 0) != nil {
		t.Error("sin calificaciones el promedio debe ser nil")
	}
	cases := []struct {
		sum, count int64
		want       float64
	}{
		{5, 1, 5}, {9, 2, 4.5}, {14, 3, 4.7}, {13, 3, 4.3}, {4, 3, 1.3}, {1, 1, 1},
	}
	for _, c := range cases {
		got := Average(c.sum, c.count)
		if got == nil || *got != c.want {
			t.Errorf("Average(%d,%d) = %v, want %v", c.sum, c.count, got, c.want)
		}
	}
}

func TestCompletedAt(t *testing.T) {
	b := completedBooking()
	got, ok := CompletedAt(b)
	if !ok || !got.Equal(done) {
		t.Errorf("CompletedAt = %v,%v", got, ok)
	}
	b.History = b.History[:3]
	if _, ok := CompletedAt(b); ok {
		t.Error("sin entrada completed no hay fecha")
	}
}

func TestExpiresAt(t *testing.T) {
	if got, want := ExpiresAt(done), done.AddDate(0, 0, 14); !got.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", got, want)
	}
}

func TestCanRate(t *testing.T) {
	b := completedBooking()
	pending := completedBooking()
	pending.Status = bookings.StatusInProgress
	noHistory := completedBooking()
	noHistory.History = nil

	tests := []struct {
		name string
		b    bookings.Booking
		user string
		now  time.Time
		want error
	}{
		{"propietario en plazo", b, ownerID, done.Add(time.Hour), nil},
		{"solicitante en plazo", b, requesterID, done.Add(time.Hour), nil},
		{"último instante del plazo", b, ownerID, ExpiresAt(done).Add(-time.Nanosecond), nil},
		{"ajeno", b, strangerID, done.Add(time.Hour), bookings.ErrNotFound},
		{"sin sesión", b, "", done.Add(time.Hour), bookings.ErrNotFound},
		{"no completada", pending, ownerID, done, ErrNotCompleted},
		{"completada sin fecha", noHistory, ownerID, done, ErrNotCompleted},
		{"plazo vencido", b, ownerID, ExpiresAt(done), ErrWindowClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanRate(tt.b, tt.user, tt.now); !errors.Is(got, tt.want) {
				t.Errorf("CanRate = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	b := completedBooking()
	t.Run("el solicitante califica al propietario", func(t *testing.T) {
		r, err := New("r1", b, requesterID, 5, str(" gracias "), done.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if r.RateeID != ownerID || r.RatedRole != bookings.RoleOwner || r.RaterID != requesterID {
			t.Errorf("partes mal asignadas: %+v", r)
		}
		if r.Comment == nil || *r.Comment != "gracias" || r.Score != 5 || r.BookingID != b.ID {
			t.Errorf("datos mal guardados: %+v", r)
		}
	})
	t.Run("el propietario califica al solicitante", func(t *testing.T) {
		r, err := New("r2", b, ownerID, 3, nil, done)
		if err != nil {
			t.Fatal(err)
		}
		if r.RateeID != requesterID || r.RatedRole != bookings.RoleRequester || r.Comment != nil {
			t.Errorf("partes mal asignadas: %+v", r)
		}
	})
	t.Run("ajeno", func(t *testing.T) {
		if _, err := New("r3", b, strangerID, 5, nil, done); !errors.Is(err, bookings.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("puntaje inválido", func(t *testing.T) {
		if _, err := New("r4", b, ownerID, 0, nil, done); err == nil {
			t.Error("quería error de validación")
		}
	})
	t.Run("comentario largo", func(t *testing.T) {
		if _, err := New("r5", b, ownerID, 4, str(strings.Repeat("x", 301)), done); err == nil {
			t.Error("quería error de validación")
		}
	})
}
