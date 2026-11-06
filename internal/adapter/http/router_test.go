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

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/handler"
)

const testOrigin = "https://app.vaiven.mx"

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func newTestRouter(pingErr error) (http.Handler, *bytes.Buffer) {
	var buf bytes.Buffer
	return NewRouter(Deps{
		Log:            slog.New(slog.NewJSONHandler(&buf, nil)),
		AllowedOrigins: []string{testOrigin},
		Pinger:         fakePinger{err: pingErr},
		Meta:           handler.MetaInfo{Name: "vaiven-backend", Version: "test", Environment: "test"},
	}), &buf
}

func do(h http.Handler, method, path string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errorBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error == nil {
		t.Fatalf("cuerpo no es un envoltorio de error: %v (%q)", err, rec.Body.String())
	}
	return env.Error
}

func TestRoutes(t *testing.T) {
	h, _ := newTestRouter(nil)
	tests := []struct {
		path string
		want string
	}{
		{"/healthz", `"status":"ok"`},
		{"/readyz", `"status":"ready"`},
		{"/api/v1/meta", `"name":"vaiven-backend"`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := do(h, http.MethodGet, tt.path, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Errorf("body = %q, want que contenga %q", rec.Body.String(), tt.want)
			}
			if rec.Header().Get("X-Request-Id") == "" {
				t.Error("falta X-Request-Id")
			}
		})
	}
}

func TestReadyzUnavailable(t *testing.T) {
	h, _ := newTestRouter(errors.New("sin mongo"))
	rec := do(h, http.MethodGet, "/readyz", map[string]string{"X-Request-Id": "abc"})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body := errorBody(t, rec); body["code"] != "service_unavailable" || body["requestId"] != "abc" {
		t.Errorf("error = %v", body)
	}
}

func TestNotFound(t *testing.T) {
	h, _ := newTestRouter(nil)
	rec := do(h, http.MethodGet, "/no-existe", map[string]string{"X-Request-Id": "abc"})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "abc" {
		t.Errorf("X-Request-Id = %q, want abc", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if body := errorBody(t, rec); body["code"] != "not_found" || body["requestId"] != "abc" {
		t.Errorf("error = %v", body)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h, _ := newTestRouter(nil)
	rec := do(h, http.MethodPost, "/healthz", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	body := errorBody(t, rec)
	if body["code"] != "not_found" {
		t.Errorf("code = %v, want not_found", body["code"])
	}
	if id, _ := body["requestId"].(string); id == "" || id != rec.Header().Get("X-Request-Id") {
		t.Errorf("requestId = %v, encabezado = %q", body["requestId"], rec.Header().Get("X-Request-Id"))
	}
}

func TestMiddlewareIsMounted(t *testing.T) {
	h, buf := newTestRouter(nil)

	// Logging: la petición 404 también se registra, con el mismo requestId.
	rec := do(h, http.MethodGet, "/no-existe", map[string]string{"X-Request-Id": "req-9"})
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("el log no es JSON: %v (%q)", err, buf.String())
	}
	if entry["requestId"] != "req-9" || entry["status"] != float64(http.StatusNotFound) {
		t.Errorf("log = %v", entry)
	}
	if rec.Header().Get("Vary") == "" {
		t.Error("CORS no está montado (falta Vary)")
	}

	// CORS: origen listado recibe el encabezado; uno no listado, no.
	ok := do(h, http.MethodGet, "/healthz", map[string]string{"Origin": testOrigin})
	if got := ok.Header().Get("Access-Control-Allow-Origin"); got != testOrigin {
		t.Errorf("Allow-Origin = %q, want %q", got, testOrigin)
	}
	bad := do(h, http.MethodGet, "/healthz", map[string]string{"Origin": "https://evil.example"})
	if bad.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("no debe reflejar un origen no listado")
	}
}

func TestLimitBody(t *testing.T) {
	var readErr error
	h := limitBody(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, readErr = io.Copy(io.Discard, r.Body)
	}))

	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{"dentro del límite", maxBodyBytes, false},
		{"excede el límite", maxBodyBytes + 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readErr = nil
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(make([]byte, tt.size)))
			h.ServeHTTP(httptest.NewRecorder(), req)

			var tooLarge *http.MaxBytesError
			if got := errors.As(readErr, &tooLarge); got != tt.wantErr {
				t.Errorf("MaxBytesError = %v (err=%v), want %v", got, readErr, tt.wantErr)
			}
		})
	}
}
