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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

const (
	bookingID  = "6731a0000000000000000001"
	incidentID = "6731c0000000000000000001"
	reporter   = "6730f1c2a9b3e4d5f6a7b8c2"
	reported   = "6730f1c2a9b3e4d5f6a7b8c1"
	adminID    = "6730f1c2a9b3e4d5f6a7b8c4"
)

var at = time.Date(2026, 11, 21, 16, 0, 0, 0, time.UTC)

type fakeService struct {
	err      error
	view     usecase.View
	page     usecase.Page
	op, user string
	id       string
	viewer   usecase.Viewer
	in       usecase.OpenInput
	status   domain.Status
	after    *domain.Cursor
	limit    int
	text     string
	visible  bool
	outcome  domain.Outcome
}

func (f *fakeService) Open(_ context.Context, u, b string, in usecase.OpenInput) (usecase.View, error) {
	f.op, f.user, f.id, f.in = "open", u, b, in
	return f.view, f.err
}
func (f *fakeService) Get(_ context.Context, v usecase.Viewer, id string) (usecase.View, error) {
	f.op, f.viewer, f.id = "get", v, id
	return f.view, f.err
}
func (f *fakeService) ListMine(_ context.Context, u string, s domain.Status, a *domain.Cursor, l int) (usecase.Page, error) {
	f.op, f.user, f.status, f.after, f.limit = "mine", u, s, a, l
	return f.page, f.err
}
func (f *fakeService) AdminList(_ context.Context, s domain.Status, a *domain.Cursor, l int) (usecase.Page, error) {
	f.op, f.status, f.after, f.limit = "admin", s, a, l
	return f.page, f.err
}
func (f *fakeService) AddNote(_ context.Context, admin, id, text string, visible bool) (usecase.View, error) {
	f.op, f.user, f.id, f.text, f.visible = "note", admin, id, text, visible
	return f.view, f.err
}
func (f *fakeService) Close(_ context.Context, admin, id, text string, o domain.Outcome) (usecase.View, error) {
	f.op, f.user, f.id, f.text, f.outcome = "close", admin, id, text, o
	return f.view, f.err
}

// testAuth: X-User identifica y X-Role fija el rol.
func testAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-User")
		if id == "" {
			respond.Error(w, r, shared.NewUnauthorized("sin sesión"))
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), auth.Identity{UserID: id, Role: r.Header.Get("X-Role")})))
	})
}

