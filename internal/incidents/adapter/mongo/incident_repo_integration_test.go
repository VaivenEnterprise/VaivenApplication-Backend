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
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/usecase"
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

func newRepo(t *testing.T) (*Repo, *drv.Database) {
	t.Helper()
	db := testClient.Database(fmt.Sprintf("incidents_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := NewRepo(db)
	if err := repo.EnsureIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	return repo, db
}

var base = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

func (r *Repo) sample(booking, reporter, reported string, n int) domain.Incident {
	at := base.Add(time.Duration(n) * time.Minute)
	return domain.Incident{
		ID: r.NewID(), BookingID: booking, ReporterID: reporter, ReportedID: reported,
		Reason: domain.ReasonDamaged, Description: "La tapa llegó rota.",
		Evidence: []images.Image{{PublicID: "vaiven/test/evidence/u/1", Width: 10, Height: 20, Format: "jpg", Bytes: 99, Delivery: images.DeliveryAuthenticated}},
		Status:   domain.StatusOpen, Notes: []domain.Note{}, CreatedAt: at, UpdatedAt: at, Version: 1,
	}
}

func TestIntegrationEnsureIndexesIdempotent(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("segunda ejecución: %v", err)
	}
	cur, err := repo.col.Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var idx []struct {
		Name string `bson:"name"`
	}
	if err := cur.All(ctx, &idx); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, i := range idx {
		got[i.Name] = true
	}
	for _, want := range []string{"booking_status", "reporter_createdAt", "reported_createdAt", "status_createdAt"} {
		if !got[want] {
			t.Errorf("falta el índice %s: %v", want, got)
		}
	}
	if len(idx) != 5 { // _id_ + 4
		t.Errorf("índices = %d, quiero 5", len(idx))
	}
}

func TestIntegrationCreateAndByID(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	in := repo.sample("6731a0000000000000000001", "ana", "beto", 0)
	saved, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.ByID(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != in.ID || got.Status != domain.StatusOpen || got.Version != 1 || got.Reason != domain.ReasonDamaged ||
		len(got.Evidence) != 1 || got.Evidence[0].PublicID != in.Evidence[0].PublicID || got.Evidence[0].Delivery != images.DeliveryAuthenticated ||
		got.Notes == nil || got.Final != nil || got.ClosedAt != nil || !got.CreatedAt.Equal(in.CreatedAt) {
		t.Errorf("incidencia guardada = %+v", got)
	}
	if _, err := repo.ByID(ctx, bson.NewObjectID().Hex()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	if _, err := repo.ByID(ctx, "no-es-un-id"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("id mal formado: %v", err)
	}
	if ok, err := repo.Exists(ctx, in.ID); err != nil || !ok {
		t.Errorf("Exists(existente) = %v, %v", ok, err)
	}
	if ok, err := repo.Exists(ctx, bson.NewObjectID().Hex()); err != nil || ok {
		t.Errorf("Exists(inexistente) = %v, %v", ok, err)
	}
	if ok, err := repo.Exists(ctx, "basura"); err != nil || ok {
		t.Errorf("Exists(mal formado) = %v, %v", ok, err)
	}
}

func TestIntegrationCountOpenIgnoresClosedAndOtherBookings(t *testing.T) { // CA-08
	repo, _ := newRepo(t)
	ctx := context.Background()
	b, other := "6731a0000000000000000002", "6731a0000000000000000003"
	var first domain.Incident
	for i := 0; i < 3; i++ {
		inc, err := repo.Create(ctx, repo.sample(b, "ana", "beto", i))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = inc
		}
	}
	if _, err := repo.Create(ctx, repo.sample(other, "ana", "beto", 9)); err != nil {
		t.Fatal(err)
	}
	if n, err := repo.CountOpen(ctx, b); err != nil || n != 3 {
		t.Fatalf("CountOpen = %d, %v, quiero 3", n, err)
	}
	f, _ := domain.NewFinalResponse("Resuelto con ambas partes.", domain.OutcomeNone, "adm", base.Add(time.Hour))
	if _, err := repo.Close(ctx, first.ID, f); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountOpen(ctx, b); n != 2 {
		t.Errorf("tras cerrar una, CountOpen = %d, quiero 2", n)
	}
}

func TestIntegrationListByPartyBothRolesWithCursor(t *testing.T) { // RF-07
	repo, _ := newRepo(t)
	ctx := context.Background()
	// ana reporta 3 veces y es reportada 2; carla no participa.
	var ids []string
	for i := 0; i < 3; i++ {
		inc, err := repo.Create(ctx, repo.sample(fmt.Sprintf("6731a000000000000000010%d", i), "ana", "beto", i))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, inc.ID)
	}
	for i := 3; i < 5; i++ {
		inc, err := repo.Create(ctx, repo.sample(fmt.Sprintf("6731a000000000000000010%d", i), "beto", "ana", i))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, inc.ID)
	}
	f, _ := domain.NewFinalResponse("Resuelto con ambas partes.", domain.OutcomeNone, "adm", base.Add(time.Hour))
	if _, err := repo.Close(ctx, ids[0], f); err != nil {
		t.Fatal(err)
	}

	p1, err := repo.ListByParty(ctx, "ana", "", nil, 3)
	if err != nil || len(p1) != 3 {
		t.Fatalf("página 1 = %d (%v)", len(p1), err)
	}
	for i, inc := range p1 { // más recientes primero: ids[4], ids[3], ids[2]
		if want := ids[4-i]; inc.ID != want {
			t.Errorf("página 1[%d] = %s, quiero %s", i, inc.ID, want)
		}
	}
	p2, err := repo.ListByParty(ctx, "ana", "", &domain.Cursor{CreatedAt: p1[2].CreatedAt, ID: p1[2].ID}, 3)
	if err != nil || len(p2) != 2 || p2[0].ID != ids[1] || p2[1].ID != ids[0] {
		t.Errorf("página 2 = %+v (%v)", p2, err)
	}
	open, err := repo.ListByParty(ctx, "ana", domain.StatusOpen, nil, 10)
	if err != nil || len(open) != 4 {
		t.Errorf("abiertas = %d (%v), quiero 4", len(open), err)
	}
	closed, err := repo.ListByParty(ctx, "ana", domain.StatusClosed, nil, 10)
	if err != nil || len(closed) != 1 || closed[0].ID != ids[0] {
		t.Errorf("cerradas = %+v (%v)", closed, err)
	}
	if none, _ := repo.ListByParty(ctx, "carla", "", nil, 10); len(none) != 0 {
		t.Errorf("carla no participa: %d", len(none))
	}
	if _, err := repo.ListByParty(ctx, "ana", "", &domain.Cursor{ID: "malo"}, 3); err == nil {
		t.Error("un cursor inválido debe fallar")
	}
}

