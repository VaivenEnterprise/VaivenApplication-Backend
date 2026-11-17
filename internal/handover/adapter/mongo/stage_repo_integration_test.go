//go:build integration

package mongo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/usecase"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

var testClient *mongodb.Client

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "no se pudo iniciar MongoDB con Docker o Podman:", err)
		os.Exit(1)
	}
	uri, err := c.ConnectionString(ctx)
	if err == nil {
		testClient, err = mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "MongoDB:", err)
		_ = c.Terminate(ctx)
		os.Exit(1)
	}
	code := m.Run()
	_ = testClient.Disconnect(ctx)
	_ = c.Terminate(ctx)
	os.Exit(code)
}

var (
	ctx = context.Background()
	t0  = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)
)

type env struct {
	t     *testing.T
	db    *drv.Database
	books *bookingsmongo.Repo
	repo  *StageRepo
}

func newEnv(t *testing.T) env {
	t.Helper()
	db := testClient.Database(fmt.Sprintf("handover_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	return env{t: t, db: db, books: bookingsmongo.NewRepo(db), repo: NewStageRepo(db)}
}

// booking siembra una reserva en el estado dado.
func (e env) booking(status bookings.Status) bookings.Booking {
	e.t.Helper()
	b := bookings.Booking{
		ID: e.books.NewID(), ItemID: "i1", OwnerID: "owner", RequesterID: "req",
		Start: bookings.MustDate("2026-11-20"), End: bookings.MustDate("2026-11-22"), Days: 3,
		Item:        bookings.ItemSnapshot{Title: "Taladro", Modality: bookings.ModalityRent},
		Price:       bookings.PriceSnapshot{Modality: bookings.ModalityRent, PricePerDayCents: 3000, TotalCents: 9000, Currency: "MXN"},
		PaymentMode: bookings.PaymentFullOnDelivery, Status: status,
		History:   []bookings.HistoryEntry{{Status: status, At: t0, By: bookings.RoleOwner}},
		CreatedAt: t0, UpdatedAt: t0, Version: 1,
	}
	if err := e.books.Create(ctx, b); err != nil {
		e.t.Fatal(err)
	}
	return b
}

func (e env) get(id string) bookings.Booking {
	e.t.Helper()
	b, err := e.books.ByID(ctx, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

func evidence(prev *bookings.Handover, at time.Time) bookings.Handover {
	return domain.Record(prev, "owner", "Sin daños", []images.Image{{PublicID: "p1", Delivery: images.DeliveryAuthenticated}}, at)
}

// ready deja una reserva accepted con evidencia y un QR vigente para "tok".
func (e env) ready() (bookings.Booking, string) {
	e.t.Helper()
	b := e.booking(bookings.StatusAccepted)
	h := evidence(nil, t0)
	if err := e.repo.SaveEvidence(ctx, b.ID, domain.KindDelivery, h); err != nil {
		e.t.Fatal(err)
	}
	qr := domain.NewQR(&h, "tok", t0)
	if err := e.repo.SaveQR(ctx, b.ID, domain.KindDelivery, qr); err != nil {
		e.t.Fatal(err)
	}
	return e.get(b.ID), domain.HashToken("tok")
}

func plan(t *testing.T, b bookings.Booking, token string, at time.Time) domain.Confirmation {
	t.Helper()
	loc, _ := bookings.Location()
	c, err := domain.PlanConfirm(b, domain.KindDelivery, "req", token, at, loc, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSaveEvidenceAndQR(t *testing.T) {
	e := newEnv(t)
	b := e.booking(bookings.StatusAccepted)
	h := evidence(nil, t0)
	if err := e.repo.SaveEvidence(ctx, b.ID, domain.KindDelivery, h); err != nil {
		t.Fatal(err)
	}
	got := e.get(b.ID)
	if got.Delivery == nil || got.Delivery.EvidenceVersion != 1 || got.Delivery.ConditionNotes != "Sin daños" ||
		len(got.Delivery.Photos) != 1 || got.Delivery.QR != nil || got.Version != 2 {
		t.Fatalf("tras guardar la evidencia: %+v (version %d)", got.Delivery, got.Version)
	}
	// El QR debe llevar la versión de la evidencia vigente.
	if err := e.repo.SaveQR(ctx, b.ID, domain.KindDelivery, bookings.QR{TokenHash: "x", ExpiresAt: t0.Add(time.Minute), EvidenceVersion: 7}); !errors.Is(err, usecase.ErrNoMatch) {
		t.Errorf("QR de otra versión: %v", err)
	}
	qr := domain.NewQR(&h, "tok", t0)
	if err := e.repo.SaveQR(ctx, b.ID, domain.KindDelivery, qr); err != nil {
		t.Fatal(err)
	}
	got = e.get(b.ID)
	if got.Delivery.QR == nil || got.Delivery.QR.TokenHash != domain.HashToken("tok") || got.Version != 3 {
		t.Fatalf("QR guardado: %+v", got.Delivery.QR)
	}
	// Reemplazar la evidencia borra el QR (CA-04).
	h2 := evidence(got.Delivery, t0.Add(time.Minute))
	if err := e.repo.SaveEvidence(ctx, b.ID, domain.KindDelivery, h2); err != nil {
		t.Fatal(err)
	}
	if got = e.get(b.ID); got.Delivery.QR != nil || got.Delivery.EvidenceVersion != 2 {
		t.Errorf("la evidencia nueva debe borrar el QR: %+v", got.Delivery)
	}
}

func TestSaveEvidenceFilters(t *testing.T) {
	e := newEnv(t)
	for _, st := range []bookings.Status{bookings.StatusPending, bookings.StatusCancelled, bookings.StatusInProgress} {
		b := e.booking(st)
		if err := e.repo.SaveEvidence(ctx, b.ID, domain.KindDelivery, evidence(nil, t0)); !errors.Is(err, usecase.ErrNoMatch) {
			t.Errorf("entrega con reserva %s: %v", st, err)
		}
	}
	b := e.booking(bookings.StatusAccepted)
	if err := e.repo.SaveEvidence(ctx, b.ID, domain.KindReturn, evidence(nil, t0)); !errors.Is(err, usecase.ErrNoMatch) {
		t.Errorf("devolución con reserva accepted: %v", err)
	}
	if err := e.repo.SaveEvidence(ctx, "no-es-id", domain.KindDelivery, evidence(nil, t0)); !errors.Is(err, bookings.ErrNotFound) {
		t.Errorf("id inválido: %v", err)
	}
	if err := e.repo.SaveQR(ctx, "no-es-id", domain.KindDelivery, bookings.QR{}); !errors.Is(err, bookings.ErrNotFound) {
		t.Errorf("SaveQR id inválido: %v", err)
	}
	if err := e.repo.Confirm(ctx, "no-es-id", domain.Confirmation{Kind: domain.KindDelivery}, "h"); !errors.Is(err, bookings.ErrNotFound) {
		t.Errorf("Confirm id inválido: %v", err)
	}
}

func TestConfirmDelivery(t *testing.T) { // CA-01
	e := newEnv(t)
	b, hash := e.ready()
	late := t0.Add(72 * time.Hour)
	c := plan(t, b, "tok", t0.Add(time.Minute))
	c.LateAt = &late
	if err := e.repo.Confirm(ctx, b.ID, c, hash); err != nil {
		t.Fatal(err)
	}
	got := e.get(b.ID)
	if got.Status != bookings.StatusInProgress || got.Delivery.ConfirmedAt == nil || *got.Delivery.ConfirmedByID != "req" ||
		got.Delivery.QR.UsedAt == nil || got.LateAt == nil || !got.LateAt.Equal(late) {
		t.Fatalf("tras confirmar: %+v", got)
	}
	if n := len(got.History); n != 2 || got.History[n-1].Status != bookings.StatusInProgress || got.History[n-1].By != bookings.RoleSystem {
		t.Errorf("historial: %+v", got.History)
	}
	if got.Version != 4 { // creación 1 + evidencia + QR + confirmación
		t.Errorf("version = %d, quiero 4", got.Version)
	}
	// CA-03: el mismo QR no vuelve a servir.
	if err := e.repo.Confirm(ctx, b.ID, c, hash); !errors.Is(err, usecase.ErrNoMatch) {
		t.Errorf("reintento: %v", err)
	}
}

func TestConfirmRejectsStaleQR(t *testing.T) {
	t.Run("hash distinto", func(t *testing.T) {
		e := newEnv(t)
		b, _ := e.ready()
		c := plan(t, b, "tok", t0.Add(time.Minute))
		if err := e.repo.Confirm(ctx, b.ID, c, domain.HashToken("otro")); !errors.Is(err, usecase.ErrNoMatch) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("vencido (CA-02)", func(t *testing.T) {
		e := newEnv(t)
		b, hash := e.ready()
		c := plan(t, b, "tok", t0.Add(time.Minute))
		c.At = t0.Add(16 * time.Minute)
		if err := e.repo.Confirm(ctx, b.ID, c, hash); !errors.Is(err, usecase.ErrNoMatch) {
			t.Errorf("err = %v", err)
		}
		if e.get(b.ID).Status != bookings.StatusAccepted {
			t.Error("la reserva no debe cambiar")
		}
	})
	t.Run("evidencia reemplazada (CA-04)", func(t *testing.T) {
		e := newEnv(t)
		b, hash := e.ready()
		c := plan(t, b, "tok", t0.Add(time.Minute))
		if err := e.repo.SaveEvidence(ctx, b.ID, domain.KindDelivery, evidence(b.Delivery, t0.Add(2*time.Minute))); err != nil {
			t.Fatal(err)
		}
		if err := e.repo.Confirm(ctx, b.ID, c, hash); !errors.Is(err, usecase.ErrNoMatch) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("evidencia ya confirmada no se cambia (RF-13)", func(t *testing.T) {
		e := newEnv(t)
		b, hash := e.ready()
		if err := e.repo.Confirm(ctx, b.ID, plan(t, b, "tok", t0.Add(time.Minute)), hash); err != nil {
			t.Fatal(err)
		}
		if err := e.repo.SaveEvidence(ctx, b.ID, domain.KindDelivery, evidence(b.Delivery, t0)); !errors.Is(err, usecase.ErrNoMatch) {
			t.Errorf("err = %v", err)
		}
	})
}

// RNF-02: con el mismo QR a la vez, exactamente una confirmación tiene éxito.
func TestConfirmConcurrentExactlyOneWins(t *testing.T) {
	e := newEnv(t)
	b, hash := e.ready()
	c := plan(t, b, "tok", t0.Add(time.Minute))
	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = e.repo.Confirm(ctx, b.ID, c, hash)
		}(i)
	}
	close(start)
	wg.Wait()
	wins, misses := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, usecase.ErrNoMatch):
			misses++
		default:
			t.Errorf("error inesperado: %v", err)
		}
	}
	if wins != 1 || misses != n-1 {
		t.Fatalf("ganadoras = %d, perdedoras = %d; quiero 1 y %d", wins, misses, n-1)
	}
	got := e.get(b.ID)
	if got.Status != bookings.StatusInProgress || len(got.History) != 2 || got.Version != 4 {
		t.Errorf("el estado se aplicó más de una vez: status=%s history=%d version=%d", got.Status, len(got.History), got.Version)
	}
}

func TestConfirmReturnRecordsAdjustment(t *testing.T) {
	e := newEnv(t)
	b := e.booking(bookings.StatusInProgress)
	h := domain.Record(nil, "req", "Devuelto", []images.Image{{PublicID: "r1", Delivery: images.DeliveryAuthenticated}}, t0)
	if err := e.repo.SaveEvidence(ctx, b.ID, domain.KindReturn, h); err != nil {
		t.Fatal(err)
	}
	qr := domain.NewQR(&h, "rtok", t0)
	if err := e.repo.SaveQR(ctx, b.ID, domain.KindReturn, qr); err != nil {
		t.Fatal(err)
	}
	adj := bookings.Adjustment{Type: bookings.AdjustmentLateReturn, LateDays: 2, AmountCents: 6000, Status: bookings.AdjustmentRecorded}
	c := domain.Confirmation{
		Kind: domain.KindReturn, NewStatus: bookings.StatusCompleted, ConfirmedBy: "owner", At: t0.Add(time.Minute),
		History:    bookings.HistoryEntry{Status: bookings.StatusCompleted, At: t0.Add(time.Minute), By: bookings.RoleSystem},
		Adjustment: &adj,
	}
	if err := e.repo.Confirm(ctx, b.ID, c, domain.HashToken("rtok")); err != nil {
		t.Fatal(err)
	}
	got := e.get(b.ID)
	if got.Status != bookings.StatusCompleted || got.Return.ConfirmedAt == nil || len(got.Adjustments) != 1 || got.Adjustments[0] != adj {
		t.Errorf("tras la devolución: status=%s adj=%+v", got.Status, got.Adjustments)
	}
}

// La 007 guarda la reserva completa con ReplaceOne: debe conservar lo que
// escribió la 009, y no pisar un cambio de la 009 hecho después de su lectura.
func TestBookingsReplaceKeepsHandoverAndDetectsConflict(t *testing.T) {
	e := newEnv(t)
	b, _ := e.ready()
	stale := e.get(b.ID)

	// Un guardado de 007 con la reserva al día conserva la etapa y el QR.
	saved, err := e.books.Update(ctx, stale)
	if err != nil {
		t.Fatal(err)
	}
	after := e.get(b.ID)
	if after.Delivery == nil || after.Delivery.QR == nil || after.Delivery.EvidenceVersion != 1 || saved.Version != after.Version {
		t.Fatalf("la 007 perdió datos de la 009: %+v", after.Delivery)
	}

	// La 009 cambia la reserva; un guardado de 007 con la copia vieja falla.
	if err := e.repo.SaveEvidence(ctx, b.ID, domain.KindDelivery, evidence(after.Delivery, t0.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if _, err := e.books.Update(ctx, saved); !errors.Is(err, bookings.ErrVersionConflict) {
		t.Errorf("err = %v, quiero ErrVersionConflict", err)
	}
	if final := e.get(b.ID); final.Delivery.EvidenceVersion != 2 {
		t.Error("el guardado viejo de 007 no debe pisar la evidencia nueva")
	}
}
