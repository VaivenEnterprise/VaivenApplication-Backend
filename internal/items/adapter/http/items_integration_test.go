//go:build integration

package http

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
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

// itImages simula images.Service: acepta cualquier publicId de quien llama.
type itImages struct{}

func (itImages) Signature(context.Context, string, images.Purpose) (images.UploadSignature, error) {
	return images.UploadSignature{}, nil
}

func (itImages) Resolve(_ context.Context, _ string, _ images.Purpose, _ string, refs []images.Ref) ([]images.Image, error) {
	out := make([]images.Image, 0, len(refs))
	for _, r := range refs {
		out = append(out, images.Image{PublicID: r.PublicID, Width: 800, Height: 600, Format: "jpg", Bytes: 1000, Delivery: images.DeliveryPublic})
	}
	return out, nil
}

func (itImages) URLs(img images.Image) (images.URLs, error) {
	return images.URLs{URL: "https://img/" + img.PublicID, ThumbURL: "https://thumb/" + img.PublicID}, nil
}

// itModeration rechaza "prohibido" (simula la 011 para CA-04).
type itModeration struct{}

func (itModeration) Check(_ context.Context, text string) (moderation.Result, error) {
	if strings.Contains(text, "prohibido") {
		return moderation.Result{Reason: "contiene contenido prohibido"}, nil
	}
	return moderation.Result{Allowed: true}, nil
}

type itUsers struct{}

func (itUsers) PublicUsers(_ context.Context, ids []string) (map[string]users.PublicUser, error) {
	out := map[string]users.PublicUser{}
	for _, id := range ids {
		out[id] = users.PublicUser{ID: id, DisplayName: "Persona " + id}
	}
	return out, nil
}

// itBookings simula la spec 007: el objeto con reservas activas (CA-07).
type itBookings struct{ busyItem *string }

func (b itBookings) HasActiveBookings(_ context.Context, id string) (bool, error) {
	return *b.busyItem == id, nil
}

