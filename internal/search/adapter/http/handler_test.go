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

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

type fakeService struct {
	res    usecase.Result
	err    error
	viewer string
	raw    domain.RawQuery
}

func (f *fakeService) Search(_ context.Context, viewer string, raw domain.RawQuery) (usecase.Result, error) {
	f.viewer, f.raw = viewer, raw
	return f.res, f.err
}

func testAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-User")
		if id == "" {
			respond.Error(w, r, shared.NewUnauthorized("sin sesión"))
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), auth.Identity{UserID: id})))
	})
}

func setup(t *testing.T, svc *fakeService) (http.Handler, *bytes.Buffer) {
	t.Helper()
	logs := &bytes.Buffer{}
	m, err := NewModule(ModuleDeps{Service: svc, Authenticate: testAuth, Log: slog.New(slog.NewJSONHandler(logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "search" {
		t.Errorf("Name = %s", m.Name())
	}
	r := chi.NewRouter()
	// Como en cmd/api: la 005 registra POST /items en otro grupo.
	r.Group(func(r chi.Router) {
		r.Use(testAuth)
		r.Post("/items", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })
	})
	m.Register(r)
	return r, logs
}

func get(t *testing.T, h http.Handler, url, user string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	if user != "" {
		req.Header.Set("X-User", user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestNewModuleRequiresDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
}

func TestSearchOK(t *testing.T) {
	dist, avg, thumb, nb := 1449.0, 4.5, "https://thumb/p1", "Del Carmen"
	at := time.Date(2026, 11, 8, 16, 0, 0, 0, time.UTC)
	svc := &fakeService{res: usecase.Result{
		Cards: []usecase.CardView{{
			Card: domain.Card{
				ID: "a", Title: "Taladro", Category: "hogar", Modality: "rent", PricePerDayCents: 3000,
				Zone:     domain.Zone{Municipality: "Coyoacán", Neighborhood: &nb},
				Location: &domain.Point{Lat: 19.35, Lng: -99.16}, DistanceMeters: &dist, PhotoCount: 2,
				Owner:     domain.Owner{ID: "o", DisplayName: "Ana", RatingAverage: &avg, RatingCount: 2},
				CreatedAt: at,
			},
			ThumbURL: &thumb,
		}, {Card: domain.Card{ID: "b"}}},
		Next: &domain.Cursor{Mode: domain.ModeNear, Key: 1449, ID: "6731a2b3c4d5e6f7a8b9c0d1"},
	}}
	h, _ := setup(t, svc)
	rec, body := get(t, h, "/items?q=taladro&category=hogar&modality=rent&municipality=Coyoac%C3%A1n&from=2026-11-10&to=2026-11-11&lat=19.3&lng=-99.1&radiusKm=5&limit=10&cursor=abc", "u1")
	if rec.Code != 200 {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	want := domain.RawQuery{Q: "taladro", Category: "hogar", Modality: "rent", Municipality: "Coyoacán", From: "2026-11-10",
		To: "2026-11-11", Lat: "19.3", Lng: "-99.1", RadiusKm: "5", Limit: "10", Cursor: "abc"}
	if svc.raw != want || svc.viewer != "u1" {
		t.Errorf("raw = %+v viewer = %s", svc.raw, svc.viewer)
	}
	items := body["items"].([]any)
	c := items[0].(map[string]any)
	if c["distanceKm"] != 1.4 || c["thumbUrl"] != thumb || c["photoCount"] != float64(2) ||
		c["approxLocation"].(map[string]any)["lat"] != 19.35 || c["owner"].(map[string]any)["ratingAverage"] != 4.5 {
		t.Errorf("tarjeta = %v", c)
	}
	for _, k := range []string{"declaredValueCents", "usageRules", "description", "photos"} {
		if _, ok := c[k]; ok {
			t.Errorf("la tarjeta no debe llevar %s", k)
		}
	}
	empty := items[1].(map[string]any)
	if empty["distanceKm"] != nil || empty["thumbUrl"] != nil || empty["approxLocation"] != nil {
		t.Errorf("tarjeta sin datos = %v", empty)
	}
	cur, _ := domain.DecodeCursor(body["nextCursor"].(string))
	if cur.Key != 1449 || cur.Mode != domain.ModeNear {
		t.Errorf("cursor = %+v", cur)
	}
}

func TestSearchErrorsAndRoutes(t *testing.T) {
	svc := &fakeService{}
	h, logs := setup(t, svc)
	if rec, _ := get(t, h, "/items", ""); rec.Code != 401 {
		t.Errorf("sin sesión = %d", rec.Code)
	}
	_, body := get(t, h, "/items", "u")
	if items, ok := body["items"].([]any); !ok || len(items) != 0 || body["nextCursor"] != nil {
		t.Errorf("vacío = %v", body)
	}
	svc.err = shared.NewValidation("x", shared.Detail{Field: "lng", Issue: "lat y lng van juntos"})
	rec, body := get(t, h, "/items?lat=1", "u")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"field":"lng"`) {
		t.Errorf("CA-06 = %d %v", rec.Code, body)
	}
	svc.err = errors.New("mongo")
	if rec, _ := get(t, h, "/items", "u"); rec.Code != 500 || !strings.Contains(logs.String(), "search: error interno") {
		t.Errorf("interno = %d", rec.Code)
	}
	// POST /items de la 005 sigue funcionando en la misma ruta.
	req := httptest.NewRequest(http.MethodPost, "/items", nil)
	req.Header.Set("X-User", "u")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("POST /items = %d", rec.Code)
	}
}
