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

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
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
	if err != nil {
		fmt.Fprintln(os.Stderr, "ConnectionString:", err)
		_ = c.Terminate(ctx)
		os.Exit(1)
	}
	// directConnection: con Podman en Windows la IP interna del replica set no
	// es accesible desde el host.
	testClient, err = mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Connect:", err)
		_ = c.Terminate(ctx)
		os.Exit(1)
	}
	code := m.Run()
	_ = testClient.Disconnect(ctx)
	_ = c.Terminate(ctx)
	os.Exit(code)
}

// newDB devuelve una base exclusiva de la prueba con el validador y los
// índices aplicados.
func newDB(t *testing.T) *drv.Database {
	t.Helper()
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("items_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	if out, err := EnsureSchema(ctx, db); err != nil || out != SchemaCreated {
		t.Fatalf("EnsureSchema = %s, %v; want creada", out, err)
	}
	if err := NewItemRepo(db).EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	return db
}

func newItem(owner string, created time.Time) domain.Item {
	return domain.Item{
		OwnerID:            owner,
		Title:              "Taladro",
		Description:        "Con estuche",
		Category:           "herramientas_electricas",
		Condition:          domain.ConditionGood,
		Modality:           domain.ModalityRent,
		DeclaredValueCents: 100_000,
		PricePerDayCents:   3_000,
		PriceSource:        domain.PriceSuggested,
		Zone:               domain.Zone{Municipality: "Coyoacán"},
		Location:           &domain.Location{Lat: 19.123456, Lng: -99.167},
		Photos:             []images.Image{{PublicID: "vaiven/test/item_photo/u/1", Width: 10, Height: 10, Format: "jpg", Bytes: 100, Delivery: images.DeliveryPublic}},
		Status:             domain.StatusActive,
		CreatedAt:          created,
		UpdatedAt:          created,
		Version:            1,
	}
}

func TestIntegrationItemRepoCRUD(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	repo := NewItemRepo(db)
	now := time.Now().UTC().Truncate(time.Millisecond)

	created, err := repo.Create(ctx, newItem("u1", now))
	if err != nil || created.ID == "" {
		t.Fatalf("Create = %+v, %v", created, err)
	}
	got, err := repo.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	// La ubicación se guarda redondeada aunque llegue sin redondear (RF-04).
	if got.Location == nil || *got.Location != (domain.Location{Lat: 19.12, Lng: -99.17}) {
		t.Errorf("Location = %+v", got.Location)
	}
	if len(got.Photos) != 1 || got.Photos[0].Delivery != images.DeliveryPublic || !got.CreatedAt.Equal(now) {
		t.Errorf("ByID = %+v", got)
	}
	var raw struct {
		Location struct {
			Type        string    `bson:"type"`
			Coordinates []float64 `bson:"coordinates"`
		} `bson:"location"`
	}
	_ = db.Collection(ItemsCollection).FindOne(ctx, bson.D{}).Decode(&raw)
	if l := raw.Location; l.Type != "Point" || len(l.Coordinates) != 2 || l.Coordinates[0] != -99.17 || l.Coordinates[1] != 19.12 {
		t.Errorf("location guardada = %+v, want Point [-99.17, 19.12]", l)
	}

	for _, id := range []string{"no-es-hex", bson.NewObjectID().Hex()} {
		if _, err := repo.ByID(ctx, id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ByID(%s) = %v, want ErrNotFound", id, err)
		}
	}

	// Update con la versión correcta: incrementa la versión.
	got.Title = "Taladro nuevo"
	got.Location = nil
	updated, err := repo.Update(ctx, got)
	if err != nil || updated.Version != 2 || updated.Title != "Taladro nuevo" {
		t.Fatalf("Update = %+v, %v", updated, err)
	}
	again, _ := repo.ByID(ctx, got.ID)
	if again.Location != nil || again.Version != 2 {
		t.Errorf("tras Update = %+v", again)
	}
	// Versión vieja (P-05) o dueño distinto → conflicto.
	if _, err := repo.Update(ctx, got); !errors.Is(err, domain.ErrVersionConflict) {
		t.Errorf("Update con versión vieja = %v", err)
	}
	other := again
	other.OwnerID = "u2"
	if _, err := repo.Update(ctx, other); !errors.Is(err, domain.ErrVersionConflict) {
		t.Errorf("Update con otro dueño = %v", err)
	}
	if _, err := repo.Update(ctx, domain.Item{}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Update sin id = %v", err)
	}
}

