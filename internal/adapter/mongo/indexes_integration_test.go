//go:build integration

package mongo

import (
	"context"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// itemsRepo es un repositorio de prueba que declara un índice único (CA-09).
type itemsRepo struct {
	col *drv.Collection
}

func (r itemsRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateOne(ctx, drv.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetName("code_unique").SetUnique(true),
	})
	return err
}

func TestIntegrationEnsureAllIsIdempotent(t *testing.T) {
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

	client, err := mongodb.Connect(ctx, uri, 30*time.Second)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	col := client.Database("vaiven_integration").Collection("integration_items")
	repo := itemsRepo{col: col}

	// Dos arranques seguidos: el segundo no debe fallar ni duplicar el índice.
	for i := 1; i <= 2; i++ {
		if err := EnsureAll(ctx, repo); err != nil {
			t.Fatalf("EnsureAll (vez %d): %v", i, err)
		}
	}

	specs, err := col.Indexes().ListSpecifications(ctx)
	if err != nil {
		t.Fatalf("ListSpecifications: %v", err)
	}
	var found int
	for _, s := range specs {
		if s.Name == "code_unique" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("índice code_unique encontrado %d veces, se esperaba 1 (índices: %d)", found, len(specs))
	}
}