func TestIntegrationListAllOldestFirstByStatus(t *testing.T) { // RF-08
	repo, _ := newRepo(t)
	ctx := context.Background()
	var ids []string
	for i := 0; i < 4; i++ {
		inc, err := repo.Create(ctx, repo.sample(fmt.Sprintf("6731a000000000000000020%d", i), "ana", "beto", i))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, inc.ID)
	}
	f, _ := domain.NewFinalResponse("Resuelto con ambas partes.", domain.OutcomeNone, "adm", base.Add(time.Hour))
	if _, err := repo.Close(ctx, ids[1], f); err != nil {
		t.Fatal(err)
	}
	p1, err := repo.ListAll(ctx, domain.StatusOpen, nil, 2)
	if err != nil || len(p1) != 2 || p1[0].ID != ids[0] || p1[1].ID != ids[2] {
		t.Fatalf("página 1 = %+v (%v)", p1, err)
	}
	p2, err := repo.ListAll(ctx, domain.StatusOpen, &domain.Cursor{CreatedAt: p1[1].CreatedAt, ID: p1[1].ID}, 2)
	if err != nil || len(p2) != 1 || p2[0].ID != ids[3] {
		t.Errorf("página 2 = %+v (%v)", p2, err)
	}
	all, err := repo.ListAll(ctx, "", nil, 10)
	if err != nil || len(all) != 4 {
		t.Errorf("todas = %d (%v)", len(all), err)
	}
}

