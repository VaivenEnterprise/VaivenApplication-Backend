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

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/usecase"
	searchdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

const itemID = "6730f1c2a9b3e4d5f6a7b8c9"

type fakeSvc struct {
	enabled             bool
	recs                []usecase.Recommendation
	err                 error
	user, item, typ     string
	limit, municipality string
	deleted             string
}

func (f *fakeSvc) Enabled() bool { return f.enabled }

func (f *fakeSvc) RecordView(_ context.Context, userID, itemID, rawType string) error {
	f.user, f.item, f.typ = userID, itemID, rawType
	return f.err
}

func (f *fakeSvc) Recommend(_ context.Context, userID, rawLimit, rawMunicipality string) ([]usecase.Recommendation, error) {
	f.user, f.limit, f.municipality = userID, rawLimit, rawMunicipality
	return f.recs, f.err
}

func (f *fakeSvc) DeleteHistory(_ context.Context, userID string) error {
	f.deleted = userID
	return f.err
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
	m, err := NewModule(ModuleDeps{Service: svc, Authenticate: fakeAuth, Log: slog.New(slog.NewJSONHandler(&logBuf, nil)), Limiter: redis.NewMemoryLimiter()})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "recommendations" {
		t.Error("Name")
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Route("/api/v1", m.Register)
	return r, &logBuf
}

func do(h http.Handler, method, path, body, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details []struct {
			Field string `json:"field"`
		} `json:"details"`
		RequestID string `json:"requestId"`
	} `json:"error"`
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var e errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error.Code == "" || e.Error.RequestID == "" {
		t.Fatalf("no es el error del Art. IV: %s", rec.Body.String())
	}
	return e
}

func TestNewModuleNeedsDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
}

