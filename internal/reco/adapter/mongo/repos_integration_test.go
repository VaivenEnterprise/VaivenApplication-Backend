//go:build integration

package mongo

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

func testDB(t *testing.T) *drv.Database {
	t.Helper()
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatalf("no se pudo iniciar MongoDB con Docker o Podman: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	uri, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	return client.Database("vaiven_reco_test")
}

var t0 = time.Date(2026, 11, 17, 18, 0, 0, 0, time.UTC)

func view(user, item string, at time.Time) domain.Event {
	return domain.Event{UserID: user, ItemID: item, Type: domain.EventView, Weight: domain.WeightView, At: at}
}

func TestIntegrationRecoRepositories(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	t.Run("interacciones", func(t *testing.T) {
		repo := NewInteractionRepo(db)
		for range 2 { // idempotente
			if err := repo.EnsureIndexes(ctx); err != nil {
				t.Fatal(err)
			}
		}
		assertTTL(t, db.Collection(InteractionsCollection), "created_ttl", 90*24*3600)

		// Dos vistas del mismo objeto en el mismo bloque de 10 min: solo una cuenta (CA-09).
		ok1, err1 := repo.Record(ctx, view("u1", "i1", t0.Add(1*time.Minute)))
		ok2, err2 := repo.Record(ctx, view("u1", "i1", t0.Add(9*time.Minute)))
		ok3, err3 := repo.Record(ctx, view("u1", "i1", t0.Add(11*time.Minute)))
		if err1 != nil || err2 != nil || err3 != nil || !ok1 || ok2 || !ok3 {
			t.Fatalf("bloques: %v %v %v / %v %v %v", ok1, ok2, ok3, err1, err2, err3)
		}
		_, _ = repo.Record(ctx, view("u1", "i2", t0.Add(20*time.Minute)))
		_, _ = repo.Record(ctx, view("u2", "i1", t0.Add(30*time.Minute)))

		recent, err := repo.Recent(ctx, "u1", 2)
		if err != nil || len(recent) != 2 || recent[0].ItemID != "i2" || recent[1].ItemID != "i1" || !recent[0].At.Equal(t0.Add(20*time.Minute)) {
			t.Fatalf("Recent = %+v, %v", recent, err)
		}
		if recent[0].Type != domain.EventView || recent[0].Weight != 1 {
			t.Errorf("tipo y peso: %+v", recent[0])
		}
		since, err := repo.Since(ctx, t0.Add(10*time.Minute), 10)
		if err != nil || len(since) != 3 {
			t.Fatalf("Since = %d, %v", len(since), err)
		}
		if lim, _ := repo.Since(ctx, t0, 2); len(lim) != 2 || lim[0].UserID != "u2" {
			t.Errorf("Since con límite: %+v", lim)
		}

		// Demostración: los repetidos se ignoran y la purga solo borra lo demo.
		demo := []domain.Event{view("demo-001", "i1", t0), view("demo-001", "i1", t0.Add(time.Minute)), view("demo-002", "i3", t0)}
		n, err := repo.InsertDemo(ctx, demo)
		if err != nil || n != 2 {
			t.Fatalf("InsertDemo = %d, %v", n, err)
		}
		if n, err := repo.InsertDemo(ctx, nil); err != nil || n != 0 {
			t.Errorf("InsertDemo vacío = %d, %v", n, err)
		}
		purged, err := repo.PurgeDemo(ctx)
		if err != nil || purged != 2 {
			t.Fatalf("PurgeDemo = %d, %v", purged, err)
		}

		if err := repo.DeleteUser(ctx, "u1"); err != nil {
			t.Fatal(err)
		}
		if left, _ := repo.Recent(ctx, "u1", 10); len(left) != 0 {
			t.Errorf("DeleteUser dejó %d", len(left))
		}
		if err := repo.DeleteUsers(ctx, nil); err != nil {
			t.Fatal(err)
		}
		if err := repo.DeleteUsers(ctx, []string{"u2", "otra"}); err != nil {
			t.Fatal(err)
		}
		if n, _ := db.Collection(InteractionsCollection).CountDocuments(ctx, bson.D{}); n != 0 {
			t.Errorf("quedan %d eventos", n)
		}
	})

	t.Run("candado", func(t *testing.T) {
		meta := NewMetaRepo(db)
		if _, _, ok, err := meta.Current(ctx); err != nil || ok {
			t.Fatalf("sin modelo: %v %v", ok, err)
		}
		// 20 procesos a la vez: exactamente uno toma el candado (CA-08).
		var wins atomic.Int32
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ok, err := meta.TryLock(ctx, fmt.Sprintf("p%d", i), t0)
				if err != nil {
					t.Errorf("TryLock: %v", err)
				}
				if ok {
					wins.Add(1)
				}
			}()
		}
		wg.Wait()
		if wins.Load() != 1 {
			t.Fatalf("ganadores = %d, want 1", wins.Load())
		}
		var d modelMetaDoc
		if err := db.Collection(MetaCollection).FindOne(ctx, bson.D{{Key: "_id", Value: modelMetaID}}).Decode(&d); err != nil {
			t.Fatal(err)
		}
		winner := d.LockOwner

		// Sigue vigente un minuto después; vence a los 2 min y otro lo toma.
		if ok, _ := meta.TryLock(ctx, "tarde", t0.Add(time.Minute)); ok {
			t.Error("el candado vigente no debe poder tomarse")
		}
		if ok, err := meta.TryLock(ctx, "nuevo", t0.Add(domain.LockTTL+time.Second)); err != nil || !ok {
			t.Fatalf("candado vencido: %v %v", ok, err)
		}
		// El dueño anterior ya no puede publicar.
		if ok, err := meta.Publish(ctx, winner, "b-viejo", t0); err != nil || ok {
			t.Errorf("publicar sin candado: %v %v", ok, err)
		}
		if ok, err := meta.Publish(ctx, "nuevo", "b1", t0.Add(3*time.Minute)); err != nil || !ok {
			t.Fatalf("Publish: %v %v", ok, err)
		}
		build, at, ok, err := meta.Current(ctx)
		if err != nil || !ok || build != "b1" || !at.Equal(t0.Add(3*time.Minute)) {
			t.Fatalf("Current = %s %v %v %v", build, at, ok, err)
		}
		// Publicar suelta el candado; Unlock también.
		if ok, _ := meta.TryLock(ctx, "otro", t0.Add(4*time.Minute)); !ok {
			t.Error("tras publicar el candado debe estar libre")
		}
		if err := meta.Unlock(ctx, "otro"); err != nil {
			t.Fatal(err)
		}
		if ok, _ := meta.TryLock(ctx, "uno-mas", t0.Add(4*time.Minute)); !ok {
			t.Error("tras Unlock el candado debe estar libre")
		}
		if build, _, _, _ := meta.Current(ctx); build != "b1" {
			t.Errorf("tomar el candado no debe cambiar el modelo vigente: %s", build)
		}

		// Borrado de historial.
		if _, ok, err := meta.ResetAt(ctx, "u9"); err != nil || ok {
			t.Fatalf("sin borrado: %v %v", ok, err)
		}
		if err := meta.SetReset(ctx, "u9", t0); err != nil {
			t.Fatal(err)
		}
		if err := meta.SetReset(ctx, "u9", t0.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if at, ok, _ := meta.ResetAt(ctx, "u9"); !ok || !at.Equal(t0.Add(time.Hour)) {
			t.Errorf("ResetAt = %v %v", at, ok)
		}
		resets, err := meta.Resets(ctx)
		if err != nil || len(resets) != 1 || !resets["u9"].Equal(t0.Add(time.Hour)) {
			t.Errorf("Resets = %v %v", resets, err)
		}
	})

	t.Run("modelo", func(t *testing.T) {
		repo := NewModelRepo(db)
		many := make([]domain.Neighbor, 25)
		for i := range many {
			many[i] = domain.Neighbor{ItemID: fmt.Sprintf("n%02d", i), CF: 1 - float64(i)/100, Content: 0.5}
		}
		pops := make([]string, 60)
		for i := range pops {
			pops[i] = fmt.Sprintf("p%02d", i)
		}
		if err := repo.Save(ctx, "b1", map[string][]domain.Neighbor{"i1": many, "i2": many[:3]}, map[string][]string{"all": pops, "c:hogar": {"x"}}, t0); err != nil {
			t.Fatal(err)
		}
		got, err := repo.Neighbors(ctx, "b1", []string{"i1", "i2", "no-existe"})
		if err != nil || len(got) != 2 || len(got["i1"]) != domain.Neighbors || len(got["i2"]) != 3 || got["i1"][0] != many[0] {
			t.Fatalf("Neighbors = %d %v", len(got), err)
		}
		pop, err := repo.Popular(ctx, "b1", []string{"all", "c:hogar", "m:nada"})
		if err != nil || len(pop) != 2 || len(pop["all"]) != domain.PopularSize || pop["c:hogar"][0] != "x" {
			t.Fatalf("Popular = %v %v", pop, err)
		}
		if empty, _ := repo.Neighbors(ctx, "b1", nil); len(empty) != 0 {
			t.Error("Neighbors sin ids")
		}
		if empty, _ := repo.Popular(ctx, "b1", nil); len(empty) != 0 {
			t.Error("Popular sin claves")
		}

		// Un modelo nuevo convive con el anterior hasta que se borra.
		if err := repo.Save(ctx, "b2", map[string][]domain.Neighbor{"i1": many[:1]}, map[string][]string{"all": {"z"}}, t0); err != nil {
			t.Fatal(err)
		}
		if old, _ := repo.Neighbors(ctx, "b1", []string{"i1"}); len(old["i1"]) != domain.Neighbors {
			t.Error("el modelo anterior debe seguir legible antes de borrarlo")
		}
		if err := repo.DeleteOtherBuilds(ctx, "b2"); err != nil {
			t.Fatal(err)
		}
		if old, _ := repo.Neighbors(ctx, "b1", []string{"i1"}); len(old) != 0 {
			t.Error("quedó el modelo anterior")
		}
		if cur, _ := repo.Popular(ctx, "b2", []string{"all"}); len(cur["all"]) != 1 {
			t.Error("se borró el modelo vigente")
		}
	})

	t.Run("lecturas de otras specs", func(t *testing.T) {
		items := db.Collection(itemsCollection)
		users := db.Collection(usersCollection)
		bookings := db.Collection(bookingsCollection)
		i1, i2, i3 := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
		u1, u2 := bson.NewObjectID(), bson.NewObjectID()
		mustInsert(t, items,
			bson.D{{Key: "_id", Value: i1}, {Key: "ownerId", Value: "o1"}, {Key: "title", Value: "Taladro"}, {Key: "category", Value: "herramientas_electricas"},
				{Key: "modality", Value: "rent"}, {Key: "pricePerDayCents", Value: int64(8000)}, {Key: "status", Value: "active"},
				{Key: "zone", Value: bson.D{{Key: "municipality", Value: "Guadalajara"}}}},
			bson.D{{Key: "_id", Value: i2}, {Key: "ownerId", Value: "o2"}, {Key: "title", Value: "Escalera"}, {Key: "status", Value: "paused"}},
			bson.D{{Key: "_id", Value: i3}, {Key: "ownerId", Value: "o2"}, {Key: "title", Value: "Sierra"}, {Key: "status", Value: "active"}},
		)
		mustInsert(t, users,
			bson.D{{Key: "_id", Value: u1}, {Key: "status", Value: "active"}},
			bson.D{{Key: "_id", Value: u2}, {Key: "status", Value: "deactivated"}},
		)
		mustInsert(t, bookings,
			bson.D{{Key: "requesterId", Value: "r1"}, {Key: "itemId", Value: i1.Hex()}, {Key: "status", Value: "completed"}, {Key: "createdAt", Value: t0}},
			bson.D{{Key: "requesterId", Value: "r1"}, {Key: "itemId", Value: i3.Hex()}, {Key: "status", Value: "pending"}, {Key: "createdAt", Value: t0.Add(time.Hour)}},
			bson.D{{Key: "requesterId", Value: "r2"}, {Key: "itemId", Value: i2.Hex()}, {Key: "status", Value: "in_progress"}, {Key: "createdAt", Value: t0.Add(-100 * 24 * time.Hour)}},
		)
		before := counts(t, items, users, bookings)

		src := NewItemSource(db)
		owner, status, found, err := src.ItemState(ctx, i2.Hex())
		if err != nil || !found || owner != "o2" || status != "paused" {
			t.Errorf("ItemState = %s %s %v %v", owner, status, found, err)
		}
		for _, id := range []string{bson.NewObjectID().Hex(), "no-es-id"} {
			if _, _, found, err := src.ItemState(ctx, id); err != nil || found {
				t.Errorf("ItemState(%s) = %v %v", id, found, err)
			}
		}
		feats, err := src.ActiveFeatures(ctx)
		if err != nil || len(feats) != 2 {
			t.Fatalf("ActiveFeatures = %d %v", len(feats), err)
		}
		for _, f := range feats {
			if f.ID == i1.Hex() && (f.Municipality != "Guadalajara" || f.PricePerDayCents != 8000 || f.Category != "herramientas_electricas" || f.OwnerID != "o1" || f.Modality != "rent") {
				t.Errorf("rasgos = %+v", f)
			}
		}
		briefs, err := src.Briefs(ctx, []string{i1.Hex(), i2.Hex(), "no-es-id"})
		if err != nil || len(briefs) != 2 || briefs[i2.Hex()].Title != "Escalera" || briefs[i1.Hex()].Category != "herramientas_electricas" {
			t.Errorf("Briefs = %v %v", briefs, err)
		}
		if empty, _ := src.Briefs(ctx, []string{"demo"}); len(empty) != 0 {
			t.Error("Briefs con ids inválidos")
		}
		gone, err := src.Deactivated(ctx, []string{u1.Hex(), u2.Hex(), "demo-001"})
		if err != nil || len(gone) != 1 || gone[0] != u2.Hex() {
			t.Errorf("Deactivated = %v %v", gone, err)
		}
		if none, err := src.Deactivated(ctx, []string{"demo-001"}); err != nil || len(none) != 0 {
			t.Errorf("Deactivated sin ObjectID = %v %v", none, err)
		}

		req := NewRequestSource(db)
		win, err := req.Since(ctx, t0.Add(-90*24*time.Hour), 10)
		if err != nil || len(win) != 2 || win[0].ItemID != i3.Hex() || win[0].Type != domain.EventRequest || win[0].Weight != domain.WeightRequest {
			t.Fatalf("Since = %+v %v", win, err)
		}
		if r, _ := req.Recent(ctx, "r1", time.Time{}, 10); len(r) != 2 {
			t.Errorf("Recent sin borrado = %d", len(r))
		}
		// Tras borrar el historial (t0 + 30 min), la solicitud anterior no cuenta (CA-05).
		if r, _ := req.Recent(ctx, "r1", t0.Add(30*time.Minute), 10); len(r) != 1 || r[0].ItemID != i3.Hex() {
			t.Errorf("Recent tras borrado = %+v", r)
		}
		active, err := req.ActiveItemIDs(ctx, "r1")
		if err != nil || len(active) != 1 || active[0] != i3.Hex() {
			t.Errorf("ActiveItemIDs = %v %v", active, err)
		}

		if after := counts(t, items, users, bookings); after != before {
			t.Errorf("las lecturas cambiaron datos: %v → %v", before, after)
		}
	})
}

func mustInsert(t *testing.T, col *drv.Collection, docs ...any) {
	t.Helper()
	if _, err := col.InsertMany(context.Background(), docs); err != nil {
		t.Fatal(err)
	}
}

func counts(t *testing.T, cols ...*drv.Collection) [3]int64 {
	t.Helper()
	var out [3]int64
	for i, c := range cols {
		n, err := c.CountDocuments(context.Background(), bson.D{})
		if err != nil {
			t.Fatal(err)
		}
		out[i] = n
	}
	return out
}

func assertTTL(t *testing.T, col *drv.Collection, name string, seconds int32) {
	t.Helper()
	cur, err := col.Indexes().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var idx []bson.M
	if err := cur.All(context.Background(), &idx); err != nil {
		t.Fatal(err)
	}
	for _, ix := range idx {
		if ix["name"] == name {
			if ix["expireAfterSeconds"] != seconds {
				t.Errorf("%s: expireAfterSeconds = %v, want %d", name, ix["expireAfterSeconds"], seconds)
			}
			return
		}
	}
	t.Errorf("no existe el índice %s", name)
}