func TestIntegrationAddNoteAtomicLimitAndStatus(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	inc, err := repo.Create(ctx, repo.sample("6731a0000000000000000030", "ana", "beto", 0))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < domain.MaxNotes; i++ {
		n := domain.Note{Text: fmt.Sprintf("nota %d", i), At: base.Add(time.Duration(i) * time.Second), ByAdminID: "adm", VisibleToParties: i%2 == 0}
		got, err := repo.AddNote(ctx, inc.ID, n)
		if err != nil {
			t.Fatalf("nota %d: %v", i, err)
		}
		if len(got.Notes) != i+1 || got.Version != int64(i+2) {
			t.Errorf("tras la nota %d: notas=%d versión=%d", i, len(got.Notes), got.Version)
		}
	}
	// La undécima no cabe.
	if _, err := repo.AddNote(ctx, inc.ID, domain.Note{Text: "extra", At: base, ByAdminID: "adm"}); !errors.Is(err, usecase.ErrNoMatch) {
		t.Errorf("11.ª nota: %v, quiero ErrNoMatch", err)
	}
	got, _ := repo.ByID(ctx, inc.ID)
	if len(got.Notes) != domain.MaxNotes || got.Notes[0].Text != "nota 0" || !got.Notes[0].VisibleToParties || got.Notes[1].VisibleToParties {
		t.Errorf("notas guardadas = %+v", got.Notes)
	}
	if _, err := repo.AddNote(ctx, bson.NewObjectID().Hex(), domain.Note{Text: "x", At: base}); !errors.Is(err, usecase.ErrNoMatch) {
		t.Errorf("incidencia inexistente: %v", err)
	}
	if _, err := repo.AddNote(ctx, "malo", domain.Note{Text: "x", At: base}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("id mal formado: %v", err)
	}
}

func TestIntegrationConcurrentNotesNeverExceedLimit(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	inc, err := repo.Create(ctx, repo.sample("6731a0000000000000000031", "ana", "beto", 0))
	if err != nil {
		t.Fatal(err)
	}
	const n = 25
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = repo.AddNote(ctx, inc.ID, domain.Note{Text: fmt.Sprintf("n%d", i), At: base, ByAdminID: "adm"})
		}()
	}
	wg.Wait()
	ok := 0
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, usecase.ErrNoMatch):
		default:
			t.Errorf("error inesperado: %v", e)
		}
	}
	got, _ := repo.ByID(ctx, inc.ID)
	if ok != domain.MaxNotes || len(got.Notes) != domain.MaxNotes {
		t.Errorf("aceptadas=%d guardadas=%d, quiero %d", ok, len(got.Notes), domain.MaxNotes)
	}
}

func TestIntegrationCloseOnlyOnceAndBlocksNotes(t *testing.T) { // CA-07
	repo, _ := newRepo(t)
	ctx := context.Background()
	inc, err := repo.Create(ctx, repo.sample("6731a0000000000000000032", "ana", "beto", 0))
	if err != nil {
		t.Fatal(err)
	}
	at := base.Add(2 * time.Hour)
	f, _ := domain.NewFinalResponse("Revisamos la evidencia y hubo un ajuste.", domain.OutcomeMonetaryAdjustment, "adm", at)
	closed, err := repo.Close(ctx, inc.ID, f)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != domain.StatusClosed || closed.Final == nil || closed.Final.Text != f.Text ||
		closed.Final.Outcome != domain.OutcomeMonetaryAdjustment || closed.Final.ByAdminID != "adm" ||
		closed.ClosedAt == nil || !closed.ClosedAt.Equal(at) || !closed.UpdatedAt.Equal(at) || closed.Version != 2 {
		t.Errorf("cierre = %+v", closed)
	}
	if _, err := repo.Close(ctx, inc.ID, f); !errors.Is(err, usecase.ErrNoMatch) {
		t.Errorf("cerrar de nuevo: %v", err)
	}
	if _, err := repo.AddNote(ctx, inc.ID, domain.Note{Text: "tarde", At: at}); !errors.Is(err, usecase.ErrNoMatch) {
		t.Errorf("nota en cerrada: %v", err)
	}
	if _, err := repo.Close(ctx, "malo", f); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("id mal formado: %v", err)
	}
}
