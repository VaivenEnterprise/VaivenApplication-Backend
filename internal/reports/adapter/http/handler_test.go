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
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

const (
	me       = "6730f1c2a9b3e4d5f6a7b8c1"
	adminID  = "6730f1c2a9b3e4d5f6a7b8ff"
	itemID   = "6731a0000000000000000002"
	reportID = "6731e0000000000000000001"
)

var at = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

type fakeService struct {
	err     error
	created bool
	rep     domain.Report
	page    usecase.Page
	view    usecase.View

	op, user, status, after, action, note, target string
	limit                                         int
	in                                            usecase.Input
}

func (f *fakeService) Create(_ context.Context, u string, in usecase.Input) (domain.Report, bool, error) {
	f.op, f.user, f.in = "create", u, in
	return f.rep, f.created, f.err
}
func (f *fakeService) List(_ context.Context, status, after string, limit int) (usecase.Page, error) {
	f.op, f.status, f.after, f.limit = "list", status, after, limit
	return f.page, f.err
}
func (f *fakeService) Resolve(_ context.Context, a, id, action, note string) (usecase.View, error) {
	f.op, f.user, f.target, f.action, f.note = "resolve", a, id, action, note
	return f.view, f.err
}

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
	if m.Name() != "reports" {
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
	if role != "" {
		req.Header.Set("X-Role", role)
	}
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

func sampleView() usecase.View {
	d := "Detalle"
	avg := 4.5
	return usecase.View{
		Report: domain.Report{
			ID: reportID, ReporterID: me, TargetType: domain.TargetItem, TargetID: itemID, Reason: domain.ReasonProhibited,
			Details: &d, Status: domain.StatusActioned, CreatedAt: at,
			Resolution: &domain.Resolution{Action: domain.ActionHideItem, Note: "Incumple", ByAdminID: adminID, At: at.Add(time.Hour)},
		},
		Reporter:      users.PublicUser{ID: me, DisplayName: "Ana", RatingAverage: &avg, RatingCount: 3, MemberSince: at},
		TargetSummary: "Taladro",
	}
}

func TestNewModuleDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias")
	}
}

func TestRoutesRequireSessionAndAdmin(t *testing.T) {
	e := newTenv(t)
	for _, p := range [][2]string{{"POST", "/reports"}, {"GET", "/admin/reports"}, {"POST", "/admin/reports/" + reportID + "/resolve"}} {
		if code, _ := e.do(t, p[0], p[1], `{}`, "", ""); code != 401 {
			t.Errorf("%s %s sin sesión = %d, quiero 401", p[0], p[1], code)
		}
	}
	for _, p := range [][2]string{{"GET", "/admin/reports"}, {"POST", "/admin/reports/" + reportID + "/resolve"}} {
		code, body := e.do(t, p[0], p[1], `{"action":"dismiss","note":"nota"}`, me, "user")
		if code != 403 || errCode(body) != "forbidden" {
			t.Errorf("%s %s como usuario = %d %v, quiero 403", p[0], p[1], code, body)
		}
	}
	if e.svc.op != "" {
		t.Errorf("sin permiso no se debe llamar al servicio (%s)", e.svc.op)
	}
	// Reportar no exige rol admin.
	e.svc.created, e.svc.rep = true, domain.Report{ID: reportID}
	if code, _ := e.do(t, "POST", "/reports", `{"targetType":"item","targetId":"`+itemID+`","reason":"spam"}`, me, "user"); code != 201 {
		t.Errorf("reportar como usuario = %d", code)
	}
}

func TestCreateStatusCodes(t *testing.T) {
	e := newTenv(t)
	body := `{"targetType":"item","targetId":"` + itemID + `","reason":"prohibited_content","details":"Algo"}`
	e.svc.rep, e.svc.created = domain.Report{ID: reportID}, true
	code, out := e.do(t, "POST", "/reports", body, me, "user")
	if code != 201 || out["id"] != reportID || len(out) != 1 {
		t.Errorf("nuevo = %d %v", code, out)
	}
	if e.svc.user != me || e.svc.in.TargetType != "item" || e.svc.in.TargetID != itemID || e.svc.in.Reason != "prohibited_content" || *e.svc.in.Details != "Algo" {
		t.Errorf("entrada = %+v user=%s", e.svc.in, e.svc.user)
	}
	e.svc.created = false
	if code, out := e.do(t, "POST", "/reports", body, me, "user"); code != 200 || out["id"] != reportID {
		t.Errorf("existente = %d %v", code, out)
	}
}