func TestIntegrationItemRepoListByOwner(t *testing.T) {
	ctx := context.Background()
	repo := NewItemRepo(newDB(t))
	base := time.Now().UTC().Truncate(time.Millisecond)
	statuses := []domain.Status{domain.StatusActive, domain.StatusPaused, domain.StatusRemoved, domain.StatusActive, domain.StatusBlocked}
	var ids []string
	for i, s := range statuses {
		it := newItem("u1", base.Add(time.Duration(i)*time.Second))
		if i == 4 {
			it.CreatedAt = base.Add(3 * time.Second) // empate con el índice 3
		}
		it.Status = s
		c, err := repo.Create(ctx, it)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	if _, err := repo.Create(ctx, newItem("u2", base)); err != nil {
		t.Fatal(err)
	}
	listable := []domain.Status{domain.StatusActive, domain.StatusPaused, domain.StatusBlocked}

	// Páginas de 2: orden createdAt desc y, en empate, _id desc; sin removed
	// ni objetos de otra persona.
	var seen []string
	var after *domain.Cursor
	for page := 0; page < 5; page++ {
		items, err := repo.ListByOwner(ctx, "u1", listable, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			seen = append(seen, it.ID)
		}
		if len(items) < 2 {
			break
		}
		last := items[len(items)-1]
		after = &domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	want := []string{ids[4], ids[3], ids[1], ids[0]}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Errorf("orden = %v, want %v", seen, want)
	}

	paused, err := repo.ListByOwner(ctx, "u1", []domain.Status{domain.StatusPaused}, nil, 10)
	if err != nil || len(paused) != 1 || paused[0].ID != ids[1] {
		t.Errorf("solo pausados = %v, %v", paused, err)
	}
	if _, err := repo.ListByOwner(ctx, "u1", listable, &domain.Cursor{ID: "x"}, 10); err == nil {
		t.Error("cursor con id inválido debe fallar")
	}
}

func TestIntegrationItemsSchemaAndIndexes(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)

	// Segunda vez: sin cambios (no intenta collMod).
	if out, err := EnsureSchema(ctx, db); err != nil || out != SchemaUnchanged {
		t.Errorf("EnsureSchema otra vez = %s, %v", out, err)
	}
	// Un validador viejo se reemplaza (con permisos de admin en la prueba).
	if err := db.RunCommand(ctx, bson.D{{Key: "collMod", Value: ItemsCollection}, {Key: "validator", Value: bson.M{}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if out, err := EnsureSchema(ctx, db); err != nil || out != SchemaUpdated {
		t.Errorf("EnsureSchema con validador viejo = %s, %v", out, err)
	}

	// El validador rechaza dinero fuera de rango y documentos incompletos.
	col := db.Collection(ItemsCollection)
	bad, _ := toDoc(newItem("u1", time.Now()))
	bad.DeclaredValueCents = 10
	for name, doc := range map[string]any{"valor declarado": bad, "incompleto": bson.M{"title": "x"}} {
		_, err := col.InsertOne(ctx, doc)
		var we drv.WriteException
		if !errors.As(err, &we) || !we.HasErrorCode(121) {
			t.Errorf("%s: insert = %v, want 121", name, err)
		}
	}

	cur, err := col.Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var idx []bson.M
	_ = cur.All(ctx, &idx)
	names := map[string]bool{}
	for _, i := range idx {
		names[fmt.Sprint(i["name"])] = true
	}
	for _, n := range []string{"owner_status_createdAt", "location_2dsphere", "status_category_createdAt"} {
		if !names[n] {
			t.Errorf("falta el índice %s (hay %v)", n, names)
		}
	}
}
