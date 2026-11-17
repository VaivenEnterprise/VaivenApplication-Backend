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

	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
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

type env struct {
	db    *drv.Database
	repo  *Repo
	users *authmongo.UserRepo
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testClient.Database(fmt.Sprintf("ratings_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	users := authmongo.NewUserRepo(db)
	repo := NewRepo(db, users)
	if err := repo.EnsureIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	return &env{db: db, repo: repo, users: users}
}

// seedUser inserta una cuenta activa mínima y devuelve su id.
func (e *env) seedUser(t *testing.T, name string) string {
	t.Helper()
	oid := bson.NewObjectID()
	now := time.Now().UTC()
	_, err := e.db.Collection(authmongo.UsersCollection).InsertOne(context.Background(), bson.M{
		"_id": oid, "displayName": name, "status": "active", "role": "user",
		"ratingSum": int64(0), "ratingCount": int64(0), "createdAt": now, "updatedAt": now, "version": int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	return oid.Hex()
}

func (e *env) profile(t *testing.T, id string) (sum, count int64) {
	t.Helper()
	var d struct {
		Sum   int64 `bson:"ratingSum"`
		Count int64 `bson:"ratingCount"`
	}
	oid, _ := bson.ObjectIDFromHex(id)
	if err := e.db.Collection(authmongo.UsersCollection).FindOne(context.Background(), bson.M{"_id": oid}).Decode(&d); err != nil {
		t.Fatal(err)
	}
	return d.Sum, d.Count
}

func (e *env) rating(bookingID, raterID, rateeID string, score int) domain.Rating {
	comment := "Todo bien"
	return domain.Rating{
		ID: e.repo.NewID(), BookingID: bookingID, RaterID: raterID, RateeID: rateeID,
		RatedRole: bookings.RoleOwner, Score: score, Comment: &comment, CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestIntegrationEnsureIndexesIdempotent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("segunda ejecución: %v", err)
	}
	cur, err := e.repo.col.Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var idx []struct {
		Name   string `bson:"name"`
		Unique bool   `bson:"unique"`
	}
	if err := cur.All(ctx, &idx); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, i := range idx {
		got[i.Name] = i.Unique
	}
	if unique, ok := got["booking_rater_unique"]; !ok || !unique {
		t.Errorf("falta el índice único booking_rater_unique: %v", got)
	}
	if _, ok := got["ratee_id"]; !ok || len(idx) != 3 { // _id_ + 2
		t.Errorf("índices = %v", got)
	}
}

func TestIntegrationCreateUpdatesProfileAtomically(t *testing.T) { // CA-01, RF-02
	e := newEnv(t)
	ctx := context.Background()
	rater, ratee := e.seedUser(t, "Ana"), e.seedUser(t, "Beto")

	saved, err := e.repo.Create(ctx, e.rating("6731a0000000000000000001", rater, ratee, 5))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Score != 5 || saved.RateeID != ratee || saved.Comment == nil {
		t.Errorf("saved = %+v", saved)
	}
	if sum, count := e.profile(t, ratee); sum != 5 || count != 1 {
		t.Errorf("perfil = %d/%d, quiero 5/1", sum, count)
	}
	if sum, count := e.profile(t, rater); sum != 0 || count != 0 {
		t.Errorf("quien califica no cambia: %d/%d", sum, count)
	}
	// El perfil público refleja promedio y cantidad.
	pub, err := e.users.PublicUsers(ctx, []string{ratee})
	if err != nil || pub[ratee].RatingAverage == nil || *pub[ratee].RatingAverage != 5 || pub[ratee].RatingCount != 1 {
		t.Errorf("PublicUsers = %+v, err = %v", pub[ratee], err)
	}
	got, found, err := e.repo.ByBookingAndRater(ctx, saved.BookingID, rater)
	if err != nil || !found || got.ID != saved.ID {
		t.Errorf("ByBookingAndRater = %+v, %v, %v", got, found, err)
	}
	if _, found, _ := e.repo.ByBookingAndRater(ctx, saved.BookingID, ratee); found {
		t.Error("la otra parte aún no calificó")
	}
}

func TestIntegrationDuplicateRatingLeavesProfileUntouched(t *testing.T) { // CA-03
	e := newEnv(t)
	ctx := context.Background()
	rater, ratee := e.seedUser(t, "Ana"), e.seedUser(t, "Beto")
	b := "6731a0000000000000000002"
	if _, err := e.repo.Create(ctx, e.rating(b, rater, ratee, 4)); err != nil {
		t.Fatal(err)
	}
	_, err := e.repo.Create(ctx, e.rating(b, rater, ratee, 1))
	if !errors.Is(err, domain.ErrAlreadyRated) {
		t.Fatalf("err = %v, quiero ErrAlreadyRated", err)
	}
	if sum, count := e.profile(t, ratee); sum != 4 || count != 1 {
		t.Errorf("el duplicado no debe sumar: %d/%d", sum, count)
	}
	// La otra parte sí puede calificar la misma reserva.
	if _, err := e.repo.Create(ctx, e.rating(b, ratee, rater, 3)); err != nil {
		t.Errorf("la contraparte debe poder calificar: %v", err)
	}
}

func TestIntegrationConcurrentDuplicatesOneWins(t *testing.T) { // RNF-01
	e := newEnv(t)
	ctx := context.Background()
	rater, ratee := e.seedUser(t, "Ana"), e.seedUser(t, "Beto")
	b := "6731a0000000000000000003"

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.repo.Create(ctx, e.rating(b, rater, ratee, 5))
		}()
	}
	wg.Wait()

	wins, dups := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, domain.ErrAlreadyRated):
			dups++
		default:
			t.Errorf("error inesperado: %v", err)
		}
	}
	if wins != 1 || dups != n-1 {
		t.Fatalf("ganadoras=%d duplicadas=%d, quiero 1 y %d", wins, dups, n-1)
	}
	if sum, count := e.profile(t, ratee); sum != 5 || count != 1 {
		t.Errorf("perfil = %d/%d, quiero 5/1", sum, count)
	}
	total, err := e.repo.col.CountDocuments(ctx, bson.M{"bookingId": b})
	if err != nil || total != 1 {
		t.Errorf("calificaciones guardadas = %d (%v), quiero 1", total, err)
	}
}

