package http

import (
	"bytes"
	"context"
	"encoding/base64"
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

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

type fakeService struct {
	err        error
	view       usecase.ItemView
	page       usecase.Page
	lastViewer usecase.Viewer
	lastID     string
	created    usecase.ItemInput
	updated    usecase.UpdateInput
	listed     usecase.ListQuery
	op         string
}

func (f *fakeService) record(v usecase.Viewer, id, op string) {
	f.lastViewer, f.lastID, f.op = v, id, op
}

func (f *fakeService) Create(_ context.Context, v usecase.Viewer, in usecase.ItemInput) (usecase.ItemView, error) {
	f.record(v, "", "create")
	f.created = in
	return f.view, f.err
}

func (f *fakeService) Get(_ context.Context, v usecase.Viewer, id string) (usecase.ItemView, error) {
	f.record(v, id, "get")
	return f.view, f.err
}

func (f *fakeService) Update(_ context.Context, v usecase.Viewer, id string, in usecase.UpdateInput) (usecase.ItemView, error) {
	f.record(v, id, "update")
	f.updated = in
	return f.view, f.err
}

func (f *fakeService) Pause(_ context.Context, v usecase.Viewer, id string) (usecase.ItemView, error) {
	f.record(v, id, "pause")
	return f.view, f.err
}

func (f *fakeService) Resume(_ context.Context, v usecase.Viewer, id string) (usecase.ItemView, error) {
	f.record(v, id, "resume")
	return f.view, f.err
}

func (f *fakeService) Delete(_ context.Context, v usecase.Viewer, id string) error {
	f.record(v, id, "delete")
	return f.err
}

func (f *fakeService) ListMine(_ context.Context, v usecase.Viewer, q usecase.ListQuery) (usecase.Page, error) {
	f.record(v, "", "list")
	f.listed = q
	return f.page, f.err
}

func (f *fakeService) SuggestPrice(declared int64) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return declared * 3 / 100, nil
}

func (f *fakeService) Categories() []domain.Category { return domain.Categories() }

const itemID = "6731a2b3c4d5e6f7a8b9c0d1"

// testAuth simula Authenticate: el encabezado X-User es la identidad y
// X-Role el rol; sin X-User responde 401.
func testAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-User")
		if id == "" {
			respond.Error(w, r, shared.NewUnauthorized("sin sesión"))
			return
		}
		ctx := auth.WithIdentity(r.Context(), auth.Identity{UserID: id, Role: r.Header.Get("X-Role")})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// testActive simula RequireActive: X-Suspended → 403.
