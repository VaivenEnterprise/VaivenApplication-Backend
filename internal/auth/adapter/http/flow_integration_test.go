//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/handler"
	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
)

// Prueba de punta a punta del flujo de sesión (spec 003, T012) con MongoDB
// real (testcontainers; con Podman ver README), argon2id y JWT reales y el
// router completo de la API. Cubre CA-01 a CA-07.

var flowClient *mongodb.Client

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "no se pudo iniciar MongoDB con Docker o Podman:", err)
		os.Exit(1)
	}
	uri, err := c.ConnectionString(ctx)
	if err == nil {
		flowClient, err = mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "MongoDB:", err)
		_ = c.Terminate(ctx)
		os.Exit(1)
	}
	code := m.Run()
	_ = flowClient.Disconnect(ctx)
	_ = c.Terminate(ctx)
	os.Exit(code)
}

type api struct {
	t   *testing.T
	srv *httptest.Server
}

// newAPI arma la API como cmd/api, con una base nueva. accessTTL permite
// probar el vencimiento del token de acceso (CA-04).
func newAPI(t *testing.T, accessTTL time.Duration) *api {
	t.Helper()
	db := flowClient.Database(fmt.Sprintf("flow_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	return newAPIOn(t, db, "clave-de-prueba-clave-de-prueba-0000", accessTTL)
}

// newAPIOn arma la API sobre una base existente con el secreto de JWT dado.
// Dos llamadas sobre la misma base simulan reiniciar la API tras rotar
// JWT_ACCESS_SECRET (SEC-002).
func newAPIOn(t *testing.T, db *drv.Database, jwtSecret string, accessTTL time.Duration) *api {
	t.Helper()
	ctx := context.Background()

	// Como cmd/api: validación $jsonSchema antes de los índices.
	if _, err := authmongo.EnsureSchemas(ctx, db); err != nil {
		t.Fatal(err)
	}
	users, refresh := authmongo.NewUserRepo(db), authmongo.NewRefreshRepo(db)
	if err := adaptermongo.EnsureAll(ctx, users, refresh); err != nil {
		t.Fatal(err)
	}
	jwt, err := security.NewJWT(jwtSecret, accessTTL)
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := security.NewHasher(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := usecase.NewService(usecase.Deps{
		Users: users, Refresh: refresh, Hasher: hasher, Access: jwt,
		NewRefresh: security.NewRefreshToken, HashRefresh: security.HashToken, RefreshTTL: 720 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	limiter := redis.NewMemoryLimiter()
	mod, err := NewModule(ModuleDeps{Service: svc, Tokens: jwt, Log: log, Limiter: limiter, LimitPerMinute: 10})
	if err != nil {
		t.Fatal(err)
	}
	router := apihttp.NewRouter(apihttp.Deps{
		Log: log, Pinger: flowClient, Meta: handler.MetaInfo{Name: "vaiven-backend"},
		Limiter: limiter, LimiterFallback: redis.NewMemoryLimiter(),
		Modules: []apihttp.Module{mod, adminProbe{auth: mod.Authenticate()}},
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &api{t: t, srv: srv}
}

// adminProbe es una ruta de administración de prueba (CA-07).
type adminProbe struct {
	auth func(http.Handler) http.Handler
}

func (adminProbe) Name() string { return "admin-probe" }

func (p adminProbe) Register(r chi.Router) {
	r.With(p.auth, RequireRole(domain.RoleAdmin)).Get("/admin/probe", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// call hace la petición y decodifica el JSON en out (si no es nil).
func (a *api) call(method, path, body, access string, out any) (int, string) {
	a.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, a.srv.URL+path, rd)
	if err != nil {
		a.t.Fatal(err)
	}
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	res, err := a.srv.Client().Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if out != nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, out)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &env)
	return res.StatusCode, env.Error.Code
}

type authBody struct {
	User struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"user"`
	Tokens struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
	} `json:"tokens"`
}

const regBody = `{"email":"%s","password":"una-contrasena-larga","displayName":"Ana","acceptTerms":true,"isAdult":true}`

func TestIntegrationSessionFlow(t *testing.T) {
	a := newAPI(t, 15*time.Minute)

	// CA-01: registro → 201 con tokens → /me.
	var reg authBody
	if st, code := a.call(http.MethodPost, "/api/v1/auth/register", fmt.Sprintf(regBody, " Ana@Example.com"), "", &reg); st != http.StatusCreated {
		t.Fatalf("registro = %d %s", st, code)
	}
	if reg.User.Email != "ana@example.com" || reg.Tokens.AccessToken == "" || reg.Tokens.RefreshToken == "" || reg.Tokens.ExpiresIn != 900 {
		t.Fatalf("registro = %+v", reg)
	}
	var me struct{ ID, Email string }
	if st, _ := a.call(http.MethodGet, "/api/v1/me", "", reg.Tokens.AccessToken, &me); st != http.StatusOK || me.ID != reg.User.ID {
		t.Fatalf("/me = %d %+v", st, me)
	}

	// CA-02: el mismo correo con otras mayúsculas o espacios → 409, sin crear otra cuenta.
	for _, email := range []string{"ana@example.com", "ANA@EXAMPLE.COM", "  Ana@example.com  "} {
		if st, code := a.call(http.MethodPost, "/api/v1/auth/register", fmt.Sprintf(regBody, email), "", nil); st != http.StatusConflict || code != "conflict" {
			t.Errorf("registro duplicado %q = %d %s, want 409 conflict", email, st, code)
		}
	}
	n, err := flowClient.Database("admin").Client().Database(dbNameOf(t, a)).Collection("users").CountDocuments(context.Background(), map[string]any{})
	if err == nil && n != 1 {
		t.Errorf("cuentas en la base = %d, want 1", n)
	}

	// Perfil público sin datos privados.
	var pub map[string]any
	if st, _ := a.call(http.MethodGet, "/api/v1/users/"+reg.User.ID, "", reg.Tokens.AccessToken, &pub); st != http.StatusOK ||
		pub["displayName"] != "Ana" || pub["email"] != nil || pub["phone"] != nil {
		t.Errorf("perfil público = %d %v", st, pub)
	}

	// CA-03: refresh rota; reutilizar el viejo revoca la familia.
	var r1 authBody
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"`+reg.Tokens.RefreshToken+`"}`, "", &r1); st != http.StatusOK || r1.Tokens.RefreshToken == "" {
		t.Fatalf("refresh = %d", st)
	}
	if st, code := a.call(http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"`+reg.Tokens.RefreshToken+`"}`, "", nil); st != http.StatusUnauthorized || code != "unauthorized" {
		t.Errorf("reutilizar = %d %s, want 401", st, code)
	}
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"`+r1.Tokens.RefreshToken+`"}`, "", nil); st != http.StatusUnauthorized {
		t.Errorf("token de la familia revocada = %d, want 401", st)
	}

	// Logout: nueva sesión, se cierra y su refresh deja de servir.
	var login authBody
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/login", `{"email":"ana@example.com","password":"una-contrasena-larga"}`, "", &login); st != http.StatusOK {
		t.Fatalf("login = %d", st)
	}
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/logout", `{"refreshToken":"`+login.Tokens.RefreshToken+`"}`, "", nil); st != http.StatusNoContent {
		t.Errorf("logout = %d", st)
	}
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"`+login.Tokens.RefreshToken+`"}`, "", nil); st != http.StatusUnauthorized {
		t.Errorf("refresh tras logout = %d", st)
	}

	// CA-07: una persona sin rol admin recibe 403 en una ruta de admin.
	if st, code := a.call(http.MethodGet, "/api/v1/admin/probe", "", login.Tokens.AccessToken, nil); st != http.StatusForbidden || code != "forbidden" {
		t.Errorf("ruta admin = %d %s, want 403", st, code)
	}

	// CA-05: baja (sin reservas) → 204; ya no inicia sesión y el correo queda libre.
	if st, code := a.call(http.MethodDelete, "/api/v1/me", `{"password":"incorrecta"}`, login.Tokens.AccessToken, nil); st != http.StatusForbidden {
		t.Errorf("baja con contraseña incorrecta = %d %s", st, code)
	}
	if st, code := a.call(http.MethodDelete, "/api/v1/me", `{"password":"una-contrasena-larga"}`, login.Tokens.AccessToken, nil); st != http.StatusNoContent {
		t.Fatalf("baja = %d %s", st, code)
	}
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/login", `{"email":"ana@example.com","password":"una-contrasena-larga"}`, "", nil); st != http.StatusUnauthorized {
		t.Errorf("login tras la baja = %d, want 401", st)
	}
	if st, _ := a.call(http.MethodGet, "/api/v1/me", "", login.Tokens.AccessToken, nil); st != http.StatusUnauthorized {
		t.Errorf("/me tras la baja = %d, want 401", st)
	}
	if st, _ := a.call(http.MethodGet, "/api/v1/users/"+reg.User.ID, "", "", nil); st != http.StatusUnauthorized {
		t.Errorf("perfil sin token = %d, want 401", st)
	}

	// CA-06: el 11.º intento por minuto desde la misma IP → 429. Ya se usaron
	// 10: 1 registro + 3 duplicados + 4 refresh + 2 login. logout, /me y
	// DELETE /me no cuentan.
	if st, code := a.call(http.MethodPost, "/api/v1/auth/register", fmt.Sprintf(regBody, "ana@example.com"), "", nil); st != http.StatusTooManyRequests || code != "rate_limited" {
		t.Errorf("11.º intento = %d %s, want 429", st, code)
	}
}

func TestIntegrationEmailFreedAfterDeactivation(t *testing.T) { // POL-17
	a := newAPI(t, 15*time.Minute)
	var reg authBody
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/register", fmt.Sprintf(regBody, "carla@example.com"), "", &reg); st != http.StatusCreated {
		t.Fatalf("registro = %d", st)
	}
	if st, _ := a.call(http.MethodDelete, "/api/v1/me", `{"password":"una-contrasena-larga"}`, reg.Tokens.AccessToken, nil); st != http.StatusNoContent {
		t.Fatalf("baja = %d", st)
	}
	var again authBody
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/register", fmt.Sprintf(regBody, "CARLA@example.com"), "", &again); st != http.StatusCreated || again.User.ID == reg.User.ID {
		t.Errorf("registrar de nuevo el correo = %d (id %s)", st, again.User.ID)
	}
	var pub map[string]any
	if st, _ := a.call(http.MethodGet, "/api/v1/users/"+reg.User.ID, "", again.Tokens.AccessToken, &pub); st != http.StatusOK || pub["displayName"] != domain.DeletedDisplayName {
		t.Errorf("perfil de la cuenta dada de baja = %d %v", st, pub)
	}
}

func TestIntegrationExpiredAccessToken(t *testing.T) { // CA-04
	a := newAPI(t, time.Second)
	var reg authBody
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/register", fmt.Sprintf(regBody, "beto@example.com"), "", &reg); st != http.StatusCreated {
		t.Fatalf("registro = %d", st)
	}
	time.Sleep(2100 * time.Millisecond)
	if st, code := a.call(http.MethodGet, "/api/v1/me", "", reg.Tokens.AccessToken, nil); st != http.StatusUnauthorized || code != "unauthorized" {
		t.Errorf("/me con token vencido = %d %s, want 401", st, code)
	}
}

// dbNameOf no tiene acceso directo al nombre de la base de newAPI; devuelve
// la base más reciente creada por la prueba.
func dbNameOf(t *testing.T, _ *api) string {
	t.Helper()
	names, err := flowClient.Database("admin").Client().ListDatabaseNames(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	latest := ""
	for _, n := range names {
		if strings.HasPrefix(n, "flow_it_") && n > latest {
			latest = n
		}
	}
	return latest
}
