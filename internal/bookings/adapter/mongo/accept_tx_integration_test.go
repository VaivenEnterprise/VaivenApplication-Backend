//go:build integration

package mongo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
)

func accepted(t *testing.T, b domain.Booking) domain.Booking {
	t.Helper()
	if err := b.Transition(domain.StatusAccepted, domain.RoleOwner, now0); err != nil {
		t.Fatal(err)
	}
	return b
}

// CA-05 / RNF-01: dos aceptaciones simultáneas que se cruzan; exactamente una gana.
func TestIntegrationAcceptConcurrencyCA05(t *testing.T) {
	for round := range 10 {
		e := newEnv(t)
		itemID := e.item("own", "active")
		a := e.request(itemID, "r1", "2026-11-10", "2026-11-12")
		b := e.request(itemID, "r2", "2026-11-12", "2026-11-14")

		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, bk := range []domain.Booking{a, b} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _, errs[i] = e.repo.Accept(context.Background(), accepted(t, bk), now0)
			}()
		}
		wg.Wait()

		ok, conflicts := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrPeriodTaken), errors.Is(err, domain.ErrVersionConflict):
				conflicts++
			default:
				t.Fatalf("ronda %d: error inesperado %v", round, err)
			}
		}
		if ok != 1 || conflicts != 1 {
			t.Fatalf("ronda %d: ganadoras %d, conflictos %d", round, ok, conflicts)
		}
		n, _ := e.db.Collection(BookingsCollection).CountDocuments(context.Background(), bson.D{{Key: "status", Value: "accepted"}})
		if n != 1 {
			t.Fatalf("ronda %d: %d aceptadas", round, n)
		}
	}
}

func TestIntegrationAcceptRejectsOverlappingPending(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	itemID := e.item("own", "active")
	win := e.request(itemID, "r1", "2026-11-10", "2026-11-12")
	lose := e.request(itemID, "r2", "2026-11-12", "2026-11-13")
	keep := e.request(itemID, "r3", "2026-11-13", "2026-11-14")

	acc, rejected, err := e.repo.Accept(ctx, accepted(t, win), now0)
	if err != nil || acc.Status != domain.StatusAccepted || acc.Version != 2 {
		t.Fatalf("Accept = %+v %v", acc, err)
	}
	if len(rejected) != 1 || rejected[0].ID != lose.ID || *rejected[0].RejectReason != "no_disponible" || rejected[0].Status != domain.StatusRejected {
		t.Fatalf("rechazadas = %+v", rejected)
	}
	if got, _ := e.repo.ByID(ctx, keep.ID); got.Status != domain.StatusPending {
		t.Errorf("la que no se cruza sigue pending: %s", got.Status)
	}
	// El objeto recibió el contador y la versión (bloquea un DELETE concurrente de la 005).
	var it bson.M
	_ = e.db.Collection(ItemsCollection).FindOne(ctx, bson.D{}).Decode(&it)
	if it["reservationSeq"] != int32(1) || it["version"] != int64(2) {
		t.Errorf("objeto = seq %v versión %v", it["reservationSeq"], it["version"])
	}

	// Aceptar dos veces la misma: conflicto de versión.
	if _, _, err := e.repo.Accept(ctx, accepted(t, win), now0); !errors.Is(err, domain.ErrPeriodTaken) && !errors.Is(err, domain.ErrVersionConflict) {
		t.Errorf("repetir = %v", err)
	}
}

func TestIntegrationAcceptItemUnavailable(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	itemID := e.item("own", "active")
	b := e.request(itemID, "r1", "2026-11-10", "2026-11-12")
	b2 := e.request(itemID, "r2", "2026-11-20", "2026-11-20")
	_, _ = e.db.Collection(ItemsCollection).UpdateOne(ctx, bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "blocked"}}}})
	if _, _, err := e.repo.Accept(ctx, accepted(t, b), now0); !errors.Is(err, domain.ErrItemUnavailable) {
		t.Errorf("bloqueado = %v", err)
	}
	// Pausado sí se puede aceptar (conserva compromisos).
	_, _ = e.db.Collection(ItemsCollection).UpdateOne(ctx, bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "paused"}}}})
	if _, _, err := e.repo.Accept(ctx, accepted(t, b), now0); err != nil {
		t.Errorf("pausado = %v", err)
	}
	// Una pendiente que venció entre leerla y aceptar no se acepta.
	if _, _, err := e.repo.Accept(ctx, accepted(t, b2), now0.Add(49*time.Hour)); !errors.Is(err, domain.ErrVersionConflict) {
		t.Errorf("vencida en la transacción = %v", err)
	}
	if _, _, err := e.repo.Accept(ctx, domain.Booking{ItemID: "zz"}, now0); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("objeto inválido = %v", err)
	}
}

func TestIntegrationActivityCheckers(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	itemID := e.item("own", "active")
	b := e.request(itemID, "req", "2026-11-10", "2026-11-12")
	users, items := NewUserActivity(e.db), NewItemActivity(e.db)

	for _, who := range []string{"own", "req"} {
		if busy, err := users.HasActiveBookings(ctx, who); err != nil || busy {
			t.Errorf("%s con pendiente = %v %v", who, busy, err)
		}
	}
	if busy, _ := items.HasActiveBookings(ctx, itemID); busy {
		t.Error("objeto con pendiente")
	}
	acc, _, err := e.repo.Accept(ctx, accepted(t, b), now0)
	if err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"own", "req"} {
		if busy, _ := users.HasActiveBookings(ctx, who); !busy {
			t.Errorf("%s con aceptada debe estar ocupada (RF-14)", who)
		}
	}
	if busy, _ := users.HasActiveBookings(ctx, "otro"); busy {
		t.Error("tercero")
	}
	if busy, _ := items.HasActiveBookings(ctx, itemID); !busy {
		t.Error("objeto con aceptada")
	}
	_ = acc.Transition(domain.StatusCancelled, domain.RoleOwner, now0)
	if _, err := e.repo.Update(ctx, acc); err != nil {
		t.Fatal(err)
	}
	if busy, _ := items.HasActiveBookings(ctx, itemID); busy {
		t.Error("cancelar libera (CA-08)")
	}
}
