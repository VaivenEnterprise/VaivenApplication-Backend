//go:build integration

package http

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/contract"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/handler"
	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
)

// Spec 018, C-12 y C-02, de punta a punta: dos réplicas de la API (dos
// módulos y dos routers) sobre el mismo MongoDB y el mismo Redis reales.

// syncBuffer es un búfer seguro para los registros de varias goroutines.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func newReplica(t *testing.T, db *drv.Database, rc *redis.Client, secret string, logs *syncBuffer) *api {
	t.Helper()
	users, refresh := authmongo.NewUserRepo(db), authmongo.NewRefreshRepo(db)
	jwt, err := security.NewJWT(secret, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := security.NewHasher(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewJSONHandler(logs, nil))
	revocations := redis.NewRevocations(rc)
	svc, err := usecase.NewService(usecase.Deps{
		Users: users, Refresh: refresh, Hasher: hasher, Access: jwt,
		NewRefresh: security.NewRefreshToken, HashRefresh: security.HashToken, RefreshTTL: 720 * time.Hour,
		Revocations: revocations, Events: SlogEvents{Log: log},
	})
	if err != nil {
		t.Fatal(err)
	}
	limiter := redis.NewMemoryLimiter()
	mod, err := NewModule(ModuleDeps{Service: svc, Tokens: jwt, Log: log, Limiter: limiter, LimitPerMinute: 1000, Revocations: revocations})
	if err != nil {
		t.Fatal(err)
	}
	router := apihttp.NewRouter(apihttp.Deps{
		Log: log, Pinger: flowClient, Meta: handler.MetaInfo{Name: "vaiven-backend", Version: "test", Environment: "test"},
		Limiter: limiter, LimiterFallback: redis.NewMemoryLimiter(), Modules: []apihttp.Module{mod},
	})
	srv := httptest.NewServer(contract.Wrap(t, router, ""))
	t.Cleanup(srv.Close)
	return &api{t: t, srv: srv}
}

func TestIntegrationRevocationAcrossReplicas(t *testing.T) {
	ctx := context.Background()
	db := flowClient.Database(fmt.Sprintf("auth_rev_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	if _, err := authmongo.EnsureSchemas(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := adaptermongo.EnsureAll(ctx, authmongo.NewUserRepo(db), authmongo.NewRefreshRepo(db)); err != nil {
		t.Fatal(err)
	}
	ctr, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatalf("Redis: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := redis.NewClient(url, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rc.Close() })

	secret := strings.Repeat("k", security.MinSecretLen)
	logs := &syncBuffer{}
	a := newReplica(t, db, rc, secret, logs)
	b := newReplica(t, db, rc, secret, logs)
	const pass = "una-contrasena-larga"

	// 1. Registro en A; el token vale en B.
	var reg authBody
	if st, code := a.call(http.MethodPost, "/api/v1/auth/register", fmt.Sprintf(regBody, "rev@example.com"), "", &reg); st != 201 {
		t.Fatalf("registro = %d %s", st, code)
	}
	if st, _ := b.call(http.MethodGet, "/api/v1/me", "", reg.Tokens.AccessToken, nil); st != 200 {
		t.Fatalf("/me en B = %d", st)
	}

	// 2. Logout en A: el token de acceso deja de valer en B al instante.
	start := time.Now()
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/logout", `{"refreshToken":"`+reg.Tokens.RefreshToken+`"}`, "", nil); st != 204 {
		t.Fatalf("logout = %d", st)
	}
	if st, code := b.call(http.MethodGet, "/api/v1/me", "", reg.Tokens.AccessToken, nil); st != 401 || code != "unauthorized" {
		t.Fatalf("token tras logout en B = %d %s", st, code)
	}
	t.Logf("revocación por logout efectiva en la otra réplica en %v (objetivo ≤ 60 s)", time.Since(start))

	// 3. Otra sesión del mismo usuario sigue viva; la reutilización de su
	// refresh la corta en ambas réplicas.
	var s2 authBody
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/login", `{"email":"rev@example.com","password":"`+pass+`"}`, "", &s2); st != 200 {
		t.Fatalf("login = %d", st)
	}
	var s3 authBody
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/login", `{"email":"rev@example.com","password":"`+pass+`"}`, "", &s3); st != 200 {
		t.Fatalf("segundo login = %d", st)
	}
	var rotated authBody
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"`+s2.Tokens.RefreshToken+`"}`, "", &rotated); st != 200 {
		t.Fatalf("refresh = %d", st)
	}
	if st, _ := b.call(http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"`+s2.Tokens.RefreshToken+`"}`, "", nil); st != 401 {
		t.Fatalf("reutilizar en B = %d", st)
	}
	if st, _ := a.call(http.MethodGet, "/api/v1/me", "", rotated.Tokens.AccessToken, nil); st != 401 {
		t.Errorf("acceso de la sesión reutilizada = %d, quiero 401", st)
	}
	if st, _ := b.call(http.MethodGet, "/api/v1/me", "", s3.Tokens.AccessToken, nil); st != 200 {
		t.Errorf("la otra sesión del usuario debe seguir = %d", st)
	}

	// 4. Baja de cuenta en B: todo token anterior deja de valer en A.
	if st, _ := b.call(http.MethodDelete, "/api/v1/me", `{"password":"`+pass+`"}`, s3.Tokens.AccessToken, nil); st != 204 {
		t.Fatalf("baja = %d", st)
	}
	if st, _ := a.call(http.MethodGet, "/api/v1/me", "", s3.Tokens.AccessToken, nil); st != 401 {
		t.Errorf("token tras la baja en A = %d", st)
	}

	// 5. Eventos sin datos personales ni tokens.
	out := logs.String()
	for _, ev := range []string{"auth.refresh_reuse", "auth.family_revoked", "auth.revoked_token_used"} {
		if !strings.Contains(out, `"event":"`+ev+`"`) {
			t.Errorf("falta el evento %s", ev)
		}
	}
	for _, leak := range []string{"rev@example.com", pass, reg.Tokens.AccessToken, reg.Tokens.RefreshToken, "Bearer "} {
		if strings.Contains(out, leak) {
			t.Errorf("los registros filtran %q", leak[:min(len(leak), 12)])
		}
	}

	// 6. Redis caído: una ruta protegida falla cerrada (503); una pública no
	// consulta el almacén.
	var s4 authBody
	if st, _ := a.call(http.MethodPost, "/api/v1/auth/register", fmt.Sprintf(regBody, "rev2@example.com"), "", &s4); st != 201 {
		t.Fatalf("registro 2 = %d", st)
	}
	if err := ctr.Stop(ctx, nil); err != nil {
		t.Fatal(err)
	}
	down := time.Now()
	if st, code := a.call(http.MethodGet, "/api/v1/me", "", s4.Tokens.AccessToken, nil); st != 503 || code != "service_unavailable" {
		t.Errorf("sin Redis = %d %s, quiero 503 (falla cerrada)", st, code)
	}
	t.Logf("503 con Redis caído en %v", time.Since(down))
	if st, _ := a.call(http.MethodGet, "/api/v1/meta", "", "", nil); st != 200 {
		t.Errorf("ruta pública sin Redis = %d", st)
	}
	if !strings.Contains(logs.String(), `"event":"auth.revocation_unavailable"`) {
		t.Error("falta el evento de almacén caído")
	}
}