func TestCreateBadBodies(t *testing.T) {
	e := newTenv(t)
	for _, b := range []string{
		`nada`, `{}`, `{"targetType":"item","targetId":"x"}`, `{"targetType":"item","reason":"spam"}`, `{"targetId":"x","reason":"spam"}`,
		`{"targetType":1,"targetId":"x","reason":"spam"}`, `{"targetType":"item","targetId":"x","reason":"spam","extra":1}`,
		`{"targetType":"item","targetId":"x","reason":"spam"} {}`,
	} {
		e.svc.op = ""
		code, body := e.do(t, "POST", "/reports", b, me, "user")
		if code != 400 || errCode(body) != "validation_failed" || e.svc.op != "" {
			t.Errorf("%q → %d %v", b, code, body)
		}
	}
}

func TestServiceErrorsMapToEnvelope(t *testing.T) {
	e := newTenv(t)
	good := `{"targetType":"message","targetId":"` + itemID + `","reason":"spam"}`
	cases := []struct {
		err  error
		code int
		name string
	}{
		{domain.ErrTargetNotFound, 404, "not_found"}, {domain.ErrOwnTarget, 409, "conflict"},
		{domain.ErrAlreadyResolved, 409, "conflict"}, {domain.ErrReportNotFound, 404, "not_found"},
	}
	for _, c := range cases {
		e.svc.err = c.err
		code, body := e.do(t, "POST", "/reports", good, me, "user")
		if code != c.code || errCode(body) != c.name {
			t.Errorf("%v → %d %v", c.err, code, body)
		}
	}
	e.svc.err = errors.New("mongo caído: secreto")
	for _, r := range [][4]string{
		{"POST", "/reports", good, "user"}, {"GET", "/admin/reports", "", "admin"},
		{"POST", "/admin/reports/" + reportID + "/resolve", `{"action":"dismiss","note":"nota"}`, "admin"},
	} {
		code, body := e.do(t, r[0], r[1], r[2], adminID, r[3])
		if code != 500 || errCode(body) != "internal_error" {
			t.Errorf("%s %s → %d %v", r[0], r[1], code, body)
		}
		if raw, _ := json.Marshal(body); strings.Contains(string(raw), "secreto") {
			t.Error("el error interno no debe filtrarse")
		}
	}
	if !strings.Contains(e.logs.String(), "reports: error interno") {
		t.Error("el error interno debe registrarse")
	}
}

func TestAdminListShape(t *testing.T) {
	e := newTenv(t)
	next := reportID
	pending := sampleView()
	pending.Report.Status, pending.Report.Resolution = domain.StatusOpen, nil
	e.svc.page = usecase.Page{Items: []usecase.View{pending, sampleView()}, Next: &next}
	code, body := e.do(t, "GET", "/admin/reports?status=open&limit=2", "", adminID, "admin")
	if code != 200 || e.svc.op != "list" || e.svc.status != "open" || e.svc.limit != 2 || e.svc.after != "" {
		t.Fatalf("code=%d svc=%+v", code, e.svc)
	}
	items := body["items"].([]any)
	first := items[0].(map[string]any)
	if first["id"] != reportID || first["status"] != "open" || first["resolution"] != nil || first["details"] != "Detalle" ||
		first["target"].(map[string]any)["summary"] != "Taladro" || first["createdAt"] != "2026-11-17T10:00:00.000Z" {
		t.Errorf("primero = %v", first)
	}
	rep := first["reporter"].(map[string]any)
	if rep["id"] != me || rep["displayName"] != "Ana" || rep["ratingAverage"] != 4.5 || rep["ratingCount"] != float64(3) || rep["memberSince"] != "2026-11-17T10:00:00.000Z" {
		t.Errorf("reporter = %v", rep)
	}
	if _, leaks := rep["email"]; leaks {
		t.Error("el reportante no debe exponer correo")
	}
	res := items[1].(map[string]any)["resolution"].(map[string]any)
	if res["action"] != "hide_item" || res["note"] != "Incumple" || res["at"] != "2026-11-17T11:00:00.000Z" {
		t.Errorf("resolución = %v", res)
	}
	if _, leaks := res["byAdminId"]; leaks {
		t.Error("byAdminId se guarda pero no se expone")
	}
	if raw, err := base64.RawURLEncoding.DecodeString(body["nextCursor"].(string)); err != nil || string(raw) != reportID {
		t.Errorf("nextCursor = %v", body["nextCursor"])
	}
}

