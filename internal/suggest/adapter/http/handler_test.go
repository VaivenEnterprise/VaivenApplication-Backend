package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/suggest/usecase"
)

const secretPhoto = "vaiven/test/item_photo/u1/foto-secreta"

type fakeSvc struct {
	out     aidomain.Suggestion
	err     error
	gotUser string
	gotID   string
}

func (f *fakeSvc) Suggest(_ context.Context, userID, publicID string) (aidomain.Suggestion, error) {
	f.gotUser, f.gotID = userID, publicID
	return f.out, f.err
}

func fakeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || id == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), auth.Identity{UserID: id, Role: "user"})))
	})
}

func newRouter(t *testing.T, svc *fakeSvc) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logBuf bytes.Buffer
	m, err := NewModule(ModuleDeps{
		Service: svc, Authenticate: fakeAuth, Log: slog.New(slog.NewJSONHandler(&logBuf, nil)),
		Limiter: redis.NewMemoryLimiter(),
		Now:     func() time.Time { return time.Date(2026, 11, 16, 23, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "suggest" {
		t.Error("Name")
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Route("/api/v1", m.Register)
	return r, &logBuf
}

func post(h http.Handler, body, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/listing-suggestions", strings.NewReader(body))
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"requestId"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error.RequestID == "" {
		t.Fatalf("no es el envoltorio del Art. IV: %s", rec.Body)
	}
	return body.Error.Code
}

func TestSuggestOK(t *testing.T) {
	svc := &fakeSvc{out: aidomain.Suggestion{Title: "Taladro", Category: "herramientas_electricas"}}
	h, logs := newRouter(t, svc)
	rec := post(h, `{"imagePublicId":"`+secretPhoto+`"}`, "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("estado %d: %s", rec.Code, rec.Body)
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["title"] != "Taladro" || got["category"] != "herramientas_electricas" {
		t.Errorf("respuesta = %v", got)
	}
	if _, ok := got["description"]; ok {
		t.Error("description vacía debe omitirse")
	}
	if svc.gotUser != "u1" || svc.gotID != secretPhoto {
		t.Errorf("el caso de uso recibió %q %q", svc.gotUser, svc.gotID)
	}
	if strings.Contains(logs.String(), "foto-secreta") || strings.Contains(logs.String(), "Taladro") {
		t.Errorf("los registros no deben tener la foto ni la sugerencia: %s", logs)
	}
}

func TestSuggestNeedsSession(t *testing.T) {
	h, _ := newRouter(t, &fakeSvc{})
	if rec := post(h, `{"imagePublicId":"x"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("sin sesión: %d", rec.Code)
	}
}

func TestSuggestInvalidBody(t *testing.T) {
	h, _ := newRouter(t, &fakeSvc{})
	for _, body := range []string{`no json`, `{}`, `{"imagePublicId":""}`, `{"imagePublicId":"x","url":"https://evil"}`,
		`{"imagePublicId":"` + strings.Repeat("a", 256) + `"}`, `{"imagePublicId":"x"}{}`} {
		rec := post(h, body, "u1")
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != "validation_failed" {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
}

func TestSuggestErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{usecase.ErrNoImage, http.StatusNotFound, "not_found"},
		{usecase.ErrDisabled, http.StatusServiceUnavailable, "service_unavailable"},
		{usecase.ErrNoProposal.Wrap(aidomain.ErrUnavailable), http.StatusServiceUnavailable, "service_unavailable"},
		{usecase.ErrDailyLimit, http.StatusTooManyRequests, "rate_limited"},
		{shared.NewForbidden("no"), http.StatusForbidden, "forbidden"},
		{errors.New("mongo caído"), http.StatusInternalServerError, "internal_error"},
	}
	for _, c := range cases {
		h, logs := newRouter(t, &fakeSvc{err: c.err})
		rec := post(h, `{"imagePublicId":"`+secretPhoto+`"}`, "u1")
		if rec.Code != c.status || errCode(t, rec) != c.code {
			t.Errorf("%v: %d %s", c.err, rec.Code, rec.Body)
		}
		if strings.Contains(logs.String(), "foto-secreta") {
			t.Errorf("%v: el publicId llegó a los registros", c.err)
		}
	}
}

func TestDailyLimitRetryAfterUntilMidnight(t *testing.T) {
	h, _ := newRouter(t, &fakeSvc{err: usecase.ErrDailyLimit})
	rec := post(h, `{"imagePublicId":"x"}`, "u1")
	// 23:00 UTC con la zona por defecto (UTC): faltan 3600 s para medianoche.
	if rec.Header().Get("Retry-After") != "3600" {
		t.Errorf("Retry-After = %q", rec.Header().Get("Retry-After"))
	}
}

func TestPerMinuteLimit(t *testing.T) {
	h, _ := newRouter(t, &fakeSvc{out: aidomain.Suggestion{Title: "x"}})
	var last int
	for i := 0; i < perMinute+1; i++ {
		last = post(h, `{"imagePublicId":"x"}`, "u1").Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("la petición %d debe ser 429, fue %d", perMinute+1, last)
	}
	if rec := post(h, `{"imagePublicId":"x"}`, "u2"); rec.Code != http.StatusOK {
		t.Errorf("otra persona tiene su propio límite: %d", rec.Code)
	}
}

func TestNewModuleRequiresDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
}
