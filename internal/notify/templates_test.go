package notify

import (
	"sort"
	"strings"
	"testing"
)

// rf02 son los 18 tipos de la spec 011 (RF-02), escritos aparte de las
// constantes para que la prueba detecte un tipo olvidado.
var rf02 = []string{
	"booking_requested", "booking_accepted", "booking_rejected", "booking_cancelled", "booking_expired",
	"message_received", "delivery_confirmed", "return_confirmed", "payment_reported", "payment_confirmed",
	"late_return", "booking_affected", "incident_opened", "incident_note", "incident_closed",
	"rating_received", "sanction_applied", "content_removed",
}

func TestEveryRF02TypeHasSpanishTemplate(t *testing.T) {
	if len(rf02) != 18 {
		t.Fatalf("RF-02 define 18 tipos, la lista tiene %d", len(rf02))
	}
	for _, typ := range rf02 {
		tpl, ok := TemplateFor(typ)
		if !ok {
			t.Errorf("falta plantilla para %q", typ)
			continue
		}
		if strings.TrimSpace(tpl.Title) == "" || strings.TrimSpace(tpl.Body) == "" {
			t.Errorf("%q: título o cuerpo vacío: %+v", typ, tpl)
		}
		if len(tpl.Title) > 60 {
			t.Errorf("%q: el título es largo para un push (%d)", typ, len(tpl.Title))
		}
		if !strings.HasSuffix(tpl.Body, ".") {
			t.Errorf("%q: el cuerpo debe ser una oración: %q", typ, tpl.Body)
		}
	}
}

func TestTemplatesCoverOnlyRF02(t *testing.T) {
	got := Types()
	sort.Strings(got)
	want := append([]string(nil), rf02...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tipos = %v, quiero exactamente %v", got, want)
	}
}

func TestTemplatesHaveNoPlaceholdersOrPersonalData(t *testing.T) { // Art. VI.5, plan P-04
	for typ, tpl := range templates {
		for _, s := range []string{tpl.Title, tpl.Body} {
			if strings.ContainsAny(s, "{}<>@") || strings.Contains(s, "%") {
				t.Errorf("%q: el texto no debe llevar parámetros ni datos: %q", typ, s)
			}
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"booking_requested", "booking_requested", true},
		{"booking.requested", "booking_requested", true}, // spec 007
		{"booking.accepted", "booking_accepted", true},
		{"Booking.Cancelled", "booking_cancelled", true},
		{"rating.received", "rating_received", true}, // spec 010
		{"incident.opened", "incident_opened", true},
		{"incident.note", "incident_note", true},
		{"incident.closed", "incident_closed", true},
		{"sanction.applied", "sanction_applied", true},
		{"booking_late", "late_return", true}, // spec 009: nombre anterior
		{" message_received ", "message_received", true},
		{"booking.meeting_point", "booking_meeting_point", false}, // spec 007: no está en el RF-02
		{"", "", false},
		{"desconocido", "desconocido", false},
	}
	for _, c := range cases {
		got, ok := Normalize(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("Normalize(%q) = %q, %v; quiero %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestEveryEmittedTypeNormalizesToATemplate(t *testing.T) {
	// Tipos que las specs 007 a 010 emiten hoy (salvo booking.meeting_point).
	for _, typ := range []string{
		"booking.requested", "booking.accepted", "booking.rejected", "booking.cancelled", "booking.expired",
		"message_received", "delivery_confirmed", "return_confirmed", "payment_reported", "payment_confirmed",
		"booking_late", "booking_affected", "incident.opened", "incident.note", "incident.closed",
		"rating.received", "sanction.applied",
	} {
		canon, ok := Normalize(typ)
		if !ok {
			t.Errorf("%q no se reconoce", typ)
			continue
		}
		if _, ok := TemplateFor(canon); !ok {
			t.Errorf("%q → %q sin plantilla", typ, canon)
		}
	}
}
