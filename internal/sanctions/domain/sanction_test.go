package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

var now = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func fields(err error) []string {
	var de *shared.Error
	if !errors.As(err, &de) {
		return nil
	}
	var out []string
	for _, d := range de.Details {
		out = append(out, d.Field)
	}
	return out
}

func TestNew(t *testing.T) {
	future := now.Add(7 * 24 * time.Hour)
	tests := []struct {
		name       string
		typ        Type
		reason     string
		endsAt     *time.Time
		incidentID *string
		badFields  []string
	}{
		{"advertencia válida", TypeWarning, "Llegó tarde varias veces.", nil, nil, nil},
		{"suspensión válida con incidencia", TypeSuspension, "Daño reiterado a objetos.", &future, ptr("6731c0000000000000000001"), nil},
		{"tipo inválido", Type("ban"), "Motivo suficiente.", nil, nil, []string{"type"}},
		{"motivo corto", TypeWarning, " abc ", nil, nil, []string{"reason"}},
		{"motivo largo", TypeWarning, strings.Repeat("x", 501), nil, nil, []string{"reason"}},
		{"suspensión sin fecha", TypeSuspension, "Motivo suficiente.", nil, nil, []string{"endsAt"}},
		{"suspensión con fecha pasada", TypeSuspension, "Motivo suficiente.", ptr(now.Add(-time.Hour)), nil, []string{"endsAt"}},
		{"suspensión con fecha igual a ahora", TypeSuspension, "Motivo suficiente.", ptr(now), nil, []string{"endsAt"}},
		{"advertencia con fecha", TypeWarning, "Motivo suficiente.", &future, nil, []string{"endsAt"}},
		{"incidencia mal formada", TypeWarning, "Motivo suficiente.", nil, ptr("xyz"), []string{"incidentId"}},
		{"varios errores", Type(""), "", &future, ptr("A"), []string{"type", "reason", "incidentId"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New("s1", "6730f1c2a9b3e4d5f6a7b8c9", tt.typ, tt.reason, tt.endsAt, tt.incidentID, "admin1", now)
			if tt.badFields == nil {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				if s.ID != "s1" || s.UserID == "" || s.CreatedByAdminID != "admin1" || !s.CreatedAt.Equal(now) {
					t.Errorf("sanción mal armada: %+v", s)
				}
				if (tt.endsAt == nil) != (s.EndsAt == nil) {
					t.Errorf("endsAt = %v", s.EndsAt)
				}
				return
			}
			got := fields(err)
			if strings.Join(got, ",") != strings.Join(tt.badFields, ",") {
				t.Errorf("campos inválidos = %v, want %v (err %v)", got, tt.badFields, err)
			}
		})
	}
}

func TestNewTrimsReason(t *testing.T) {
	s, err := New("s1", "u", TypeWarning, "   Motivo suficiente.  ", nil, nil, "a", now)
	if err != nil || s.Reason != "Motivo suficiente." {
		t.Errorf("s = %+v, err = %v", s, err)
	}
}

func TestIsObjectID(t *testing.T) {
	for in, want := range map[string]bool{
		"6731c0000000000000000001": true, "": false, "6731C0000000000000000001": false,
		"6731c000000000000000000": false, "6731c00000000000000000zz": false,
	} {
		if got := IsObjectID(in); got != want {
			t.Errorf("IsObjectID(%q) = %v", in, got)
		}
	}
}