func TestEvent(t *testing.T) {
	svc := &fakeSvc{enabled: true}
	h, _ := newRouter(t, svc)
	body := `{"itemId":"` + itemID + `","type":"view"}`
	if rec := do(h, http.MethodPost, "/api/v1/recommendations/events", body, "u1"); rec.Code != http.StatusNoContent ||
		svc.user != "u1" || svc.item != itemID || svc.typ != "view" {
		t.Fatalf("válido: %d %+v", rec.Code, svc)
	}
	if rec := do(h, http.MethodPost, "/api/v1/recommendations/events", body, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("sin sesión: %d", rec.Code)
	}
	for body, field := range map[string]string{
		`{"itemId":"x","type":"view"}`:                               "itemId",
		`{"itemId":"` + strings.ToUpper(itemID) + `","type":"view"}`: "itemId",
		`{"itemId":"` + itemID + `","type":"click"}`:                 "type",
		`{"itemId":"` + itemID + `"}`:                                "type",
	} {
		rec := do(h, http.MethodPost, "/api/v1/recommendations/events", body, "u1")
		if e := decodeError(t, rec); rec.Code != http.StatusBadRequest || e.Error.Code != "validation_failed" || e.Error.Details[0].Field != field {
			t.Errorf("%s: %d %+v", body, rec.Code, e)
		}
	}
	for _, body := range []string{`{`, `{"itemId":"` + itemID + `","type":"view","extra":1}`, `{} {}`} {
		if rec := do(h, http.MethodPost, "/api/v1/recommendations/events", body, "u1"); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
	svc.err = domain.ErrItemNotFound
	if rec := do(h, http.MethodPost, "/api/v1/recommendations/events", body, "u1"); rec.Code != http.StatusNotFound {
		t.Errorf("inexistente: %d", rec.Code)
	}
}

// CA-09: la petición 61 en un minuto recibe 429 con Retry-After.
func TestEventRateLimit(t *testing.T) {
	h, _ := newRouter(t, &fakeSvc{enabled: true})
	body := `{"itemId":"` + itemID + `","type":"view"}`
	for i := range domain.EventsPerMinute {
		if rec := do(h, http.MethodPost, "/api/v1/recommendations/events", body, "u1"); rec.Code != http.StatusNoContent {
			t.Fatalf("petición %d: %d", i+1, rec.Code)
		}
	}
	rec := do(h, http.MethodPost, "/api/v1/recommendations/events", body, "u1")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || decodeError(t, rec).Error.Code != "rate_limited" {
		t.Errorf("61.ª: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, http.MethodPost, "/api/v1/recommendations/events", body, "otra"); rec.Code != http.StatusNoContent {
		t.Errorf("el límite es por persona: %d", rec.Code)
	}
}

func TestList(t *testing.T) {
	thumb := "https://res.cloudinary.com/t.jpg"
	svc := &fakeSvc{enabled: true, recs: []usecase.Recommendation{
		{Card: searchusecase.CardView{Card: searchdomain.Card{ID: itemID, Title: "Taladro", Category: "herramientas_electricas", Modality: "rent"}, ThumbURL: &thumb},
			Reason: domain.SimilarTo("6731a2b3c4d5e6f7a8b9c0d1", "Sierra")},
		{Card: searchusecase.CardView{Card: searchdomain.Card{ID: "6732b3c4d5e6f7a8b9c0d1e2", Title: "Escalera"}},
			Reason: domain.Reason{Code: domain.ReasonPopularNearby}},
	}}
	h, _ := newRouter(t, svc)
	rec := do(h, http.MethodGet, "/api/v1/recommendations?limit=5&municipality=Tula", "", "u1")
	if rec.Code != http.StatusOK || svc.limit != "5" || svc.municipality != "Tula" || svc.user != "u1" {
		t.Fatalf("lista: %d %+v", rec.Code, svc)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if v, ok := raw["nextCursor"]; !ok || v != nil {
		t.Errorf("nextCursor debe ser null: %v", raw)
	}
	items := raw["items"].([]any)
	first := items[0].(map[string]any)
	item, reason := first["item"].(map[string]any), first["reason"].(map[string]any)
	if item["id"] != itemID || item["thumbUrl"] != thumb || reason["code"] != "similar_to" || reason["itemTitle"] != "Sierra" {
		t.Errorf("primero = %v", first)
	}
	second := items[1].(map[string]any)["reason"].(map[string]any)
	if _, has := second["itemId"]; has || second["code"] != "popular_nearby" {
		t.Errorf("motivo sin objeto = %v", second)
	}

	svc.recs = nil
	rec = do(h, http.MethodGet, "/api/v1/recommendations", "", "u1")
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("lista vacía debe ser []: %s", rec.Body.String())
	}
	svc.err = shared.NewValidation("Revisa los parámetros.", shared.Detail{Field: "limit", Issue: "x"})
	if rec := do(h, http.MethodGet, "/api/v1/recommendations?limit=0", "", "u1"); rec.Code != http.StatusBadRequest {
		t.Errorf("limit inválido: %d", rec.Code)
	}
}

func TestDeleteHistory(t *testing.T) {
	svc := &fakeSvc{enabled: true}
	h, _ := newRouter(t, svc)
	if rec := do(h, http.MethodDelete, "/api/v1/recommendations/history", "", "u1"); rec.Code != http.StatusNoContent || svc.deleted != "u1" {
		t.Errorf("borrar: %d %s", rec.Code, svc.deleted)
	}
}

// CA-06: apagado, las tres rutas responden 503 antes de validar nada.
func TestDisabledCA06(t *testing.T) {
	h, _ := newRouter(t, &fakeSvc{})
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/recommendations/events", `{"itemId":"x"}`},
		{http.MethodGet, "/api/v1/recommendations?limit=0", ""},
		{http.MethodDelete, "/api/v1/recommendations/history", ""},
	} {
		rec := do(h, c.method, c.path, c.body, "u1")
		if rec.Code != http.StatusServiceUnavailable || decodeError(t, rec).Error.Code != "service_unavailable" {
			t.Errorf("%s %s: %d", c.method, c.path, rec.Code)
		}
	}
}

// CA-07: un error interno se registra sin el objeto ni el evento.
func TestInternalErrorLogsWithoutItem(t *testing.T) {
	svc := &fakeSvc{enabled: true, err: errors.New("mongo caído")}
	h, logs := newRouter(t, svc)
	rec := do(h, http.MethodPost, "/api/v1/recommendations/events", `{"itemId":"`+itemID+`","type":"view"}`, "u1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("interno: %d", rec.Code)
	}
	if !strings.Contains(logs.String(), "mongo caído") || strings.Contains(logs.String(), itemID) {
		t.Errorf("registro = %s", logs.String())
	}
	if strings.Contains(rec.Body.String(), "mongo") {
		t.Errorf("la respuesta filtra la causa: %s", rec.Body.String())
	}
}
