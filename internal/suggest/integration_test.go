//go:build integration

// Prueba de integración de la parte A de la spec 014 (T007): el endpoint real
// con router, autenticación JWT, images.Service contra un Cloudinary simulado,
// el contador en MongoDB real (Testcontainers) y un proveedor de IA falso, sin
// red. Cubre CA-01 a CA-06.
package suggest_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	aimongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/adapter/mongo"
	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	authhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/adapter/cloudinary"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
	suggesthttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/suggest/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/suggest/usecase"
)

const (
	itCloud  = "nube-de-prueba"
	itKey    = "clave-publica-de-prueba"
	itSecret = "secreto-de-prueba"
	itBase   = "vaiven/test"
	itOwner  = "6730f1c2a9b3e4d5f6a7b8c9"
	itOther  = "6730f1c2a9b3e4d5f6a7b8d0"
	itLimit  = 3
)

// fakeCloudinary responde GET /resources/image/<type>/<public_id> de la Admin API.
type fakeCloudinary struct {
	mu    sync.Mutex
	known map[string]bool
	down  bool
}

func (f *fakeCloudinary) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1_1/{cloud}/resources/image/{type}/{id...}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		down, ok := f.down, f.known[r.PathValue("id")]
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if down {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"mantenimiento"}}`))
			return
		}
		if u, p, okAuth := r.BasicAuth(); !okAuth || u != itKey || p != itSecret || !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"Resource not found"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"public_id": r.PathValue("id"), "type": "upload", "format": "jpg", "width": 800, "height": 600, "bytes": 9000,
		})
	})
	return mux
}

// fakeAI registra lo que recibe y devuelve una respuesta fija.
type fakeAI struct {
	mu    sync.Mutex
	calls int
	last  aidomain.Request
	out   string
	err   error
}

func (f *fakeAI) Generate(_ context.Context, req aidomain.Request) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = req
	return []byte(f.out), f.err
}

type blockWord string

func (b blockWord) Check(_ context.Context, text string) (moderation.Result, error) {
	return moderation.Result{Allowed: !strings.Contains(strings.ToLower(text), string(b))}, nil
}

type env struct {
	t     *testing.T
	api   *httptest.Server
	cloud *fakeCloudinary
	ai    *fakeAI
	jwt   *security.JWT
}

func mongoDB(t *testing.T) *mongodb.Client {
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
	return client
}

func newEnv(t *testing.T, client *mongodb.Client, dbName string, enabled bool, mod moderation.Checker) *env {
	t.Helper()
	cloud := &fakeCloudinary{known: map[string]bool{}}
	cloudTS := httptest.NewServer(cloud.handler())
	t.Cleanup(cloudTS.Close)
	imgs, err := images.New(images.Config{
		CloudName: itCloud, APIKey: itKey, APISecret: itSecret, BaseFolder: itBase, MaxBytes: imagesdomain.DefaultMaxBytes,
		AdminOptions: []cloudinary.AdminOption{cloudinary.WithAdminBaseURL(cloudTS.URL)},
	})
	if err != nil {
		t.Fatal(err)
	}
	usage := aimongo.NewUsageRepo(client.Database(dbName))
	if err := usage.EnsureIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	ai := &fakeAI{out: `{"title":"Taladro inalámbrico","category":"herramientas_electricas","description":"Con estuche"}`}
	svc := usecase.New(usecase.Config{Enabled: enabled, DailyLimit: itLimit, BaseFolder: itBase},
		usecase.Deps{AI: ai, Images: imgs, Usage: usage, Moderation: mod})
	jwt, err := security.NewJWT("clave-de-prueba-clave-de-prueba-0000", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m, err := suggesthttp.NewModule(suggesthttp.ModuleDeps{
		Service: svc, Authenticate: authhttp.Authenticate(jwt), Log: log, Limiter: redis.NewMemoryLimiter(),
	})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(apihttp.NewRouter(apihttp.Deps{Log: log, Modules: []apihttp.Module{m}}))
	t.Cleanup(api.Close)
	return &env{t: t, api: api, cloud: cloud, ai: ai, jwt: jwt}
}

func (e *env) photo(owner, purpose, name string) string {
	id := itBase + "/" + purpose + "/" + owner + "/" + name
	e.cloud.mu.Lock()
	e.cloud.known[id] = true
	e.cloud.mu.Unlock()
	return id
}

func (e *env) suggest(user, publicID string) (int, map[string]any) {
	e.t.Helper()
	tok, err := e.jwt.Issue(user, "user")
	if err != nil {
		e.t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"imagePublicId": publicID})
	req, _ := http.NewRequest(http.MethodPost, e.api.URL+"/api/v1/ai/listing-suggestions", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func errCode(out map[string]any) string {
	if e, ok := out["error"].(map[string]any); ok {
		c, _ := e["code"].(string)
		return c
	}
	return ""
}

func TestIntegrationListingSuggestions(t *testing.T) {
	client := mongoDB(t)

	t.Run("CA-01 foto propia", func(t *testing.T) {
		e := newEnv(t, client, "ca01", true, nil)
		status, out := e.suggest(itOwner, e.photo(itOwner, "item_photo", "a1"))
		if status != http.StatusOK || out["title"] != "Taladro inalámbrico" || out["category"] != "herramientas_electricas" {
			t.Fatalf("%d %v", status, out)
		}
		if !strings.HasPrefix(e.ai.last.ImageURL, "https://res.cloudinary.com/"+itCloud+"/image/upload/") {
			t.Errorf("la miniatura la construye el servidor: %q", e.ai.last.ImageURL)
		}
	})

	t.Run("CA-02 foto ajena o de evidencia", func(t *testing.T) {
		e := newEnv(t, client, "ca02", true, nil)
		foreign := e.photo(itOther, "item_photo", "b1")
		evidence := e.photo(itOwner, "evidence", "b2")
		for _, id := range []string{foreign, evidence, itBase + "/item_photo/" + itOwner + "/no-existe"} {
			if status, out := e.suggest(itOwner, id); status != http.StatusNotFound || errCode(out) != "not_found" {
				t.Errorf("%s: %d %v", id, status, out)
			}
		}
		if e.ai.calls != 0 {
			t.Errorf("no debe llamar al proveedor: %d llamadas", e.ai.calls)
		}
	})

	t.Run("CA-03 título que el filtro rechaza", func(t *testing.T) {
		e := newEnv(t, client, "ca03", true, blockWord("taladro"))
		status, out := e.suggest(itOwner, e.photo(itOwner, "item_photo", "c1"))
		if status != http.StatusServiceUnavailable || out["title"] != nil {
			t.Errorf("%d %v", status, out)
		}
	})

	t.Run("CA-04 apagada, proveedor caído o Cloudinary caído", func(t *testing.T) {
		e := newEnv(t, client, "ca04a", false, nil)
		if status, out := e.suggest(itOwner, e.photo(itOwner, "item_photo", "d1")); status != http.StatusServiceUnavailable || errCode(out) != "service_unavailable" {
			t.Errorf("apagada: %d %v", status, out)
		}
		e = newEnv(t, client, "ca04b", true, nil)
		e.ai.err = aidomain.ErrUnavailable
		if status, _ := e.suggest(itOwner, e.photo(itOwner, "item_photo", "d2")); status != http.StatusServiceUnavailable {
			t.Errorf("proveedor caído: %d", status)
		}
		e.ai.err = nil
		e.cloud.mu.Lock()
		e.cloud.down = true
		e.cloud.mu.Unlock()
		if status, _ := e.suggest(itOwner, e.photo(itOwner, "item_photo", "d3")); status != http.StatusServiceUnavailable {
			t.Errorf("Cloudinary caído: %d", status)
		}
	})

	t.Run("CA-05 límite diario con el contador real", func(t *testing.T) {
		e := newEnv(t, client, "ca05", true, nil)
		id := e.photo(itOwner, "item_photo", "e1")
		for i := 0; i < itLimit; i++ {
			if status, _ := e.suggest(itOwner, id); status != http.StatusOK {
				t.Fatalf("uso %d: %d", i+1, status)
			}
		}
		status, out := e.suggest(itOwner, id)
		if status != http.StatusTooManyRequests || errCode(out) != "rate_limited" {
			t.Errorf("uso %d: %d %v", itLimit+1, status, out)
		}
		if e.ai.calls != itLimit {
			t.Errorf("llamadas al proveedor = %d, want %d", e.ai.calls, itLimit)
		}
		// Otra persona tiene su propio cupo.
		if status, _ := e.suggest(itOther, e.photo(itOther, "item_photo", "e2")); status != http.StatusOK {
			t.Errorf("otra persona: %d", status)
		}
	})
}

// CA-06: el repositorio no contiene la clave del proveedor. Busca en lo
// versionado y en lo nuevo (sin lo ignorado por .gitignore) cualquier
// asignación de AI_API_KEY con valor. Se excluye este archivo, porque la
// expresión de búsqueda coincidiría consigo misma.
func TestIntegrationNoProviderKeyInRepo(t *testing.T) {
	out, err := exec.Command("git", "grep", "--untracked", "-n", "-E", `AI_API_KEY=[^ \t$"'{]+`,
		"--", ":(top)", ":(top,exclude,glob)**/*.md", ":(top,exclude)internal/suggest/integration_test.go").Output()
	var exitErr *exec.ExitError
	if err != nil && (!errors.As(err, &exitErr) || exitErr.ExitCode() != 1) {
		if _, statErr := os.Stat("../../.git"); statErr != nil {
			t.Skip("sin repositorio git")
		}
		t.Fatalf("git grep: %v", err)
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("hay un valor de AI_API_KEY en el repositorio:\n%s", out)
	}
}