func TestAdminListEmptyAndParams(t *testing.T) {
	e := newTenv(t)
	code, body := e.do(t, "GET", "/admin/reports", "", adminID, "admin")
	items, ok := body["items"].([]any)
	if code != 200 || !ok || len(items) != 0 || body["nextCursor"] != nil || e.svc.status != "" {
		t.Errorf("vacío = %d %v", code, body)
	}
	cur := base64.RawURLEncoding.EncodeToString([]byte(reportID))
	if code, _ := e.do(t, "GET", "/admin/reports?cursor="+cur, "", adminID, "admin"); code != 200 || e.svc.after != reportID {
		t.Errorf("cursor válido: %d after=%q", code, e.svc.after)
	}
	for _, q := range []string{"limit=0", "limit=51", "limit=x", "cursor=!!!", "cursor=" + base64.RawURLEncoding.EncodeToString([]byte("nope")), "cursor=" + strings.Repeat("a", 70)} {
		e.svc.op = ""
		code, body := e.do(t, "GET", "/admin/reports?"+q, "", adminID, "admin")
		if code != 400 || errCode(body) != "validation_failed" || e.svc.op != "" {
			t.Errorf("%s → %d %v", q, code, body)
		}
	}
	// Un estado inválido lo rechaza el caso de uso.
	e.svc.err = shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: "status", Issue: "x"})
	if code, body := e.do(t, "GET", "/admin/reports?status=cerrado", "", adminID, "admin"); code != 400 || errCode(body) != "validation_failed" {
		t.Errorf("estado inválido → %d %v", code, body)
	}
}

func TestResolve(t *testing.T) {
	e := newTenv(t)
	e.svc.view = sampleView()
	code, body := e.do(t, "POST", "/admin/reports/"+reportID+"/resolve", `{"action":"hide_item","note":"Incumple"}`, adminID, "admin")
	if code != 200 || body["id"] != reportID || body["status"] != "actioned" {
		t.Fatalf("resolve = %d %v", code, body)
	}
	if e.svc.op != "resolve" || e.svc.user != adminID || e.svc.target != reportID || e.svc.action != "hide_item" || e.svc.note != "Incumple" {
		t.Errorf("svc = %+v", e.svc)
	}
	for _, b := range []string{``, `{}`, `{"action":"dismiss"}`, `{"note":"nota"}`, `{"action":3,"note":"x"}`, `{"action":"dismiss","note":"nota","x":1}`} {
		e.svc.op = ""
		code, body := e.do(t, "POST", "/admin/reports/"+reportID+"/resolve", b, adminID, "admin")
		if code != 400 || errCode(body) != "validation_failed" || e.svc.op != "" {
			t.Errorf("%q → %d %v", b, code, body)
		}
	}
	e.svc.err = domain.ErrAlreadyResolved
	if code, body := e.do(t, "POST", "/admin/reports/"+reportID+"/resolve", `{"action":"dismiss","note":"nota"}`, adminID, "admin"); code != 409 || errCode(body) != "conflict" {
		t.Errorf("ya resuelto → %d %v", code, body)
	}
}

func TestReporterFallsBackToReportReporterID(t *testing.T) {
	e := newTenv(t)
	v := sampleView()
	v.Reporter = users.PublicUser{}
	e.svc.view = v
	_, body := e.do(t, "POST", "/admin/reports/"+reportID+"/resolve", `{"action":"dismiss","note":"nota"}`, adminID, "admin")
	if body["reporter"].(map[string]any)["id"] != me {
		t.Errorf("reporter = %v", body["reporter"])
	}
}
