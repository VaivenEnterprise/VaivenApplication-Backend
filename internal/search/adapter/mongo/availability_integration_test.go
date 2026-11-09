//go:build integration

package mongo

import (
	"context"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
)

// booking siembra una reserva a mano con el modelo de la spec 007 (plan §3:
// itemId en texto y fechas YYYY-MM-DD).
func (s *seed) booking(itemID, status, start, end string) {
	s.t.Helper()
	_, err := s.db.Collection(BookingsCollection).InsertOne(context.Background(), bson.M{
		"itemId": itemID, "status": status, "startDate": start, "endDate": end,
	})
	if err != nil {
		s.t.Fatal(err)
	}
}

func (s *seed) available(from, to string) map[string]bool {
	s.t.Helper()
	out := map[string]bool{}
	for _, id := range ids(s.search(domain.Query{ViewerID: "yo", Period: &domain.Period{From: from, To: to}})) {
		out[id] = true
	}
	return out
}

// T007: solo accepted e in_progress bloquean; pending, cancelled, rejected y
// expired no (RF-03).
func TestIntegrationAvailabilityCA03(t *testing.T) {
	s := newSeed(t)
	ana := s.user("ana", "active", nil)
	accepted := s.item(ana)
	inProgress := s.item(ana)
	pending := s.item(ana)
	cancelled := s.item(ana)
	free := s.item(ana)
	other := s.item(ana)

	s.booking(accepted, "accepted", "2026-11-10", "2026-11-12") // CA-03
	s.booking(inProgress, "in_progress", "2026-11-11", "2026-11-11")
	s.booking(pending, "pending", "2026-11-10", "2026-11-12")
	s.booking(cancelled, "cancelled", "2026-11-10", "2026-11-12")
	s.booking(other, "rejected", "2026-11-11", "2026-11-11")
	s.booking(other, "expired", "2026-11-11", "2026-11-11")

	cases := []struct {
		from, to   string
		blocked    []string
		notBlocked []string
	}{
		// CA-03: del 11 al 13 se cruza; del 13 al 14 ya no.
		{"2026-11-11", "2026-11-13", []string{accepted, inProgress}, []string{pending, cancelled, free, other}},
		{"2026-11-13", "2026-11-14", nil, []string{accepted, inProgress, pending, cancelled, free, other}},
		// Bordes: el periodo toca exactamente el inicio o el fin de la reserva.
		{"2026-11-08", "2026-11-10", []string{accepted}, []string{inProgress}},
		{"2026-11-12", "2026-11-12", []string{accepted}, []string{inProgress}},
		{"2026-11-09", "2026-11-09", nil, []string{accepted, inProgress}},
		// El periodo contiene la reserva completa.
		{"2026-11-09", "2026-11-13", []string{accepted, inProgress}, []string{free}},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s_%s", c.from, c.to), func(t *testing.T) {
			got := s.available(c.from, c.to)
			for _, id := range c.blocked {
				if got[id] {
					t.Errorf("%s debería estar ocupado", id)
				}
			}
			for _, id := range c.notBlocked {
				if !got[id] {
					t.Errorf("%s debería estar disponible", id)
				}
			}
		})
	}
	// Sin periodo, las reservas no filtran nada.
	if n := len(ids(s.search(domain.Query{ViewerID: "yo"}))); n != 6 {
		t.Errorf("sin periodo = %d objetos", n)
	}
}

// Si la colección bookings aún no existe (antes de la 007), nada se bloquea.
func TestIntegrationAvailabilityWithoutBookings(t *testing.T) {
	s := newSeed(t)
	ana := s.user("ana", "active", nil)
	s.item(ana)
	if n := len(s.available("2026-11-10", "2026-11-11")); n != 1 {
		t.Errorf("sin bookings = %d", n)
	}
}
