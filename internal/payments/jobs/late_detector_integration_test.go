//go:build integration

package jobs_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"

	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	paymentsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/jobs"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/usecase"
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

type notifier struct {
	mu   sync.Mutex
	sent []string
}

func (n *notifier) Notify(_ context.Context, to string, e notify.Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, e.Type+":"+to)
	return nil
}

func (n *notifier) count(s string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := 0
	for _, x := range n.sent {
		if x == s {
			c++
		}
	}
	return c
}

// Dos réplicas del detector corriendo a la vez sobre el mismo Mongo: cada
// reserva se marca y se avisa una sola vez (plan P-05, Art. XIV).
func TestIntegrationTwoReplicasMarkOnce(t *testing.T) {
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("late_job_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := bookingsmongo.NewRepo(db)
	payRepo := paymentsmongo.NewRepo(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := payRepo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	loc, err := bookings.Location()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	today := bookings.Today(now, loc)
	lateness := domain.NewLatenessIn(loc)

	seed := func(item string, st bookings.Status, start, end bookings.Date, late bool) string {
		b := bookings.Booking{
			ID: repo.NewID(), ItemID: item, OwnerID: "own", RequesterID: "req", Start: start, End: end, Days: 1,
			Item: bookings.ItemSnapshot{Modality: bookings.ModalityRent}, Status: st,
			Price:     bookings.PriceSnapshot{Modality: bookings.ModalityRent, PricePerDayCents: 3000, Currency: "MXN"},
			CreatedAt: now, UpdatedAt: now, Version: 1,
		}
		if late {
			la := lateness.LateAt(b)
			b.LateAt = &la
		}
		if err := repo.Create(ctx, b); err != nil {
			t.Fatal(err)
		}
		return b.ID
	}
	var lateIDs []string
	for i := range 5 {
		lateIDs = append(lateIDs, seed(fmt.Sprintf("item%d", i), bookings.StatusInProgress, today.AddDays(-4), today.AddDays(-2), true))
		seed(fmt.Sprintf("item%d", i), bookings.StatusAccepted, today, today.AddDays(1), false) // afectada
	}
	seed("ok", bookings.StatusInProgress, today.AddDays(-1), today.AddDays(3), true) // aún no vence

	not := &notifier{}
	newSvc := func() *usecase.Service {
		s, err := usecase.NewService(usecase.Deps{
			Repo: payRepo, Bookings: repo, Notifier: not,
			Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	// Dos réplicas corren RunLateDetector a la vez, cada una con su tick. La
	// primera pasada ocurre al arrancar; el tick fuerza una segunda, y recién
	// entonces se cancela, así que ambas hicieron al menos dos pasadas.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	ticks := make([]chan time.Time, 2)
	for i := range ticks {
		ticks[i] = make(chan time.Time)
		svc := newSvc()
		wg.Add(1)
		go func() {
			defer wg.Done()
			jobs.RunLateDetector(runCtx, svc, ticks[i], slog.New(slog.NewTextHandler(io.Discard, nil)))
		}()
	}
	for _, tk := range ticks {
		tk <- time.Now()
	}
	cancel()
	wg.Wait()

	if got := not.count("booking_late:own"); got != 5 {
		t.Errorf("booking_late al propietario = %d, quiero 5", got)
	}
	if got := not.count("booking_late:req"); got != 5 {
		t.Errorf("booking_late al solicitante = %d, quiero 5", got)
	}
	if got := not.count("booking_affected:req"); got != 5 {
		t.Errorf("booking_affected = %d, quiero 5", got)
	}
	for _, id := range lateIDs {
		b, err := repo.ByID(ctx, id)
		if err != nil || !b.IsLate {
			t.Errorf("reserva %s: isLate = %v, error %v", id, b.IsLate, err)
		}
	}
}
