package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

const (
	ownerID     = "6730f1c2a9b3e4d5f6a7b8c1"
	requesterID = "6730f1c2a9b3e4d5f6a7b8c2"
	strangerID  = "6730f1c2a9b3e4d5f6a7b8c3"
)

var (
	loc  = mustLoc()
	done = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)
)

func mustLoc() *time.Location {
	l, err := bookings.Location()
	if err != nil {
		panic(err)
	}
	return l
}

func booking(status bookings.Status) bookings.Booking {
	b := bookings.Booking{
		ID: "6731a0000000000000000001", OwnerID: ownerID, RequesterID: requesterID,
		Start: bookings.MustDate("2026-11-18"), End: bookings.MustDate("2026-11-20"),
		Status: status,
	}
	if status == bookings.StatusCompleted {
		b.History = []bookings.HistoryEntry{{Status: bookings.StatusCompleted, At: done, By: bookings.RoleSystem}}
	}
	return b
}

func fieldsOf(err error) string {
	var de *shared.Error
	if !errors.As(err, &de) {
		return ""
	}
	var out []string
	for _, d := range de.Details {
		out = append(out, d.Field)
	}
	return strings.Join(out, ",")
}

func TestReasonStatusOutcomeValid(t *testing.T) {
	for _, r := range []Reason{ReasonNoShow, ReasonLateReturn, ReasonDamaged, ReasonNotReturned, ReasonPaymentDispute, ReasonOther} {
		if !r.Valid() {
			t.Errorf("%s debería ser válido", r)
		}
	}
	if Reason("robo").Valid() || Reason("").Valid() {
		t.Error("motivo desconocido no es válido")
	}
	if !StatusOpen.Valid() || !StatusClosed.Valid() || Status("pending").Valid() {
		t.Error("solo hay dos estados: open y closed (RF-06)")
	}
	if !OutcomeNone.Valid() || !OutcomeMonetaryAdjustment.Valid() || !OutcomeRestriction.Valid() || Outcome("x").Valid() {
		t.Error("resultados inválidos")
	}
}