func newIntegrationRouter(t *testing.T) (http.Handler, *string) {
	t.Helper()
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("items_http_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	if _, err := itemsmongo.EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := itemsmongo.NewItemRepo(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	busy := new(string)
	svc, err := usecase.NewService(usecase.Deps{
		Items: repo, Images: itImages{}, Moderation: itModeration{}, Users: itUsers{}, Bookings: itBookings{busyItem: busy},
	})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewModule(ModuleDeps{Service: svc, Authenticate: testAuth, RequireActive: testActive, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	m.Register(r)
	return r, busy
}

func call(t *testing.T, h http.Handler, user, method, path, body string) (int, map[string]any) {
	t.Helper()
	e := tenv{router: h}
	rec, out := e.do(t, method, path, body, "X-User", user)
	return rec.Code, out
}

func TestIntegrationItemLifecycle(t *testing.T) {
	h, busy := newIntegrationRouter(t)
	const ana, beto = "ana", "beto"

	// CA-01: préstamo con precio → 400 en pricePerDayCents.
	loan := strings.NewReplacer(`"modality": "rent"`, `"modality": "loan"`, `"priceSource": "suggested"`, `"priceSource": "manual"`).Replace(validCreate)
	if code, out := call(t, h, ana, "POST", "/items", loan); code != 400 || errorFields(out)[0] != "pricePerDayCents" {
		t.Fatalf("CA-01 = %d %v", code, out)
	}
	// CA-04: contenido prohibido → 400 con el motivo.
	if code, out := call(t, h, ana, "POST", "/items", strings.Replace(validCreate, `"Taladro"`, `"Algo prohibido"`, 1)); code != 400 || errorFields(out)[0] != "title" {
		t.Fatalf("CA-04 = %d %v", code, out)
	}

	// Crear (CA-03: lat 19.123456 → 19.12).
	code, item := call(t, h, ana, "POST", "/items", validCreate)
	if code != 201 || item["approxLocation"].(map[string]any)["lat"] != 19.12 || item["version"] != float64(1) {
		t.Fatalf("crear = %d %v", code, item)
	}
	id := item["id"].(string)
	path := "/items/" + id

	// Leer: Beto no ve el valor declarado.
	code, got := call(t, h, beto, "GET", path, "")
	if code != 200 || got["declaredValueCents"] != nil || got["owner"].(map[string]any)["id"] != ana {
		t.Fatalf("leer = %d %v", code, got)
	}

	// CA-05: Beto no puede editar.
	if code, _ := call(t, h, beto, "PATCH", path, `{"version": 1, "title": "Mío"}`); code != 403 {
		t.Errorf("CA-05 = %d", code)
	}
	// Editar con versión correcta; luego con la vieja → 409.
	code, got = call(t, h, ana, "PATCH", path, `{"version": 1, "pricePerDayCents": 3500, "priceSource": "manual", "approxLocation": null}`)
	if code != 200 || got["pricePerDayCents"] != float64(3500) || got["version"] != float64(2) || got["approxLocation"] != nil {
		t.Fatalf("editar = %d %v", code, got)
	}
	if code, _ := call(t, h, ana, "PATCH", path, `{"version": 1, "title": "Viejo"}`); code != 409 {
		t.Errorf("versión vieja = %d", code)
	}

	// Pausar; CA-06: Beto ya no lo ve.
	if code, got = call(t, h, ana, "POST", path+"/pause", ""); code != 200 || got["status"] != "paused" {
		t.Fatalf("pausar = %d %v", code, got)
	}
	if code, _ := call(t, h, beto, "GET", path, ""); code != 404 {
		t.Errorf("CA-06 = %d", code)
	}
	code, page := call(t, h, ana, "GET", "/me/items?status=paused", "")
	if code != 200 || len(page["items"].([]any)) != 1 || page["nextCursor"] != nil {
		t.Errorf("mis pausados = %d %v", code, page)
	}
	if code, page = call(t, h, beto, "GET", "/me/items", ""); code != 200 || len(page["items"].([]any)) != 0 {
		t.Errorf("objetos de Beto = %d %v", code, page)
	}

	// Reanudar.
	if code, got = call(t, h, ana, "POST", path+"/resume", ""); code != 200 || got["status"] != "active" || got["version"] != float64(4) {
		t.Fatalf("reanudar = %d %v", code, got)
	}

	// CA-07: con reserva activa no se elimina.
	*busy = id
	if code, _ := call(t, h, ana, "DELETE", path, ""); code != 409 {
		t.Errorf("CA-07 = %d", code)
	}
	*busy = ""
	if code, _ := call(t, h, beto, "DELETE", path, ""); code != 403 {
		t.Errorf("Beto elimina = %d", code)
	}
	if code, _ := call(t, h, ana, "DELETE", path, ""); code != 204 {
		t.Fatalf("eliminar = %d", code)
	}
	// Eliminado: 404 para todos y fuera de la lista.
	if code, _ := call(t, h, ana, "GET", path, ""); code != 404 {
		t.Errorf("leer eliminado = %d", code)
	}
	if code, page = call(t, h, ana, "GET", "/me/items", ""); code != 200 || len(page["items"].([]any)) != 0 {
		t.Errorf("lista tras eliminar = %v", page)
	}
}

func TestIntegrationPagination(t *testing.T) {
	h, _ := newIntegrationRouter(t)
	for range 5 {
		if code, out := call(t, h, "ana", "POST", "/items", validCreate); code != 201 {
			t.Fatalf("crear = %d %v", code, out)
		}
	}
	seen := map[string]bool{}
	url := "/me/items?limit=2"
	for range 5 {
		code, page := call(t, h, "ana", "GET", url, "")
		if code != 200 {
			t.Fatalf("página = %d %v", code, page)
		}
		for _, it := range page["items"].([]any) {
			id := it.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("objeto repetido entre páginas: %s", id)
			}
			seen[id] = true
		}
		next, _ := page["nextCursor"].(string)
		if next == "" {
			break
		}
		url = "/me/items?limit=2&cursor=" + next
	}
	if len(seen) != 5 {
		t.Errorf("vistos %d objetos, want 5", len(seen))
	}
	if code, out := call(t, h, "ana", "GET", "/items/price-suggestion?declaredValueCents=100000", ""); code != 200 || out["suggestedPricePerDayCents"] != float64(3000) {
		t.Errorf("CA-02 = %d %v", code, out)
	}
}
