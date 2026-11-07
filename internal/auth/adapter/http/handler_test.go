package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
)

// testSecret es un valor de prueba de baja entropía (no es un secreto real).
const testSecret = "clave-de-prueba-clave-de-prueba-0000"

var errBoom = errors.New("falla simulada con datos internos")

// fakeService registra la última llamada y devuelve lo configurado.
type fakeService struct {
	err        error
	gotUserID  string
	gotChanges usecase.ProfileChanges
	gotToken   string
	gotPwd     string
	gotReg     usecase.RegisterInput
}

var (
	created = time.Date(2026, 11, 6, 18, 0, 0, 0, time.UTC)
	phone   = "+525512345678"
	ana     = domain.User{ID: "6730f1c2a9b3e4d5f6a7b8c9", Email: "ana@example.com", DisplayName: "Ana", Phone: &phone,
		Role: domain.RoleUser, Status: domain.StatusActive, PasswordHash: "hash-secreto", CreatedAt: created}
	tokens = usecase.Tokens{AccessToken: "acc", RefreshToken: "ref", ExpiresIn: 15 * time.Minute}
)

func (f *fakeService) Register(_ context.Context, in usecase.RegisterInput) (usecase.AuthResult, error) {
	f.gotReg = in
	return usecase.AuthResult{User: ana, Tokens: tokens}, f.err
}

func (f *fakeService) Login(context.Context, usecase.LoginInput) (usecase.AuthResult, error) {
	return usecase.AuthResult{User: ana, Tokens: tokens}, f.err
}

func (f *fakeService) Refresh(_ context.Context, token string) (usecase.Tokens, error) {
	f.gotToken = token
	return tokens, f.err
}

func (f *fakeService) Logout(_ context.Context, token string) error {
	f.gotToken = token
	return f.err
}

func (f *fakeService) Me(_ context.Context, id string) (domain.User, error) {
	f.gotUserID = id
	return ana, f.err
}

func (f *fakeService) UpdateProfile(_ context.Context, id string, c usecase.ProfileChanges) (domain.User, error) {
	f.gotUserID, f.gotChanges = id, c
	return ana, f.err
}

func (f *fakeService) Deactivate(_ context.Context, id, pwd string) error {
	f.gotUserID, f.gotPwd = id, pwd
	return f.err
}

func (f *fakeService) PublicProfile(_ context.Context, id string) (domain.PublicUser, error) {
	f.gotUserID = id
	return ana.Public(), f.err
}

type harness struct {
	h   http.Handler
	svc *fakeService
	jwt *security.JWT
	log *bytes.Buffer
}