func TestCanOpen(t *testing.T) {
	afterStart := time.Date(2026, 11, 18, 12, 0, 0, 0, loc)
	beforeStart := time.Date(2026, 11, 17, 23, 59, 0, 0, loc)
	tests := []struct {
		name   string
		b      bookings.Booking
		user   string
		reason Reason
		open   int
		now    time.Time
		want   error
	}{
		{"aceptada, daño", booking(bookings.StatusAccepted), ownerID, ReasonDamaged, 0, afterStart, nil},
		{"en curso, tardía", booking(bookings.StatusInProgress), requesterID, ReasonLateReturn, 2, afterStart, nil},
		{"completada dentro de 72 h", booking(bookings.StatusCompleted), ownerID, ReasonDamaged, 0, done.Add(71 * time.Hour), nil},
		{"completada justo a las 72 h", booking(bookings.StatusCompleted), ownerID, ReasonDamaged, 0, done.Add(72 * time.Hour), ErrWindowClosed},
		{"completada sin fecha", func() bookings.Booking { b := booking(bookings.StatusCompleted); b.History = nil; return b }(), ownerID, ReasonOther, 0, done, ErrWindowClosed},
		{"ajeno", booking(bookings.StatusAccepted), strangerID, ReasonOther, 0, afterStart, bookings.ErrNotFound},
		{"pendiente", booking(bookings.StatusPending), ownerID, ReasonOther, 0, afterStart, ErrBookingStatus},
		{"cancelada", booking(bookings.StatusCancelled), ownerID, ReasonOther, 0, afterStart, ErrBookingStatus},
		{"rechazada", booking(bookings.StatusRejected), ownerID, ReasonOther, 0, afterStart, ErrBookingStatus},
		{"no_show antes del inicio", booking(bookings.StatusAccepted), ownerID, ReasonNoShow, 0, beforeStart, ErrTooEarly},
		{"no_show el día de inicio", booking(bookings.StatusAccepted), ownerID, ReasonNoShow, 0, afterStart, nil},
		{"otro motivo antes del inicio", booking(bookings.StatusAccepted), ownerID, ReasonOther, 0, beforeStart, nil},
		{"3 abiertas ya es el máximo (CA-08)", booking(bookings.StatusAccepted), ownerID, ReasonOther, 3, afterStart, ErrTooManyOpen},
		{"más de 3 abiertas", booking(bookings.StatusAccepted), ownerID, ReasonOther, 4, afterStart, ErrTooManyOpen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanOpen(tt.b, tt.user, tt.reason, tt.open, tt.now, loc); !errors.Is(got, tt.want) {
				t.Errorf("CanOpen = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateInput(t *testing.T) {
	ok := strings.Repeat("a", 10)
	tests := []struct {
		name     string
		reason   Reason
		desc     string
		evidence int
		want     string // campos inválidos; "" = válido
	}{
		{"válido", ReasonDamaged, "  " + ok + "  ", 0, ""},
		{"5 imágenes", ReasonOther, ok, 5, ""},
		{"1000 caracteres con acentos", ReasonOther, strings.Repeat("é", 1000), 1, ""},
		{"motivo inválido", Reason("x"), ok, 0, "reason"},
		{"descripción corta", ReasonOther, "corta", 0, "description"},
		{"descripción solo espacios", ReasonOther, strings.Repeat(" ", 20), 0, "description"},
		{"descripción larga", ReasonOther, strings.Repeat("a", 1001), 0, "description"},
		{"6 imágenes", ReasonOther, ok, 6, "evidence"},
		{"todo mal", Reason(""), "", 9, "reason,description,evidence"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, err := ValidateInput(tt.reason, tt.desc, tt.evidence)
			if tt.want == "" {
				if err != nil || text != strings.TrimSpace(tt.desc) {
					t.Fatalf("text=%q err=%v", text, err)
				}
				return
			}
			if got := fieldsOf(err); got != tt.want {
				t.Errorf("campos = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	b := booking(bookings.StatusAccepted)
	imgs := []images.Image{{PublicID: "p1"}}

	i, err := New("i1", b, requesterID, ReasonDamaged, "descripción larga", imgs, done)
	if err != nil {
		t.Fatal(err)
	}
	if i.ReporterID != requesterID || i.ReportedID != ownerID || i.Status != StatusOpen || i.Version != 1 {
		t.Errorf("incidencia mal armada: %+v", i)
	}
	if len(i.Evidence) != 1 || i.Notes == nil || len(i.Notes) != 0 || i.Final != nil || i.ClosedAt != nil {
		t.Errorf("evidencia/notas/cierre mal iniciados: %+v", i)
	}

	i2, err := New("i2", b, ownerID, ReasonOther, "descripción larga", nil, done)
	if err != nil || i2.ReportedID != requesterID || i2.Evidence == nil {
		t.Errorf("i2 = %+v, err = %v", i2, err)
	}
	if _, err := New("i3", b, strangerID, ReasonOther, "descripción larga", nil, done); !errors.Is(err, bookings.ErrNotFound) {
		t.Errorf("ajeno: err = %v", err)
	}
}

func TestIsPartyAndVisibleNotes(t *testing.T) {
	i := Incident{ReporterID: requesterID, ReportedID: ownerID, Notes: []Note{
		{Text: "interna", VisibleToParties: false},
		{Text: "pública", VisibleToParties: true},
	}}
	if !i.IsParty(requesterID) || !i.IsParty(ownerID) || i.IsParty(strangerID) || i.IsParty("") {
		t.Error("IsParty incorrecto")
	}
	v := i.VisibleNotes()
	if len(v) != 1 || v[0].Text != "pública" {
		t.Errorf("VisibleNotes = %+v", v)
	}
	if got := (Incident{}).VisibleNotes(); got == nil || len(got) != 0 {
		t.Errorf("sin notas debe dar lista vacía no nula: %v", got)
	}
}

func TestNotes(t *testing.T) {
	open := Incident{Status: StatusOpen}
	if err := CanAddNote(open); err != nil {
		t.Fatal(err)
	}
	if err := CanAddNote(Incident{Status: StatusClosed}); !errors.Is(err, ErrNotOpen) {
		t.Errorf("cerrada: %v", err)
	}
	full := Incident{Status: StatusOpen, Notes: make([]Note, MaxNotes)}
	if err := CanAddNote(full); !errors.Is(err, ErrTooManyNotes) {
		t.Errorf("11.ª nota: %v", err)
	}
	nine := Incident{Status: StatusOpen, Notes: make([]Note, MaxNotes-1)}
	if err := CanAddNote(nine); err != nil {
		t.Errorf("la nota 10 sí cabe: %v", err)
	}

	n, err := NewNote("  hola equipo ", true, "adm", done)
	if err != nil || n.Text != "hola equipo" || !n.VisibleToParties || n.ByAdminID != "adm" || !n.At.Equal(done) {
		t.Errorf("n = %+v, err = %v", n, err)
	}
	for _, bad := range []string{"", "  ", "ab", strings.Repeat("x", 1001)} {
		if _, err := NewNote(bad, false, "adm", done); fieldsOf(err) != "text" {
			t.Errorf("NewNote(%q) = %v", bad, err)
		}
	}
}

func TestClose(t *testing.T) {
	f, err := NewFinalResponse("  Resuelto con ambas partes. ", OutcomeNone, "adm", done)
	if err != nil || f.Text != "Resuelto con ambas partes." || f.Outcome != OutcomeNone {
		t.Fatalf("f = %+v, err = %v", f, err)
	}
	for _, c := range []struct {
		text    string
		outcome Outcome
		want    string
	}{
		{"corta", OutcomeNone, ""},
		{"abc", OutcomeNone, "finalResponse"},
		{strings.Repeat("x", 1001), OutcomeNone, "finalResponse"},
		{"respuesta válida", Outcome("x"), "outcome"},
		{"", Outcome(""), "finalResponse,outcome"},
	} {
		_, err := NewFinalResponse(c.text, c.outcome, "adm", done)
		if got := fieldsOf(err); got != c.want {
			t.Errorf("NewFinalResponse(%q,%q) campos = %q, want %q", c.text, c.outcome, got, c.want)
		}
	}

	open := Incident{ID: "i1", Status: StatusOpen, Version: 3}
	closed, err := Close(open, f)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != StatusClosed || closed.Final == nil || closed.Final.Text != f.Text ||
		closed.ClosedAt == nil || !closed.ClosedAt.Equal(done) || !closed.UpdatedAt.Equal(done) {
		t.Errorf("cierre mal aplicado: %+v", closed)
	}
	if open.Status != StatusOpen || open.Final != nil {
		t.Error("Close no debe mutar el original")
	}
	if _, err := Close(closed, f); !errors.Is(err, ErrNotOpen) {
		t.Errorf("cerrar dos veces: %v", err)
	}
	if err := CanClose(closed); !errors.Is(err, ErrNotOpen) {
		t.Errorf("CanClose cerrada: %v", err)
	}
}
