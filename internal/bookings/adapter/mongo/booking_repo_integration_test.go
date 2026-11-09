//go:build integration

package mongo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
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

var now0 = time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC)

type env struct {
	t    *testing.T
	db   *drv.Database
	repo *Repo
	loc  *time.Location
}

func newEnv(t *testing.T) env {
	t.Helper()
	db := testClient.Database(fmt.Sprintf("bookings_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := NewRepo(db)
	if err := repo.EnsureIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Las transacciones necesitan las colecciones creadas de antemano.
	for _, c := range []string{BookingsCollection, ItemsCollection} {
		_ = db.CreateCollection(context.Background(), c)
	}
	loc, _ := domain.Location()
	return env{t: t, db: db, repo: repo, loc: loc}
}

// item siembra un objeto con el modelo de la 005.
func (e env) item(owner, status string) string {
	e.t.Helper()
	res, err := e.db.Collection(ItemsCollection).InsertOne(context.Background(), bson.M{
		"ownerId": owner, "status": status, "title": "Taladro", "usageRules": "Limpio", "modality": "rent",
		"pricePerDayCents": int64(3000), "version": int64(1),
		"photos": bson.A{
			bson.M{"publicId": "p1", "width": 10, "height": 20, "format": "jpg", "bytes": int64(5), "delivery": "public"},
			bson.M{"publicId": "p2"},
		},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return res.InsertedID.(bson.ObjectID).Hex()
}

// request crea y guarda una solicitud pending.
func (e env) request(itemID, requester, start, end string) domain.Booking {
	e.t.Helper()
	it, err := e.repo.ItemForBooking(context.Background(), itemID)
	if err != nil {
		e.t.Fatal(err)
	}
	q, err := domain.NewQuote(domain.QuoteInput{
		ItemID: itemID, Modality: it.Modality, PricePerDayCents: it.PricePerDayCents,
		Start: start, End: end, PaymentMode: domain.PaymentSplit5050, Today: domain.Today(now0, e.loc),
	})
	if err != nil {
		e.t.Fatal(err)
	}
	b, err := domain.NewRequest(e.repo.NewID(), requester, it, q, now0, e.loc)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.repo.Create(context.Background(), b); err != nil {
		e.t.Fatal(err)
	}
	return b
}

func TestIntegrationRepoCRUD(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	itemID := e.item("own", "active")

	it, err := e.repo.ItemForBooking(ctx, itemID)
	if err != nil || it.OwnerID != "own" || it.FirstPhoto == nil || it.FirstPhoto.PublicID != "p1" || *it.UsageRules != "Limpio" {
		t.Fatalf("ItemForBooking = %+v %v", it, err)
	}
	removed := e.item("own", "removed")
	for _, id := range []string{removed, "zz", bson.NewObjectID().Hex()} {
		if _, err := e.repo.ItemForBooking(ctx, id); !errors.Is(err, domain.ErrItemNotFound) {
			t.Errorf("ItemForBooking(%s) = %v", id, err)
		}
	}

	b := e.request(itemID, "req", "2026-11-10", "2026-11-12")
	got, err := e.repo.ByID(ctx, b.ID)
	if err != nil || got.Start.String() != "2026-11-10" || got.Price.TotalCents != 9000 || len(got.Payments) != 2 ||
		got.Item.FirstPhoto.PublicID != "p1" || len(got.History) != 1 || !got.ExpiresAt.Equal(*b.ExpiresAt) {
		t.Fatalf("ByID = %+v %v", got, err)
	}
	// itemId se guarda como texto hexadecimal (la búsqueda de la 006 lo compara así).
	var raw bson.M
	_ = e.db.Collection(BookingsCollection).FindOne(ctx, bson.D{}).Decode(&raw)
	if raw["itemId"] != itemID || raw["startDate"] != "2026-11-10" {
		t.Errorf("documento = %v %v", raw["itemId"], raw["startDate"])
	}
	for _, id := range []string{"zz", bson.NewObjectID().Hex()} {
		if _, err := e.repo.ByID(ctx, id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ByID(%s) = %v", id, err)
		}
	}

	_ = got.Transition(domain.StatusCancelled, domain.RoleRequester, now0)
	upd, err := e.repo.Update(ctx, got)
	if err != nil || upd.Version != 2 || upd.Status != domain.StatusCancelled || upd.ExpiresAt != nil {
		t.Fatalf("Update = %+v %v", upd, err)
	}
	if _, err := e.repo.Update(ctx, got); !errors.Is(err, domain.ErrVersionConflict) {
		t.Errorf("versión vieja = %v", err)
	}
	if _, err := e.repo.Update(ctx, domain.Booking{ID: "zz"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("id inválido = %v", err)
	}
	if err := e.repo.Create(ctx, domain.Booking{ID: "zz"}); err == nil {
		t.Error("Create con id inválido")
	}
}

func TestIntegrationOverlapsAndList(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	itemID := e.item("own", "active")
	b1 := e.request(itemID, "req", "2026-11-10", "2026-11-12")
	e.request(itemID, "otro", "2026-11-11", "2026-11-11")

	d := domain.MustDate
	if p, _ := e.repo.HasPendingOverlap(ctx, itemID, "req", d("2026-11-12"), d("2026-11-13"), now0); !p {
		t.Error("pendiente propia que se cruza")
	}
	if p, _ := e.repo.HasPendingOverlap(ctx, itemID, "req", d("2026-11-13"), d("2026-11-14"), now0); p {
		t.Error("sin cruce")
	}
	if p, _ := e.repo.HasPendingOverlap(ctx, itemID, "req", d("2026-11-10"), d("2026-11-10"), now0.Add(49*time.Hour)); p {
		t.Error("una pendiente vencida no cuenta")
	}
	if blk, _ := e.repo.HasBlockingOverlap(ctx, itemID, d("2026-11-10"), d("2026-11-12")); blk {
		t.Error("las pendientes no bloquean")
	}

	acc := b1
	_ = acc.Transition(domain.StatusAccepted, domain.RoleOwner, now0)
	if _, _, err := e.repo.Accept(ctx, acc, now0); err != nil {
		t.Fatal(err)
	}
	if blk, _ := e.repo.HasBlockingOverlap(ctx, itemID, d("2026-11-12"), d("2026-11-14")); !blk {
		t.Error("la aceptada bloquea")
	}

	// Listado por rol con paginación.
	for i := range 3 {
		e.request(itemID, "req", fmt.Sprintf("2026-11-%02d", 20+i), fmt.Sprintf("2026-11-%02d", 20+i))
	}
	var seen int
	var after *domain.Cursor
	for range 5 {
		page, err := e.repo.ListByParty(ctx, "req", domain.RoleRequester, nil, after, 2, now0)
		if err != nil {
			t.Fatal(err)
		}
		seen += len(page)
		if len(page) < 2 {
			break
		}
		last := page[len(page)-1]
		after = &domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	if seen != 4 {
		t.Errorf("solicitante vio %d", seen)
	}
	pend := domain.StatusPending
	if own, _ := e.repo.ListByParty(ctx, "own", domain.RoleOwner, &pend, nil, 50, now0); len(own) != 3 {
		t.Errorf("pendientes del propietario = %d", len(own))
	}
	// Estado efectivo: a las 49 h las pendientes cuentan como expired (P-04).
	late := now0.Add(49 * time.Hour)
	if own, _ := e.repo.ListByParty(ctx, "own", domain.RoleOwner, &pend, nil, 50, late); len(own) != 0 {
		t.Errorf("pendientes vencidas = %d", len(own))
	}
	exp := domain.StatusExpired
	if own, _ := e.repo.ListByParty(ctx, "own", domain.RoleOwner, &exp, nil, 50, late); len(own) != 3 {
		t.Errorf("vencidas sin barrer = %d", len(own))
	}
	accSt := domain.StatusAccepted
	if own, _ := e.repo.ListByParty(ctx, "own", domain.RoleOwner, &accSt, nil, 50, late); len(own) != 1 {
		t.Errorf("aceptadas = %d", len(own))
	}
	if _, err := e.repo.ListByParty(ctx, "req", domain.RoleRequester, nil, &domain.Cursor{ID: "x"}, 5, now0); err == nil {
		t.Error("cursor inválido")
	}

	// Vencidas.
	due, err := e.repo.DuePending(ctx, now0.Add(49*time.Hour), 100)
	if err != nil || len(due) != 3 {
		t.Errorf("vencidas = %d %v", len(due), err)
	}
}