func newHarness(t *testing.T, limit int) *harness {
	t.Helper()
	jwt, err := security.NewJWT(testSecret, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var logBuf bytes.Buffer
	svc := &fakeService{}
	m, err := NewModule(ModuleDeps{
		Service: svc, Tokens: jwt, Log: slog.New(slog.NewJSONHandler(&logBuf, nil)),
		Limiter: redis.NewMemoryLimiter(), LimitPerMinute: limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "auth" || m.Authenticate() == nil {
		t.Fatal("Name o Authenticate incorrectos")
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Route("/api/v1", m.Register)
	return &harness{h: r, svc: svc, jwt: jwt, log: &logBuf}
}

func (hs *harness) bearer(t *testing.T, userID, role string) string {
	t.Helper()
	tok, err := hs.jwt.Issue(userID, role)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + tok
}

func (hs *harness) do(method, path, body, authz string) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = "10.0.0.1:1234"
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	return rec
}

// errOf devuelve el envoltorio de error del Art. IV.
func errOf(t *testing.T, rec *httptest.ResponseRecorder) (code string, fields []string) {
	t.Helper()
	var env struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"requestId"`
			Details   []struct {
				Field string `json:"field"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code == "" ||
		env.Error.Message == "" || env.Error.RequestID == "" {
		t.Fatalf("no es el envoltorio de error: %v %s", err, rec.Body.String())
	}
	for _, d := range env.Error.Details {
		fields = append(fields, d.Field)
	}
	return env.Error.Code, fields
}

const validRegisterBody = `{"email":"ana@example.com","password":"una-contrasena-larga","displayName":"Ana","acceptTerms":true,"isAdult":true}`

func TestRegisterAndLoginResponses(t *testing.T) {
	hs := newHarness(t, 100)
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/api/v1/auth/register", validRegisterBody, http.StatusCreated},
		{"/api/v1/auth/login", `{"email":"ana@example.com","password":"x"}`, http.StatusOK},
	} {
		rec := hs.do(http.MethodPost, tc.path, tc.body, "")
		if rec.Code != tc.status {
			t.Fatalf("%s = %d %s", tc.path, rec.Code, rec.Body.String())
		}
		var got map[string]map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		u, tk := got["user"], got["tokens"]
		if u["id"] != ana.ID || u["email"] != "ana@example.com" || u["phone"] != phone || u["role"] != "user" ||
			u["ratingAverage"] != nil || u["ratingCount"] != float64(0) || u["createdAt"] != "2026-11-06T18:00:00Z" {
			t.Errorf("user = %v", u)
		}
		if tk["accessToken"] != "acc" || tk["refreshToken"] != "ref" || tk["expiresIn"] != float64(900) {
			t.Errorf("tokens = %v", tk)
		}
		if strings.Contains(rec.Body.String(), "hash-secreto") || strings.Contains(rec.Body.String(), "passwordHash") {
			t.Error("la respuesta expone el hash de la contraseña")
		}
	}
	if !hs.svc.gotReg.AcceptTerms || !hs.svc.gotReg.IsAdult || hs.svc.gotReg.DisplayName != "Ana" {
		t.Errorf("Register recibió %+v", hs.svc.gotReg)
	}
}

func TestBodyValidation(t *testing.T) {
	hs := newHarness(t, 100)
	tests := []struct {
		name, path, body string
		fields           []string
	}{
		{"JSON roto", "/api/v1/auth/register", `{"email":`, nil},
		{"dos objetos", "/api/v1/auth/login", `{"email":"a","password":"b"}{}`, nil},
		{"tipo inválido", "/api/v1/auth/register", `{"email":1}`, []string{"email"}},
		{"faltan campos", "/api/v1/auth/register", `{}`, []string{"email", "password", "displayName"}},
		{"contraseña larga", "/api/v1/auth/login", `{"email":"a@b.mx","password":"` + strings.Repeat("a", 73) + `"}`, []string{"password"}},
		{"refresh vacío", "/api/v1/auth/refresh", `{"refreshToken":""}`, []string{"refreshToken"}},
		{"logout sin cuerpo", "/api/v1/auth/logout", ``, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := hs.do(http.MethodPost, tt.path, tt.body, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
			}
			code, fields := errOf(t, rec)
			if code != "validation_failed" || strings.Join(fields, ",") != strings.Join(tt.fields, ",") {
				t.Errorf("code = %s, campos = %v, want %v", code, fields, tt.fields)
			}
		})
	}
	// Cuerpo mayor que el límite global (lo pone el router; aquí se simula).
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"`+strings.Repeat("a", 100)+`"}`))
	rec := httptest.NewRecorder()
	req.Body = http.MaxBytesReader(rec, req.Body, 10)
	hs.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("cuerpo grande = %d", rec.Code)
	}
}

func TestDomainErrorsMapToStatus(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrEmailTaken, http.StatusConflict, "conflict"},
		{domain.ErrInvalidCredentials, http.StatusUnauthorized, "unauthorized"},
		{domain.ErrWrongPassword, http.StatusForbidden, "forbidden"},
		{domain.ErrUserNotFound, http.StatusNotFound, "not_found"},
		{errBoom, http.StatusInternalServerError, "internal_error"},
	}
	for _, tt := range tests {
		hs := newHarness(t, 100)
		hs.svc.err = tt.err
		rec := hs.do(http.MethodPost, "/api/v1/auth/register", validRegisterBody, "")
		code, _ := errOf(t, rec)
		if rec.Code != tt.status || code != tt.code {
			t.Errorf("%v → %d %s, want %d %s", tt.err, rec.Code, code, tt.status, tt.code)
		}
		if strings.Contains(rec.Body.String(), "datos internos") {
			t.Error("la respuesta expone la causa interna")
		}
		internal := strings.Contains(hs.log.String(), "auth: error interno")
		if internal != (tt.status == http.StatusInternalServerError) {
			t.Errorf("%v: registrado en el log = %v", tt.err, internal)
		}
		if strings.Contains(hs.log.String(), "una-contrasena-larga") {
			t.Error("el log contiene la contraseña (RNF-01)")
		}
	}
}

func TestRefreshAndLogout(t *testing.T) {
	hs := newHarness(t, 100)
	rec := hs.do(http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"rt-1"}`, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tokens":{"accessToken":"acc"`) || hs.svc.gotToken != "rt-1" {
		t.Errorf("refresh = %d %s", rec.Code, rec.Body.String())
	}
	rec = hs.do(http.MethodPost, "/api/v1/auth/logout", `{"refreshToken":"rt-2"}`, "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 || hs.svc.gotToken != "rt-2" {
		t.Errorf("logout = %d %q", rec.Code, rec.Body.String())
	}

	hs.svc.err = domain.ErrInvalidSession
	if rec := hs.do(http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"x"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("refresh inválido = %d", rec.Code)
	}
	hs.svc.err = errBoom
	if rec := hs.do(http.MethodPost, "/api/v1/auth/logout", `{"refreshToken":"x"}`, ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("logout con falla = %d", rec.Code)
	}
	hs.svc.err = domain.ErrInvalidCredentials
	if rec := hs.do(http.MethodPost, "/api/v1/auth/login", `{"email":"a@b.mx","password":"x"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("login inválido = %d", rec.Code)
	}
}

func TestProtectedRoutes(t *testing.T) {
	hs := newHarness(t, 100)
	authz := hs.bearer(t, ana.ID, "user")

	rec := hs.do(http.MethodGet, "/api/v1/me", "", authz)
	if rec.Code != http.StatusOK || hs.svc.gotUserID != ana.ID || !strings.Contains(rec.Body.String(), `"email":"ana@example.com"`) {
		t.Errorf("GET /me = %d %s", rec.Code, rec.Body.String())
	}

	rec = hs.do(http.MethodPatch, "/api/v1/me", `{"displayName":"Ana L","phone":null}`, authz)
	if rec.Code != http.StatusOK || hs.svc.gotChanges.DisplayName == nil || *hs.svc.gotChanges.DisplayName != "Ana L" || !hs.svc.gotChanges.ClearPhone {
		t.Errorf("PATCH /me (null) = %d, cambios %+v", rec.Code, hs.svc.gotChanges)
	}
	rec = hs.do(http.MethodPatch, "/api/v1/me", `{"phone":"5512345678"}`, authz)
	if rec.Code != http.StatusOK || hs.svc.gotChanges.Phone == nil || *hs.svc.gotChanges.Phone != "5512345678" ||
		hs.svc.gotChanges.ClearPhone || hs.svc.gotChanges.DisplayName != nil {
		t.Errorf("PATCH /me (teléfono) = %d, cambios %+v", rec.Code, hs.svc.gotChanges)
	}
	rec = hs.do(http.MethodPatch, "/api/v1/me", `{"phone":5512345678}`, authz)
	if code, fields := errOf(t, rec); rec.Code != http.StatusBadRequest || code != "validation_failed" || len(fields) != 1 || fields[0] != "phone" {
		t.Errorf("PATCH /me teléfono numérico = %d %v", rec.Code, fields)
	}
	if rec := hs.do(http.MethodPatch, "/api/v1/me", `[`, authz); rec.Code != http.StatusBadRequest {
		t.Errorf("PATCH /me JSON roto = %d", rec.Code)
	}

	rec = hs.do(http.MethodDelete, "/api/v1/me", `{"password":"una-contrasena-larga"}`, authz)
	if rec.Code != http.StatusNoContent || hs.svc.gotPwd != "una-contrasena-larga" {
		t.Errorf("DELETE /me = %d", rec.Code)
	}
	if rec := hs.do(http.MethodDelete, "/api/v1/me", `{}`, authz); rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE /me sin contraseña = %d", rec.Code)
	}

	rec = hs.do(http.MethodGet, "/api/v1/users/"+ana.ID, "", authz)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "email") || strings.Contains(rec.Body.String(), "phone") ||
		!strings.Contains(rec.Body.String(), `"memberSince":"2026-11-06T18:00:00Z"`) {
		t.Errorf("GET /users/{id} = %d %s", rec.Code, rec.Body.String())
	}

	// Errores de dominio en rutas protegidas.
	cases := []struct {
		err          error
		method, path string
		body         string
		status       int
	}{
		{domain.ErrInvalidSession, http.MethodGet, "/api/v1/me", "", http.StatusUnauthorized},
		{domain.ErrInvalidSession, http.MethodPatch, "/api/v1/me", `{"displayName":"Ana"}`, http.StatusUnauthorized},
		{domain.ErrWrongPassword, http.MethodDelete, "/api/v1/me", `{"password":"x"}`, http.StatusForbidden},
		{domain.ErrActiveBookings, http.MethodDelete, "/api/v1/me", `{"password":"x"}`, http.StatusConflict},
		{domain.ErrUserNotFound, http.MethodGet, "/api/v1/users/nadie", "", http.StatusNotFound},
	}
	for _, c := range cases {
		hs.svc.err = c.err
		if rec := hs.do(c.method, c.path, c.body, authz); rec.Code != c.status {
			t.Errorf("%s %s con %v = %d, want %d", c.method, c.path, c.err, rec.Code, c.status)
		}
	}
}

func TestAuthenticate(t *testing.T) { // CA-04
	hs := newHarness(t, 100)
	expired := func() string {
		j, _ := security.NewJWT(testSecret, time.Nanosecond)
		tok, _ := j.Issue(ana.ID, "user")
		time.Sleep(1100 * time.Millisecond)
		return "Bearer " + tok
	}()
	other, _ := security.NewJWT(testSecret+"-otra", time.Minute)
	otherTok, _ := other.Issue(ana.ID, "user")

	for name, authz := range map[string]string{
		"sin encabezado": "",
		"sin esquema":    "abc",
		"otro esquema":   "Basic abc",
		"Bearer vacío":   "Bearer  ",
		"basura":         "Bearer a.b.c",
		"vencido":        expired,
		"otro secreto":   "Bearer " + otherTok,
	} {
		rec := hs.do(http.MethodGet, "/api/v1/me", "", authz)
		if code, _ := errOf(t, rec); rec.Code != http.StatusUnauthorized || code != "unauthorized" {
			t.Errorf("%s = %d %s", name, rec.Code, code)
		}
	}
	// El esquema no distingue mayúsculas.
	tok, _ := hs.jwt.Issue(ana.ID, "user")
	if rec := hs.do(http.MethodGet, "/api/v1/me", "", "bearer "+tok); rec.Code != http.StatusOK {
		t.Errorf("bearer en minúsculas = %d", rec.Code)
	}
}

func TestAuthRateLimit(t *testing.T) { // RF-07, CA-06
	hs := newHarness(t, 10)
	for i := 1; i <= 10; i++ {
		if rec := hs.do(http.MethodPost, "/api/v1/auth/login", `{"email":"a@b.mx","password":"x"}`, ""); rec.Code != http.StatusOK {
			t.Fatalf("intento %d = %d", i, rec.Code)
		}
	}
	rec := hs.do(http.MethodPost, "/api/v1/auth/register", validRegisterBody, "")
	if code, _ := errOf(t, rec); rec.Code != http.StatusTooManyRequests || code != "rate_limited" {
		t.Fatalf("11.º intento = %d %s", rec.Code, code)
	}
	// El límite es solo para register, login y refresh.
	if rec := hs.do(http.MethodPost, "/api/v1/auth/logout", `{"refreshToken":"x"}`, ""); rec.Code != http.StatusNoContent {
		t.Errorf("logout con el límite agotado = %d", rec.Code)
	}
	if rec := hs.do(http.MethodGet, "/api/v1/me", "", hs.bearer(t, ana.ID, "user")); rec.Code != http.StatusOK {
		t.Errorf("/me con el límite agotado = %d", rec.Code)
	}
}

func TestRequireRoleAndActive(t *testing.T) { // CA-07, RF-13
	hs := newHarness(t, 100)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	users := map[string]domain.User{
		"activa":     {ID: "activa", Status: domain.StatusActive},
		"suspendida": {ID: "suspendida", Status: domain.StatusActive, SuspendedUntil: &later},
	}
	lookup := func(_ context.Context, id string) (domain.User, error) {
		u, ok := users[id]
		if !ok {
			return domain.User{}, domain.ErrInvalidSession
		}
		return u, nil
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.With(Authenticate(hs.jwt), RequireRole(domain.RoleAdmin)).Get("/admin", ok)
	r.With(Authenticate(hs.jwt), RequireActive(lookup, func() time.Time { return now })).Post("/items", ok)
	r.With(RequireRole(domain.RoleAdmin)).Get("/sin-auth-admin", ok)
	r.With(RequireActive(lookup, nil)).Post("/sin-auth-items", ok)

	run := func(method, path, authz string) int {
		req := httptest.NewRequest(method, path, nil)
		if authz != "" {
			req.Header.Set("Authorization", authz)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	tests := []struct {
		name, method, path, authz string
		want                      int
	}{
		{"admin con rol user", http.MethodGet, "/admin", hs.bearer(t, "activa", "user"), http.StatusForbidden},
		{"admin con rol admin", http.MethodGet, "/admin", hs.bearer(t, "activa", "admin"), http.StatusTeapot},
		{"admin sin token", http.MethodGet, "/admin", "", http.StatusUnauthorized},
		{"role sin identidad", http.MethodGet, "/sin-auth-admin", "", http.StatusUnauthorized},
		{"crear activa", http.MethodPost, "/items", hs.bearer(t, "activa", "user"), http.StatusTeapot},
		{"crear suspendida", http.MethodPost, "/items", hs.bearer(t, "suspendida", "user"), http.StatusForbidden},
		{"crear cuenta inexistente", http.MethodPost, "/items", hs.bearer(t, "nadie", "user"), http.StatusUnauthorized},
		{"active sin identidad", http.MethodPost, "/sin-auth-items", "", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		if got := run(tt.method, tt.path, tt.authz); got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, got, tt.want)
		}
	}
	// La identidad llega a los handlers por auth.UserID (P-07).
	var seen string
	r2 := chi.NewRouter()
	r2.With(Authenticate(hs.jwt)).Get("/who", func(w http.ResponseWriter, req *http.Request) {
		seen, _ = auth.UserID(req.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/who", nil)
	req.Header.Set("Authorization", hs.bearer(t, "u-77", "user"))
	r2.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "u-77" {
		t.Errorf("auth.UserID = %q", seen)
	}
}

func TestNewModuleRequiresDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("NewModule sin dependencias debe fallar")
	}
	if jsonName("-") != "" || jsonName("email,omitempty") != "email" {
		t.Error("jsonName")
	}
	if issueFor("otra") != "valor inválido" {
		t.Error("issueFor")
	}
}