// testAdmin simula auth.RequireRole(admin): 403 si el rol no es admin.
func testAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.Role(r.Context()) != "admin" {
			respond.Error(w, r, shared.NewForbidden("No tienes permiso para realizar esta acción."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type tenv struct {
	svc  *fakeService
	h    http.Handler
	logs *bytes.Buffer
}

func newTenv(t *testing.T) tenv {
	t.Helper()
	svc := &fakeService{}
	logs := &bytes.Buffer{}
	m, err := NewModule(ModuleDeps{Service: svc, Authenticate: testAuth, RequireAdmin: testAdmin, Log: slog.New(slog.NewJSONHandler(logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "incidents" {
		t.Error("Name")
	}
	r := chi.NewRouter()
	m.Register(r)
	return tenv{svc: svc, h: r, logs: logs}
}

func (e tenv) do(t *testing.T, method, path, body, user, role string) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if user != "" {
		req.Header.Set("X-User", user)
	}
	req.Header.Set("X-Role", role)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func sampleView(admin bool) usecase.View {
	closed := at.Add(time.Hour)
	i := domain.Incident{
		ID: incidentID, BookingID: bookingID, ReporterID: reporter, ReportedID: reported,
		Reason: domain.ReasonDamaged, Description: "La tapa llegó rota.", Status: domain.StatusClosed,
		Evidence:  []images.Image{{PublicID: "ev/1", Width: 800, Height: 600, Format: "jpg", Bytes: 1234, Delivery: images.DeliveryAuthenticated}},
		CreatedAt: at, ClosedAt: &closed,
		Final: &domain.FinalResponse{Text: "Se acordó un ajuste.", Outcome: domain.OutcomeMonetaryAdjustment, ByAdminID: adminID, At: closed},
	}
	notes := []domain.Note{{Text: "Pública", At: at, VisibleToParties: true}}
	if admin {
		notes = append(notes, domain.Note{Text: "Interna", At: at, ByAdminID: adminID})
	}
	rating := 4.5
	return usecase.View{
		Incident: i, Notes: notes, AdminView: admin,
		Reporter: users.PublicUser{ID: reporter, DisplayName: "Ana", RatingAverage: &rating, RatingCount: 2, MemberSince: at},
		Reported: users.PublicUser{ID: reported, DisplayName: "Beto", MemberSince: at},
		URLs:     []images.URLs{{URL: "https://img/ev1?sig=1", ThumbURL: "https://img/t/ev1"}},
	}
}

func TestNewModuleDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias")
	}
}

func TestRoutesRequireSession(t *testing.T) {
	e := newTenv(t)
	for _, p := range [][2]string{
		{"POST", "/bookings/" + bookingID + "/incidents"}, {"GET", "/incidents/" + incidentID}, {"GET", "/me/incidents"},
		{"GET", "/admin/incidents"}, {"POST", "/admin/incidents/" + incidentID + "/notes"}, {"POST", "/admin/incidents/" + incidentID + "/close"},
	} {
		if code, _ := e.do(t, p[0], p[1], `{}`, "", ""); code != 401 {
			t.Errorf("%s %s = %d, quiero 401", p[0], p[1], code)
		}
	}
	if e.svc.op != "" {
		t.Error("sin sesión no se llama al caso de uso")
	}
}

func TestAdminRoutesRejectNonAdmin(t *testing.T) { // CA-05
	e := newTenv(t)
	for _, p := range [][2]string{
		{"GET", "/admin/incidents"},
		{"POST", "/admin/incidents/" + incidentID + "/notes"},
		{"POST", "/admin/incidents/" + incidentID + "/close"},
	} {
		code, out := e.do(t, p[0], p[1], `{"text":"nota válida","visibleToParties":true,"finalResponse":"Respuesta final.","outcome":"none"}`, reporter, "user")
		if code != 403 || errCode(out) != "forbidden" {
			t.Errorf("%s %s = %d %v, quiero 403 forbidden", p[0], p[1], code, out)
		}
	}
	if e.svc.op != "" {
		t.Error("un usuario común no llega al caso de uso")
	}
}

func TestOpenCreated(t *testing.T) { // CA-04
	e := newTenv(t)
	e.svc.view = sampleView(false)
	body := `{"reason":"damaged","description":"La tapa llegó rota.","evidence":[{"publicId":"ev/1"},{"publicId":"ev/2"}]}`
	code, out := e.do(t, "POST", "/bookings/"+bookingID+"/incidents", body, reporter, "user")
	if code != 201 {
		t.Fatalf("%d %v", code, out)
	}
	if e.svc.user != reporter || e.svc.id != bookingID || e.svc.in.Reason != domain.ReasonDamaged || e.svc.in.Description != "La tapa llegó rota." ||
		len(e.svc.in.Evidence) != 2 || e.svc.in.Evidence[1].PublicID != "ev/2" {
		t.Errorf("entrada = %+v", e.svc)
	}
	ev, _ := out["evidence"].([]any)
	first, _ := ev[0].(map[string]any)
	if out["id"] != incidentID || out["status"] != "closed" || first["url"] != "https://img/ev1?sig=1" || first["thumbUrl"] != "https://img/t/ev1" || first["delivery"] != "authenticated" {
		t.Errorf("respuesta = %v", out)
	}
}

func TestOpenRejectsBadBodiesAndMapsErrors(t *testing.T) {
	e := newTenv(t)
	for name, body := range map[string]string{"no JSON": `no`, "tipo": `{"reason":3}`, "dos": `{}{}`} {
		if code, out := e.do(t, "POST", "/bookings/"+bookingID+"/incidents", body, reporter, "user"); code != 400 || errCode(out) != "validation_failed" {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	for err, want := range map[error]int{
		bookings.ErrNotFound: 404, domain.ErrBookingStatus: 409, domain.ErrWindowClosed: 409, domain.ErrTooEarly: 409, domain.ErrTooManyOpen: 409,
		images.ErrUnavailable: 503,
	} {
		e := newTenv(t)
		e.svc.err = err
		if code, _ := e.do(t, "POST", "/bookings/"+bookingID+"/incidents", `{"reason":"other","description":"Descripción larga."}`, reporter, "user"); code != want {
			t.Errorf("%v → %d, quiero %d", err, code, want)
		}
	}
	e.svc.err = errors.New("mongo caído")
	code, out := e.do(t, "POST", "/bookings/"+bookingID+"/incidents", `{"reason":"other"}`, reporter, "user")
	if code != 500 || errCode(out) != "internal_error" || !strings.Contains(e.logs.String(), "incidents: error interno") {
		t.Errorf("500: %d %v", code, out)
	}
}

func TestGetPassesViewerRoleAndHidesInternalFlag(t *testing.T) { // CA-07
	e := newTenv(t)
	e.svc.view = sampleView(false)
	code, out := e.do(t, "GET", "/incidents/"+incidentID, "", reporter, "user")
	notes, _ := out["notes"].([]any)
	n0, _ := notes[0].(map[string]any)
	if code != 200 || e.svc.viewer.Admin || e.svc.viewer.ID != reporter || len(notes) != 1 {
		t.Fatalf("%d %v %+v", code, out, e.svc.viewer)
	}
	if _, has := n0["visibleToParties"]; has {
		t.Error("la vista de las partes no expone visibleToParties")
	}
	fr, _ := out["finalResponse"].(map[string]any)
	if fr["text"] != "Se acordó un ajuste." || fr["outcome"] != "monetary_adjustment" || out["closedAt"] != "2026-11-21T17:00:00Z" {
		t.Errorf("finalResponse = %v", out)
	}
	if _, has := fr["byAdminId"]; has {
		t.Error("no se expone qué admin cerró")
	}

	e.svc.view = sampleView(true)
	_, out = e.do(t, "GET", "/incidents/"+incidentID, "", adminID, "admin")
	notes, _ = out["notes"].([]any)
	n1, _ := notes[1].(map[string]any)
	if !e.svc.viewer.Admin || len(notes) != 2 || n1["visibleToParties"] != false {
		t.Errorf("vista admin: %v admin=%v", out, e.svc.viewer.Admin)
	}

	e.svc.err = domain.ErrNotFound
	if code, _ := e.do(t, "GET", "/incidents/"+incidentID, "", "6730f1c2a9b3e4d5f6a7b8c9", "user"); code != 404 {
		t.Errorf("ajeno = %d", code)
	}
}

func TestListsPaginateAndValidate(t *testing.T) {
	e := newTenv(t)
	next := &domain.Cursor{CreatedAt: at, ID: incidentID}
	e.svc.page = usecase.Page{Items: []usecase.View{sampleView(false)}, Next: next}

	code, out := e.do(t, "GET", "/me/incidents?status=open&limit=5", "", reporter, "user")
	items, _ := out["items"].([]any)
	cur, _ := out["nextCursor"].(string)
	if code != 200 || len(items) != 1 || e.svc.op != "mine" || e.svc.user != reporter || e.svc.status != domain.StatusOpen || e.svc.limit != 5 {
		t.Fatalf("%d %v %+v", code, out, e.svc)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(cur)
	if string(raw) != strconv.FormatInt(at.UnixMilli(), 10)+"."+incidentID {
		t.Errorf("cursor = %q", raw)
	}
	if code, _ = e.do(t, "GET", "/me/incidents?cursor="+cur, "", reporter, "user"); code != 200 || e.svc.after == nil ||
		e.svc.after.ID != incidentID || !e.svc.after.CreatedAt.Equal(at) {
		t.Errorf("segunda página: %d %+v", code, e.svc.after)
	}

	e.svc.view = sampleView(true)
	e.svc.page = usecase.Page{Items: []usecase.View{sampleView(true)}}
	code, out = e.do(t, "GET", "/admin/incidents", "", adminID, "admin")
	items, _ = out["items"].([]any)
	it, _ := items[0].(map[string]any)
	notes, _ := it["notes"].([]any)
	if code != 200 || e.svc.op != "admin" || e.svc.status != "" || out["nextCursor"] != nil || len(notes) != 2 {
		t.Errorf("admin: %d %v %+v", code, out, e.svc)
	}
	e.svc.page = usecase.Page{}
	if _, out = e.do(t, "GET", "/admin/incidents?status=closed", "", adminID, "admin"); out["items"] == nil || e.svc.status != domain.StatusClosed {
		t.Errorf("vacía: %v", out)
	}

	for _, q := range []string{"status=x", "limit=0", "limit=51", "limit=x", "cursor=!!!", "cursor=" + base64.RawURLEncoding.EncodeToString([]byte("1.zz")),
		"cursor=" + base64.RawURLEncoding.EncodeToString([]byte("abc")), "cursor=" + strings.Repeat("a", 130)} {
		for _, p := range []struct{ path, role string }{{"/me/incidents", "user"}, {"/admin/incidents", "admin"}} {
			if code, out := e.do(t, "GET", p.path+"?"+q, "", adminID, p.role); code != 400 || errCode(out) != "validation_failed" {
				t.Errorf("%s?%s → %d %v", p.path, q, code, out)
			}
		}
	}
	e.svc.err = errors.New("caído")
	if code, _ := e.do(t, "GET", "/me/incidents", "", reporter, "user"); code != 500 {
		t.Errorf("error interno = %d", code)
	}
	if code, _ := e.do(t, "GET", "/admin/incidents", "", adminID, "admin"); code != 500 {
		t.Errorf("error interno admin = %d", code)
	}
}

func TestAddNote(t *testing.T) {
	e := newTenv(t)
	e.svc.view = sampleView(true)
	path := "/admin/incidents/" + incidentID + "/notes"
	code, out := e.do(t, "POST", path, `{"text":"Pide otra foto.","visibleToParties":false}`, adminID, "admin")
	if code != 200 || e.svc.op != "note" || e.svc.user != adminID || e.svc.id != incidentID || e.svc.text != "Pide otra foto." || e.svc.visible || out["id"] != incidentID {
		t.Fatalf("%d %v %+v", code, out, e.svc)
	}
	if code, _ = e.do(t, "POST", path, `{"text":"Pública","visibleToParties":true}`, adminID, "admin"); code != 200 || !e.svc.visible {
		t.Errorf("visible: %d %v", code, e.svc.visible)
	}
	for name, body := range map[string]string{
		"sin text": `{"visibleToParties":true}`, "sin visibleToParties": `{"text":"nota válida"}`, "vacío": `{}`,
		"tipo": `{"text":5,"visibleToParties":true}`, "no JSON": `x`,
	} {
		if code, out := e.do(t, "POST", path, body, adminID, "admin"); code != 400 || errCode(out) != "validation_failed" {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	for err, want := range map[error]int{domain.ErrNotFound: 404, domain.ErrNotOpen: 409, domain.ErrTooManyNotes: 409, domain.ErrChanged: 409} {
		e.svc.err = err
		if code, _ := e.do(t, "POST", path, `{"text":"nota válida","visibleToParties":true}`, adminID, "admin"); code != want {
			t.Errorf("%v → %d, quiero %d", err, code, want)
		}
	}
}

func TestClose(t *testing.T) {
	e := newTenv(t)
	e.svc.view = sampleView(true)
	path := "/admin/incidents/" + incidentID + "/close"
	code, out := e.do(t, "POST", path, `{"finalResponse":"Se acordó un ajuste.","outcome":"monetary_adjustment"}`, adminID, "admin")
	if code != 200 || e.svc.op != "close" || e.svc.user != adminID || e.svc.text != "Se acordó un ajuste." || e.svc.outcome != domain.OutcomeMonetaryAdjustment || out["status"] != "closed" {
		t.Fatalf("%d %v %+v", code, out, e.svc)
	}
	for name, body := range map[string]string{
		"sin finalResponse": `{"outcome":"none"}`, "sin outcome": `{"finalResponse":"Respuesta final."}`, "tipo": `{"finalResponse":1,"outcome":"none"}`, "no JSON": `x`,
	} {
		if code, out := e.do(t, "POST", path, body, adminID, "admin"); code != 400 || errCode(out) != "validation_failed" {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	for err, want := range map[error]int{domain.ErrNotFound: 404, domain.ErrNotOpen: 409, domain.ErrChanged: 409} {
		e.svc.err = err
		if code, _ := e.do(t, "POST", path, `{"finalResponse":"Respuesta final.","outcome":"none"}`, adminID, "admin"); code != want {
			t.Errorf("%v → %d, quiero %d", err, code, want)
		}
	}
}
