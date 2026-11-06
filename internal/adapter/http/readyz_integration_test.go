//go:build integration

package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/handler"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// TestIntegrationReadyz cubre CA-04 (Mongo activo → 200) y CA-03 (Mongo
// detenido → 503 service_unavailable con requestId) usando el router real.
func TestIntegrationReadyz(t *testing.T) {
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

	srv := httptest.NewServer(NewRouter(Deps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Pinger: client,
		Meta:   handler.MetaInfo{Name: "vaiven-api", Version: "test", Environment: "test"},
	}))
	t.Cleanup(srv.Close)

	// CA-04: con MongoDB activo, 200.
	if code, _ := getReadyz(t, srv.URL); code != http.StatusOK {
		t.Fatalf("/readyz con Mongo activo = %d, want 200", code)
	}

	// CA-03: tras detener MongoDB, 503 con el envoltorio de error.
	stopTimeout := 10 * time.Second
	if err := c.Stop(ctx, &stopTimeout); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	code, body := getReadyz(t, srv.URL)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz con Mongo detenido = %d, want 503", code)
	}
	if body.Error.Code != "service_unavailable" {
		t.Errorf("error.code = %q, want service_unavailable", body.Error.Code)
	}
	if body.Error.RequestID == "" {
		t.Error("error.requestId vacío")
	}
}

type readyzErrorBody struct {
	Error struct {
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	} `json:"error"`
}

func getReadyz(t *testing.T, base string) (int, readyzErrorBody) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/readyz", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer res.Body.Close()

	var body readyzErrorBody
	if res.StatusCode != http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("respuesta de error no es JSON: %v", err)
		}
	}
	return res.StatusCode, body
}