func testActive(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Suspended") != "" {
			respond.Error(w, r, shared.NewForbidden("suspendida"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type tenv struct {
	svc    *fakeService
	router http.Handler
	logs   *bytes.Buffer
}

func newTenv(t *testing.T) tenv {
	t.Helper()
	svc := &fakeService{view: sampleView(true)}
	logs := &bytes.Buffer{}
	m, err := NewModule(ModuleDeps{Service: svc, Authenticate: testAuth, RequireActive: testActive, Log: slog.New(slog.NewJSONHandler(logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "items" {
		t.Errorf("Name = %s", m.Name())
	}
	r := chi.NewRouter()
	m.Register(r)
	return tenv{svc: svc, router: r, logs: logs}
}

func sampleView(showDeclared bool) usecase.ItemView {
	nb := "Del Carmen"
	at := time.Date(2026, 11, 8, 16, 0, 0, 0, time.UTC)
	return usecase.ItemView{
		Item: domain.Item{
			ID: itemID, OwnerID: "owner1", Title: "Taladro", Description: "x", Category: "hogar",
			Condition: domain.ConditionGood, Modality: domain.ModalityRent, DeclaredValueCents: 100_000,
			PricePerDayCents: 3_000, PriceSource: domain.PriceSuggested,
			Zone:     domain.Zone{Municipality: "Coyoacán", Neighborhood: &nb},
			Location: &domain.Location{Lat: 19.123456, Lng: -99.16},
			Photos:   []images.Image{{PublicID: "p1", Width: 1, Height: 2, Format: "jpg", Bytes: 3, Delivery: images.DeliveryPublic}},
			Status:   domain.StatusActive, CreatedAt: at, UpdatedAt: at, Version: 1,
		},
		Owner:             users.PublicUser{ID: "owner1", DisplayName: "Ana", MemberSince: at},
		PhotoURLs:         []images.URLs{{URL: "https://img/p1", ThumbURL: "https://thumb/p1"}},
		ShowDeclaredValue: showDeclared,
	}
}

func (e tenv) do(t *testing.T, method, path, body string, headers ...string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("X-User", "owner1")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 && strings.HasPrefix(strings.TrimSpace(rec.Body.String()), "{") {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("JSON inválido: %s", rec.Body.String())
		}
	}
	return rec, out
}

func errorFields(body map[string]any) []string {
	e, _ := body["error"].(map[string]any)
	ds, _ := e["details"].([]any)
	var out []string
	for _, d := range ds {
		out = append(out, d.(map[string]any)["field"].(string))
	}
	return out
}

func errorCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

const validCreate = `{
  "title": "Taladro", "description": "x", "category": "hogar", "condition": "good",
  "modality": "rent", "declaredValueCents": 100000, "pricePerDayCents": 3000,
  "priceSource": "suggested", "usageRules": "Limpio",
  "zone": {"municipality": "Coyoacán", "neighborhood": "Del Carmen"},
  "approxLocation": {"lat": 19.123456, "lng": -99.16},
  "photos": [{"publicId": "p1"}, {"publicId": "p2"}]
}`

func TestNewModuleRequiresDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("NewModule sin dependencias debe fallar")
	}
}

func TestCreate(t *testing.T) {
	e := newTenv(t)
	rec, body := e.do(t, http.MethodPost, "/items", validCreate)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	in := e.svc.created
	if in.Title != "Taladro" || in.DeclaredValueCents != 100_000 || in.Modality != domain.ModalityRent ||
		*in.UsageRules != "Limpio" || *in.Zone.Neighborhood != "Del Carmen" || in.Location.Lat != 19.123456 ||
		len(in.Photos) != 2 || in.Photos[1].PublicID != "p2" || e.svc.lastViewer.ID != "owner1" {
		t.Errorf("entrada = %+v", in)
	}
	// RNF-02: la respuesta muestra 2 decimales; el dueño ve el valor declarado.
	loc := body["approxLocation"].(map[string]any)
	if loc["lat"] != 19.12 || body["declaredValueCents"] != float64(100000) {
		t.Errorf("respuesta = %v", body)
	}
	photo := body["photos"].([]any)[0].(map[string]any)
	if photo["url"] != "https://img/p1" || photo["thumbUrl"] != "https://thumb/p1" || photo["delivery"] != "public" {
		t.Errorf("foto = %v", photo)
	}
	owner := body["owner"].(map[string]any)
	if owner["displayName"] != "Ana" || owner["ratingAverage"] != nil {
		t.Errorf("owner = %v", owner)
	}
	if _, ok := owner["email"]; ok {
		t.Error("el perfil público no lleva correo")
	}
}

func TestCreateValidation(t *testing.T) {
	e := newTenv(t)
	cases := []struct {
		name, body string
		fields     []string
	}{
		{"vacío", `{}`, []string{"title", "description", "category", "condition", "modality", "declaredValueCents", "pricePerDayCents", "priceSource", "zone", "photos"}},
		{"sin publicId", strings.Replace(validCreate, `{"publicId": "p2"}`, `{}`, 1), []string{"photos[1].publicId"}},
		{"ubicación incompleta", strings.Replace(validCreate, `"lng": -99.16`, `"x": 1`, 1), []string{"approxLocation.lng"}},
		{"municipio vacío", strings.Replace(validCreate, `"municipality": "Coyoacán", `, ``, 1), []string{"zone.municipality"}},
		{"6 fotos", strings.Replace(validCreate, `{"publicId": "p1"}`, `{"publicId":"a"},{"publicId":"b"},{"publicId":"c"},{"publicId":"d"},{"publicId":"e"}`, 1), []string{"photos"}},
		{"tipo", strings.Replace(validCreate, `100000`, `"mil"`, 1), []string{"declaredValueCents"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, body := e.do(t, http.MethodPost, "/items", c.body)
			if rec.Code != http.StatusBadRequest || errorCode(body) != "validation_failed" {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
			if got := errorFields(body); strings.Join(got, ",") != strings.Join(c.fields, ",") {
				t.Errorf("campos = %v, want %v", got, c.fields)
			}
		})
	}
	for _, raw := range []string{`no json`, `{} {}`, `[]`} {
		if rec, _ := e.do(t, http.MethodPost, "/items", raw); rec.Code != http.StatusBadRequest {
			t.Errorf("%q → %d", raw, rec.Code)
		}
	}
}

func TestCreateAuthAndSuspended(t *testing.T) {
	e := newTenv(t)
	req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(validCreate))
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin sesión = %d", rec.Code)
	}
	// RF-12: cuenta suspendida no crea; sí puede leer.
	if rec, _ := e.do(t, http.MethodPost, "/items", validCreate, "X-Suspended", "1"); rec.Code != http.StatusForbidden {
		t.Errorf("suspendida crea = %d", rec.Code)
	}
	if rec, _ := e.do(t, http.MethodGet, "/items/"+itemID, "", "X-Suspended", "1"); rec.Code != http.StatusOK {
		t.Errorf("suspendida lee = %d", rec.Code)
	}
}

func TestGetHidesDeclaredValue(t *testing.T) {
	e := newTenv(t)
	e.svc.view = sampleView(false)
	e.svc.view.Item.Location = nil
	rec, body := e.do(t, http.MethodGet, "/items/"+itemID, "", "X-User", "otro", "X-Role", "admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if v, ok := body["declaredValueCents"]; !ok || v != nil {
		t.Errorf("declaredValueCents = %v (debe ser null)", v)
	}
	if v, ok := body["approxLocation"]; !ok || v != nil {
		t.Errorf("approxLocation = %v", v)
	}
	if e.svc.lastID != itemID || !e.svc.lastViewer.Admin || e.svc.lastViewer.ID != "otro" {
		t.Errorf("viewer = %+v id = %s", e.svc.lastViewer, e.svc.lastID)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{domain.ErrNotFound, 404},
		{domain.ErrNotOwner, 403},
		{domain.ErrVersionConflict, 409},
		{domain.ErrHasActiveBookings, 409},
		{shared.NewUnavailable("x"), 503},
		{errors.New("mongo caído con datos"), 500},
	}
	for _, c := range cases {
		e := newTenv(t)
		e.svc.err = c.err
		for _, req := range [][2]string{
			{http.MethodGet, "/items/" + itemID}, {http.MethodPost, "/items/" + itemID + "/pause"},
			{http.MethodPost, "/items/" + itemID + "/resume"}, {http.MethodDelete, "/items/" + itemID},
			{http.MethodGet, "/me/items"}, {http.MethodGet, "/items/price-suggestion?declaredValueCents=100000"},
		} {
			if rec, _ := e.do(t, req[0], req[1], ""); rec.Code != c.status {
				t.Errorf("%s %s con %v = %d, want %d", req[0], req[1], c.err, rec.Code, c.status)
			}
		}
		for _, req := range [][2]string{{http.MethodPost, "/items"}, {http.MethodPatch, "/items/" + itemID}} {
			body := validCreate
			if req[0] == http.MethodPatch {
				body = `{"version": 1, "title": "Nuevo"}`
			}
			if rec, _ := e.do(t, req[0], req[1], body); rec.Code != c.status {
				t.Errorf("%s con %v = %d", req[0], c.err, rec.Code)
			}
		}
		internal := strings.Contains(e.logs.String(), "items: error interno")
		if internal != (c.status == 500) {
			t.Errorf("%v: registrado = %v", c.err, internal)
		}
	}
}

func TestStatusAndDelete(t *testing.T) {
	e := newTenv(t)
	for _, c := range []struct{ path, op string }{{"/pause", "pause"}, {"/resume", "resume"}} {
		if rec, _ := e.do(t, http.MethodPost, "/items/"+itemID+c.path, ""); rec.Code != 200 || e.svc.op != c.op {
			t.Errorf("%s = %d %s", c.path, rec.Code, e.svc.op)
		}
	}
	rec, _ := e.do(t, http.MethodDelete, "/items/"+itemID, "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 || e.svc.op != "delete" {
		t.Errorf("delete = %d %q", rec.Code, rec.Body)
	}
}

func TestUpdate(t *testing.T) {
	e := newTenv(t)
	rec, _ := e.do(t, http.MethodPatch, "/items/"+itemID,
		`{"version": 3, "title": "Nuevo", "usageRules": null, "approxLocation": {"lat": 1, "lng": 2},
		  "zone": {"municipality": "Tlalpan"}, "modality": "loan", "condition": "fair", "priceSource": "manual",
		  "category": "otros", "description": "d", "declaredValueCents": 5000, "pricePerDayCents": 0,
		  "photos": [{"publicId": "p3"}]}`)
	if rec.Code != 200 {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	in := e.svc.updated
	if in.Version != 3 || *in.Title != "Nuevo" || !in.SetUsageRules || in.UsageRules != nil || !in.SetLocation ||
		in.Location.Lng != 2 || in.Zone.Municipality != "Tlalpan" || *in.Modality != domain.ModalityLoan ||
		len(in.Photos) != 1 || *in.PricePerDayCents != 0 {
		t.Errorf("entrada = %+v", in)
	}

	_, _ = e.do(t, http.MethodPatch, "/items/"+itemID, `{"version": 1, "approxLocation": null, "usageRules": "r"}`)
	in = e.svc.updated
	if !in.SetLocation || in.Location != nil || *in.UsageRules != "r" || in.Photos != nil || in.Title != nil {
		t.Errorf("null/ausentes = %+v", in)
	}

	bad := map[string]string{
		`{"title": "x"}`:                                    "version",
		`{"version": 0, "title": "x"}`:                      "version",
		`{"version": 1}`:                                    "",
		`{"version": 1, "photos": []}`:                      "photos",
		`{"version": 1, "zone": {}}`:                        "zone.municipality",
		`{"version": 1, "approxLocation": {"lat": 1}}`:      "approxLocation",
		`{"version": 1, "title": null, "description": "x"}`: "title",
		`{"version": 1, "photos": null}`:                    "photos",
		`{"version": 1, "zone": null, "title": "Nuevo"}`:    "zone",
		`{"version": 1, "usageRules": 5}`:                   "",
		`{"version": 1, "photos": [{"publicId": ""}]}`:      "photos[0].publicId",
	}
	for body, field := range bad {
		rec, out := e.do(t, http.MethodPatch, "/items/"+itemID, body)
		if rec.Code != 400 {
			t.Errorf("%s = %d", body, rec.Code)
			continue
		}
		if f := errorFields(out); field != "" && (len(f) != 1 || f[0] != field) {
			t.Errorf("%s: campos = %v, want %s", body, f, field)
		}
	}
}

func TestListMine(t *testing.T) {
	e := newTenv(t)
	next := &domain.Cursor{CreatedAt: time.Date(2026, 11, 8, 16, 0, 0, 5_000_000, time.UTC), ID: itemID}
	e.svc.page = usecase.Page{Items: []usecase.ItemView{sampleView(true)}, Next: next}
	rec, body := e.do(t, http.MethodGet, "/me/items?status=paused&limit=10", "")
	if rec.Code != 200 || len(body["items"].([]any)) != 1 {
		t.Fatalf("status = %d %v", rec.Code, body)
	}
	if *e.svc.listed.Status != domain.StatusPaused || e.svc.listed.Limit != 10 || e.svc.listed.After != nil {
		t.Errorf("consulta = %+v", e.svc.listed)
	}
	cursor := body["nextCursor"].(string)
	_, _ = e.do(t, http.MethodGet, "/me/items?cursor="+cursor, "")
	if a := e.svc.listed.After; a == nil || a.ID != itemID || !a.CreatedAt.Equal(next.CreatedAt) {
		t.Errorf("cursor decodificado = %+v", a)
	}

	e.svc.page = usecase.Page{}
	_, body = e.do(t, http.MethodGet, "/me/items", "")
	if items, ok := body["items"].([]any); !ok || len(items) != 0 || body["nextCursor"] != nil {
		t.Errorf("página vacía = %v", body)
	}

	for _, q := range []string{"status=removed", "status=x", "limit=0", "limit=51", "limit=a", "cursor=***", "cursor=" + encodeCursorRaw("1.zz"), "cursor=" + encodeCursorRaw("x."+itemID), "cursor=" + encodeCursorRaw("nada"), "cursor=" + strings.Repeat("a", 129)} {
		if rec, _ := e.do(t, http.MethodGet, "/me/items?"+q, ""); rec.Code != 400 {
			t.Errorf("%s = %d", q, rec.Code)
		}
	}
}

func encodeCursorRaw(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func TestPriceSuggestionAndCategories(t *testing.T) {
	e := newTenv(t)
	rec, body := e.do(t, http.MethodGet, "/items/price-suggestion?declaredValueCents=100000", "")
	if rec.Code != 200 || body["suggestedPricePerDayCents"] != float64(3000) || e.svc.op == "get" {
		t.Errorf("sugerencia = %d %v (op %s)", rec.Code, body, e.svc.op)
	}
	for _, q := range []string{"", "?declaredValueCents=abc", "?declaredValueCents=1.5"} {
		if rec, out := e.do(t, http.MethodGet, "/items/price-suggestion"+q, ""); rec.Code != 400 || errorFields(out)[0] != "declaredValueCents" {
			t.Errorf("%q = %d", q, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	req.Header.Set("X-User", "u")
	e.router.ServeHTTP(rec, req)
	var cats []map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &cats); err != nil || len(cats) != 7 || cats[0]["key"] != "herramientas_manuales" || cats[0]["label"] == "" {
		t.Errorf("categorías = %s", rec.Body)
	}
}
