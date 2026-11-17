package mongodb

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestConnectRejectsInvalidParams(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		timeout time.Duration
	}{
		{"URI vacía", "", time.Second},
		{"URI solo espacios", "   ", time.Second},
		{"esquema no mongodb", "http://localhost:27017", time.Second},
		{"URI mal formada", "mongodb://", time.Second},
		{"timeout cero", "mongodb://localhost:27017", 0},
		{"timeout negativo", "mongodb://localhost:27017", -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Connect(context.Background(), tt.uri, tt.timeout)
			if err == nil {
				_ = c.Disconnect(context.Background())
				t.Fatal("Connect: se esperaba error")
			}
			if !errors.Is(err, ErrInvalidParams) {
				t.Errorf("Connect: error %q no envuelve ErrInvalidParams", err)
			}
		})
	}
}

func TestConnectNoServer(t *testing.T) {
	// Puerto 1 en localhost: nadie escucha, así que la selección de servidor
	// agota el tiempo sin red externa.
	start := time.Now()
	c, err := Connect(context.Background(), "mongodb://127.0.0.1:1/?directConnection=true", 300*time.Millisecond)
	if err == nil {
		_ = c.Disconnect(context.Background())
		t.Fatal("Connect: se esperaba error sin servidor")
	}
	if errors.Is(err, ErrInvalidParams) {
		t.Errorf("Connect: sin servidor no es un parámetro inválido: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "mongodb: ping") {
		t.Errorf("Connect: error %q sin el contexto «mongodb: ping»", err)
	}
	if errors.Unwrap(err) == nil {
		t.Errorf("Connect: el error %q no envuelve la causa con %%w", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Connect tardó %v; debe respetar el timeout", d)
	}
}

// TestConnectAndPingReal solo corre con un MongoDB disponible
// (por ejemplo `make up` y VAIVEN_TEST_MONGO_URI=mongodb://localhost:27017/?replicaSet=rs0).
// La prueba de integración formal va en T016.
func TestConnectAndPingReal(t *testing.T) {
	uri := os.Getenv("VAIVEN_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("VAIVEN_TEST_MONGO_URI no definida")
	}
	ctx := context.Background()

	c, err := Connect(ctx, uri, 10*time.Second)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Disconnect(ctx); err != nil {
			t.Errorf("Disconnect: %v", err)
		}
	})

	if err := c.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}
	if db := c.Database("vaiven_test"); db.Name() != "vaiven_test" {
		t.Errorf("Database().Name() = %q", db.Name())
	}
}
