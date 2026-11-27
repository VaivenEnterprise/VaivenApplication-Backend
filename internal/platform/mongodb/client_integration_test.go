//go:build integration

package mongodb

import (
	"context"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// startMongo levanta un MongoDB 7 real (replica set de un nodo, Art. II.2)
// y devuelve su URI. El contenedor se elimina al terminar la prueba.
func startMongo(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatalf("no se pudo iniciar MongoDB con Docker: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	uri, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	// directConnection: el replica set se anuncia con la IP interna del
	// contenedor, que no siempre es accesible desde el host (Podman en
	// Windows); así el driver solo usa la dirección publicada.
	uri += "&directConnection=true"
	return uri
}

func TestIntegrationConnectAndPing(t *testing.T) {
	uri := startMongo(t)
	ctx := context.Background()

	c, err := Connect(ctx, uri, 30*time.Second)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() {
		if err := c.Disconnect(ctx); err != nil {
			t.Errorf("Disconnect: %v", err)
		}
	}()

	if err := c.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}
	if got := c.Database("vaiven_integration").Name(); got != "vaiven_integration" {
		t.Errorf("Database().Name() = %q", got)
	}
}

// TestIntegrationNilCollectionsAreNotNull comprueba (spec 021, RF-10) que un
// slice o map nil se guarda como [] o {} y que un documento viejo con null se
// sigue leyendo.
func TestIntegrationNilCollectionsAreNotNull(t *testing.T) {
	uri := startMongo(t)
	ctx := context.Background()
	c, err := Connect(ctx, uri, 30*time.Second)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Disconnect(ctx) }()
	col := c.Database("vaiven_integration").Collection("nil_opts")

	type doc struct {
		ID   int               `bson:"_id"`
		List []string          `bson:"list"`
		Map  map[string]string `bson:"map"`
	}
	if _, err := col.InsertOne(ctx, doc{ID: 1}); err != nil {
		t.Fatalf("InsertOne: %v", err)
	}
	var raw bson.Raw
	if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: 1}}).Decode(&raw); err != nil {
		t.Fatalf("FindOne: %v", err)
	}
	if raw.Lookup("list").Type != bson.TypeArray || raw.Lookup("map").Type != bson.TypeEmbeddedDocument {
		t.Errorf("list/map = %v/%v; deben ser array y documento vacíos, no null", raw.Lookup("list").Type, raw.Lookup("map").Type)
	}

	// Un documento viejo con null se lee sin error.
	if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 2}, {Key: "list", Value: nil}, {Key: "map", Value: nil}}); err != nil {
		t.Fatalf("InsertOne viejo: %v", err)
	}
	var old doc
	if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: 2}}).Decode(&old); err != nil || old.List != nil || old.Map != nil {
		t.Errorf("leer documento viejo = %+v, %v", old, err)
	}
}