func TestIntegrationCreateRollsBackWhenProfileUpdateFails(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rater := e.seedUser(t, "Ana")
	ghost := bson.NewObjectID().Hex() // la persona calificada no existe
	b := "6731a0000000000000000004"

	if _, err := e.repo.Create(ctx, e.rating(b, rater, ghost, 5)); err == nil {
		t.Fatal("quería error al no existir la persona calificada")
	}
	if _, found, err := e.repo.ByBookingAndRater(ctx, b, rater); err != nil || found {
		t.Errorf("si falla el perfil no debe quedar la calificación (found=%v, err=%v)", found, err)
	}
}

func TestIntegrationListByRateeNewestFirstWithCursor(t *testing.T) { // RF-04
	e := newEnv(t)
	ctx := context.Background()
	ratee := e.seedUser(t, "Beto")
	var ids []string
	for i := 0; i < 5; i++ {
		rater := e.seedUser(t, fmt.Sprintf("Persona %d", i))
		r, err := e.repo.Create(ctx, e.rating(fmt.Sprintf("6731a000000000000000010%d", i), rater, ratee, i%5+1))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	// Una calificación a otra persona no aparece.
	other := e.seedUser(t, "Otra")
	if _, err := e.repo.Create(ctx, e.rating("6731a0000000000000000199", e.seedUser(t, "X"), other, 3)); err != nil {
		t.Fatal(err)
	}

	page1, err := e.repo.ListByRatee(ctx, ratee, "", 3)
	if err != nil || len(page1) != 3 {
		t.Fatalf("página 1 = %d (%v)", len(page1), err)
	}
	for i, r := range page1 {
		if want := ids[len(ids)-1-i]; r.ID != want {
			t.Errorf("página 1[%d] = %s, quiero %s (más recientes primero)", i, r.ID, want)
		}
	}
	page2, err := e.repo.ListByRatee(ctx, ratee, page1[2].ID, 3)
	if err != nil || len(page2) != 2 || page2[0].ID != ids[1] || page2[1].ID != ids[0] {
		t.Errorf("página 2 = %+v (%v)", page2, err)
	}
	if _, err := e.repo.ListByRatee(ctx, ratee, "no-es-un-id", 3); err == nil {
		t.Error("un cursor inválido debe fallar")
	}
}
