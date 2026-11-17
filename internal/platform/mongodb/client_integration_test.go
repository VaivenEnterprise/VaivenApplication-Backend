//go:build integration

package mongodb

import (
	"context"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
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
