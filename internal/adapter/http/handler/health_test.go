package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
)

// fakePinger es un Pinger que devuelve siempre err.
type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	Healthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Errorf("body = %q", got)
	}
}

func TestReadyzReady(t *testing.T) {
	rec := httptest.NewRecorder()
	Readyz(fakePinger{})(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ready"}` {
		t.Errorf("body = %q", got)
	}
}

func TestReadyzUnavailable(t *testing.T) {
	cause := errors.New("mongodb: ping: server selection timeout mongodb://user:clave@host")
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req = req.WithContext(respond.WithRequestID(req.Context(), "req-3"))
	rec := httptest.NewRecorder()
	Readyz(fakePinger{err: cause})(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var env struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("cuerpo no es JSON: %v", err)
	}
	if env.Error["code"] != "service_unavailable" || env.Error["requestId"] != "req-3" {
		t.Errorf("error = %v", env.Error)
	}
	if msg, _ := env.Error["message"].(string); msg == "" {
		t.Error("message vacío")
	}
	if strings.Contains(rec.Body.String(), "clave") || strings.Contains(rec.Body.String(), "timeout") {
		t.Errorf("la respuesta filtra la causa: %s", rec.Body.String())
	}
}

func TestMeta(t *testing.T) {
	// Hora con zona distinta de UTC: debe salir convertida a UTC.
	local := time.FixedZone("CST", -6*60*60)
	fixed := time.Date(2026, 11, 6, 11, 30, 15, 0, local)
	info := MetaInfo{Name: "vaiven-backend", Version: "1.2.3", Environment: "test"}

	rec := httptest.NewRecorder()
	meta(info, func() time.Time { return fixed })(rec, httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("cuerpo no es JSON: %v", err)
	}
	want := map[string]string{
		"name":        "vaiven-backend",
		"version":     "1.2.3",
		"environment": "test",
		"serverTime":  "2026-11-06T17:30:15Z",
	}
	if len(got) != len(want) {
		t.Errorf("campos = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestMetaUsesCurrentUTCTime(t *testing.T) {
	rec := httptest.NewRecorder()
	before := time.Now().UTC().Truncate(time.Second)
	Meta(MetaInfo{Name: "vaiven-backend"})(rec, httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil))
	after := time.Now().UTC()

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	ts, err := time.Parse(time.RFC3339, got["serverTime"])
	if err != nil {
		t.Fatalf("serverTime no es RFC 3339: %v", err)
	}
	if !strings.HasSuffix(got["serverTime"], "Z") || ts.Before(before) || ts.After(after) {
		t.Errorf("serverTime = %q fuera de [%v, %v] o sin Z", got["serverTime"], before, after)
	}
}
