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

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
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
	db := testClient.Database(fmt.Sprintf("reports_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := NewRepo(db)
	if err := repo.EnsureIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	return repo, db
}

const (
	reporter  = "6730f1c2a9b3e4d5f6a7b8c1"
	reporter2 = "6730f1c2a9b3e4d5f6a7b8c2"
	itemID    = "6731a0000000000000000002"
	adminID   = "6730f1c2a9b3e4d5f6a7b8ff"
)

func newReport(repo *Repo, by string, tt domain.TargetType, target string) domain.Report {
	d := "Detalle"
	return domain.Report{
		ID: repo.NewID(), ReporterID: by, TargetType: tt, TargetID: target, Reason: domain.ReasonProhibited,
		Details: &d, CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestIndexes(t *testing.T) {
	repo, db := newRepo(t)
	if err := repo.EnsureIndexes(context.Background()); err != nil { // idempotente
		t.Fatal(err)
	}
	cur, err := db.Collection(ReportsCollection).Indexes().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var idx []bson.M
	if err := cur.All(context.Background(), &idx); err != nil {
		t.Fatal(err)
	}
	found := map[string]bson.M{}
	for _, i := range idx {
		found[i["name"].(string)] = i
	}
	if found["reporter_target_unique"] == nil || found["reporter_target_unique"]["unique"] != true || found["status_created"] == nil {
		t.Errorf("faltan índices del plan §3: %v", found)
	}
}

func TestCreateAndByID(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	in := newReport(repo, reporter, domain.TargetItem, itemID)
	got, created, err := repo.Create(ctx, in)
	if err != nil || !created {
		t.Fatalf("Create: %v created=%v", err, created)
	}
	if got.ID != in.ID || got.Status != domain.StatusOpen || got.Resolution != nil || got.Details == nil || *got.Details != "Detalle" || !got.CreatedAt.Equal(in.CreatedAt) {
		t.Errorf("reporte guardado = %+v", got)
	}
	read, err := repo.ByID(ctx, in.ID)
	if err != nil || read.ID != in.ID || read.TargetType != domain.TargetItem || read.Reason != domain.ReasonProhibited {
		t.Errorf("ByID = %+v, %v", read, err)
	}
	for _, bad := range []string{"nope", repo.NewID()} {
		if _, err := repo.ByID(ctx, bad); !errors.Is(err, domain.ErrReportNotFound) {
			t.Errorf("ByID(%q) = %v, quiero ErrReportNotFound", bad, err)
		}
	}
	if _, _, err := repo.Create(ctx, domain.Report{ID: "malo"}); err == nil {
		t.Error("un id inválido debe fallar")
	}
}

func TestCreateWithoutDetailsStoresNull(t *testing.T) {
	repo, _ := newRepo(t)
	in := newReport(repo, reporter, domain.TargetUser, reporter2)
	in.Details = nil
	got, _, err := repo.Create(context.Background(), in)
	if err != nil || got.Details != nil {
		t.Errorf("details = %v, %v", got.Details, err)
	}
}

func TestDuplicateReturnsExisting(t *testing.T) { // RF-08
	repo, db := newRepo(t)
	ctx := context.Background()
	first, created, _ := repo.Create(ctx, newReport(repo, reporter, domain.TargetItem, itemID))
	if !created {
		t.Fatal("el primero se crea")
	}
	again, created, err := repo.Create(ctx, newReport(repo, reporter, domain.TargetItem, itemID))
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("duplicado: err=%v created=%v id=%s (quiero %s)", err, created, again.ID, first.ID)
	}
	// Otra persona sí puede reportar lo mismo; la misma, otro objetivo.
	if _, created, _ := repo.Create(ctx, newReport(repo, reporter2, domain.TargetItem, itemID)); !created {
		t.Error("otra persona puede reportar el mismo objeto")
	}
	if _, created, _ := repo.Create(ctx, newReport(repo, reporter, domain.TargetUser, itemID)); !created {
		t.Error("otro tipo de objetivo no es duplicado")
	}
	if n, _ := db.Collection(ReportsCollection).CountDocuments(ctx, bson.D{}); n != 3 {
		t.Errorf("hay %d reportes, quiero 3", n)
	}
}

func TestConcurrentDuplicatesCreateOne(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make(chan string, 10)
	var creates int
	var mu sync.Mutex
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, created, err := repo.Create(ctx, newReport(repo, reporter, domain.TargetItem, itemID))
			if err != nil {
				t.Errorf("Create concurrente: %v", err)
				return
			}
			mu.Lock()
			if created {
				creates++
			}
			mu.Unlock()
			ids <- r.ID
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		seen[id] = true
	}
	if creates != 1 || len(seen) != 1 {
		t.Errorf("creados=%d, ids distintos=%d; quiero 1 y 1", creates, len(seen))
	}
	if n, _ := db.Collection(ReportsCollection).CountDocuments(ctx, bson.D{}); n != 1 {
		t.Errorf("hay %d documentos, quiero 1", n)
	}
}

func TestListByStatusWithCursor(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	var ids []string
	for i := 0; i < 5; i++ {
		r := newReport(repo, reporter, domain.TargetModerationDecision, fmt.Sprintf("ref-%d", i))
		if _, _, err := repo.Create(ctx, r); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	if _, err := repo.Resolve(ctx, ids[0], domain.StatusDismissed, domain.Resolution{Action: domain.ActionDismiss, Note: "ok", ByAdminID: adminID, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	open, err := repo.List(ctx, domain.StatusOpen, "", 3)
	if err != nil || len(open) != 3 || open[0].ID != ids[1] || open[1].ID != ids[2] {
		t.Fatalf("página 1 de open = %v, %v", open, err)
	}
	rest, err := repo.List(ctx, domain.StatusOpen, open[2].ID, 3)
	if err != nil || len(rest) != 1 || rest[0].ID != ids[4] {
		t.Fatalf("página 2 de open = %v, %v", rest, err)
	}
	done, _ := repo.List(ctx, domain.StatusDismissed, "", 10)
	if len(done) != 1 || done[0].ID != ids[0] {
		t.Errorf("dismissed = %v", done)
	}
	if none, _ := repo.List(ctx, domain.StatusActioned, "", 10); len(none) != 0 {
		t.Errorf("actioned debe estar vacío: %v", none)
	}
	if _, err := repo.List(ctx, domain.StatusOpen, "no-es-id", 3); err == nil {
		t.Error("un cursor inválido debe fallar")
	}
}

func TestResolveKeepsDecisionAndAdmin(t *testing.T) { // RF-11
	repo, _ := newRepo(t)
	ctx := context.Background()
	in := newReport(repo, reporter, domain.TargetItem, itemID)
	if _, _, err := repo.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Millisecond)
	got, err := repo.Resolve(ctx, in.ID, domain.StatusActioned, domain.Resolution{Action: domain.ActionHideItem, Note: "Incumple la política.", ByAdminID: adminID, At: at})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusActioned || got.Resolution == nil || got.Resolution.Action != domain.ActionHideItem ||
		got.Resolution.Note != "Incumple la política." || got.Resolution.ByAdminID != adminID || !got.Resolution.At.Equal(at) {
		t.Errorf("resuelto = %+v / %+v", got, got.Resolution)
	}
	read, _ := repo.ByID(ctx, in.ID)
	if read.Resolution == nil || read.Resolution.ByAdminID != adminID || read.Status != domain.StatusActioned {
		t.Errorf("persistido = %+v", read)
	}
}

func TestResolveTwiceAndMissing(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	in := newReport(repo, reporter, domain.TargetItem, itemID)
	repo.Create(ctx, in)
	res := domain.Resolution{Action: domain.ActionDismiss, Note: "sin falta", ByAdminID: adminID, At: time.Now()}
	if _, err := repo.Resolve(ctx, in.ID, domain.StatusDismissed, res); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Resolve(ctx, in.ID, domain.StatusActioned, res); !errors.Is(err, domain.ErrAlreadyResolved) {
		t.Errorf("segunda resolución = %v, quiero ErrAlreadyResolved", err)
	}
	read, _ := repo.ByID(ctx, in.ID)
	if read.Status != domain.StatusDismissed || read.Resolution.Action != domain.ActionDismiss {
		t.Errorf("la primera decisión debe conservarse: %+v", read)
	}
	for _, id := range []string{repo.NewID(), "malo"} {
		if _, err := repo.Resolve(ctx, id, domain.StatusDismissed, res); !errors.Is(err, domain.ErrReportNotFound) {
			t.Errorf("Resolve(%q) = %v, quiero ErrReportNotFound", id, err)
		}
	}
}

func TestConcurrentResolveOneWins(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	in := newReport(repo, reporter, domain.TargetItem, itemID)
	repo.Create(ctx, in)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Resolve(ctx, in.ID, domain.StatusDismissed, domain.Resolution{Action: domain.ActionDismiss, Note: "sin falta", ByAdminID: adminID, At: time.Now()})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrAlreadyResolved):
			conflicts++
		default:
			t.Errorf("error inesperado: %v", err)
		}
	}
	if ok != 1 || conflicts != 7 {
		t.Errorf("ok=%d conflictos=%d; quiero 1 y 7", ok, conflicts)
	}
}
