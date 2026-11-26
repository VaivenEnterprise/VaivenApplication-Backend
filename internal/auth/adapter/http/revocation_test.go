package http

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/revocation"
)

// Spec 018, C-12 y C-02: revocación en el middleware y eventos.

type fakeChecker struct {
	revoked map[string]bool // por sesión o por jti
	err     error
	seen    []revocation.Access
}

func (f *fakeChecker) IsAccessRevoked(_ context.Context, a revocation.Access) (bool, error) {
	f.seen = append(f.seen, a)
	if f.err != nil {
		return false, f.err
	}
	return f.revoked[a.SessionID] || f.revoked[a.TokenID], nil
}

func revJWT(t *testing.T) *security.JWT {
	t.Helper()
	j, err := security.NewJWT(strings.Repeat("s", security.MinSecretLen), 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func callProtected(h http.Handler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthenticateWithRevocation(t *testing.T) {
	j := revJWT(t)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	chk := &fakeChecker{revoked: map[string]bool{"fam-cerrada": true}}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	h := AuthenticateWith(j, chk, log)(ok)

	live, _ := j.IssueSession("u1", "user", "fam-viva")
	if rec := callProtected(h, live); rec.Code != 204 {
		t.Fatalf("sesión viva = %d", rec.Code)
	}
	a := chk.seen[0]
	if a.UserID != "u1" || a.SessionID != "fam-viva" || a.TokenID == "" || a.IssuedAt.IsZero() {
		t.Errorf("consulta = %+v", a)
	}
	closed, _ := j.IssueSession("u1", "user", "fam-cerrada")
	if rec := callProtected(h, closed); rec.Code != 401 || !strings.Contains(rec.Body.String(), `"unauthorized"`) {
		t.Errorf("sesión cerrada = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(buf.String(), `"event":"auth.revoked_token_used"`) {
		t.Errorf("falta el evento: %s", buf.String())
	}
	if strings.Contains(buf.String(), closed) {
		t.Error("el evento filtra el token")
	}

	// Almacén caído: falla cerrada con 503, nunca acepta el token.
	chk.err = errors.New("redis caído")
	rec := callProtected(h, live)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"service_unavailable"`) {
		t.Errorf("sin almacén = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(buf.String(), `"event":"auth.revocation_unavailable"`) {
		t.Error("falta el evento de almacén caído")
	}

	// Un token inválido ni siquiera consulta el almacén.
	n := len(chk.seen)
	if rec := callProtected(h, "basura"); rec.Code != 401 || len(chk.seen) != n {
		t.Errorf("token inválido = %d", rec.Code)
	}
}

func TestAuthenticateWithoutCheckerKeepsBehavior(t *testing.T) {
	j := revJWT(t)
	tok, _ := j.Issue("u1", "user")
	h := Authenticate(j)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	if rec := callProtected(h, tok); rec.Code != 204 {
		t.Errorf("sin revocación = %d", rec.Code)
	}
}

func TestRequireRoleEmitsForbiddenEvent(t *testing.T) {
	j := revJWT(t)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	lookup := func(_ context.Context, id string) (domain.User, error) {
		return domain.User{ID: id, Role: domain.RoleUser, Status: domain.StatusActive}, nil
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	h := Authenticate(j)(RequireRole(domain.RoleAdmin, lookup, log)(ok))

	user, _ := j.Issue("u1", "user")
	if rec := callProtected(h, user); rec.Code != 403 {
		t.Fatalf("usuario común = %d", rec.Code)
	}
	// Token que dice admin pero la base dice user (AUD-017): severidad alta.
	forged, _ := j.Issue("u2", "admin")
	if rec := callProtected(h, forged); rec.Code != 403 {
		t.Fatalf("rol no confirmado = %d", rec.Code)
	}
	out := buf.String()
	if strings.Count(out, `"event":"authz.forbidden"`) != 2 || !strings.Contains(out, `"severity":"high"`) {
		t.Errorf("eventos = %s", out)
	}
	if strings.Contains(out, user) || strings.Contains(out, "Bearer") {
		t.Error("el evento filtra el token")
	}
}

func TestSlogEventsRecord(t *testing.T) {
	var buf bytes.Buffer
	SlogEvents{Log: slog.New(slog.NewJSONHandler(&buf, nil))}.Record(context.Background(),
		usecase.SecurityEvent{Name: "auth.refresh_reuse", Result: "denied", Severity: "high", Subject: "user:u1"})
	for _, want := range []string{`"event":"auth.refresh_reuse"`, `"category":"auth"`, `"subject":"user:u1"`, `"severity":"high"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("falta %s en %s", want, buf.String())
		}
	}
}
