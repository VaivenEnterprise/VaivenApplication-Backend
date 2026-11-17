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
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/usecase"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

const bid = "6731a0000000000000000001"

var t0 = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

type fakeService struct {
	err   error
	view  usecase.StageView
	qr    usecase.QRIssued
	user  string
	id    string
	kind  domain.Kind
	notes string
	refs  []images.Ref
	token string
	op    string
}

func (f *fakeService) RecordEvidence(_ context.Context, v usecase.Viewer, id string, k domain.Kind, notes string, refs []images.Ref) (usecase.StageView, error) {
	f.user, f.id, f.kind, f.notes, f.refs, f.op = v.ID, id, k, notes, refs, "evidence"
	return f.view, f.err
}
func (f *fakeService) IssueQR(_ context.Context, v usecase.Viewer, id string, k domain.Kind) (usecase.QRIssued, error) {
	f.user, f.id, f.kind, f.op = v.ID, id, k, "qr"
	return f.qr, f.err
}
func (f *fakeService) Confirm(_ context.Context, v usecase.Viewer, id string, k domain.Kind, token string) error {
	f.user, f.id, f.kind, f.token, f.op = v.ID, id, k, token, "confirm"
	return f.err
}
func (f *fakeService) GetStage(_ context.Context, v usecase.Viewer, id string, k domain.Kind) (usecase.StageView, error) {
	f.user, f.id, f.kind, f.op = v.ID, id, k, "stage"
	return f.view, f.err
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

type tenv struct {
	svc     *fakeService
	h       http.Handler
	logs    *bytes.Buffer
	written string
}

func newEnv(t *testing.T) *tenv {
	t.Helper()
	e := &tenv{svc: &fakeService{}, logs: &bytes.Buffer{}}
	m, err := NewModule(ModuleDeps{
		Service: e.svc, Authenticate: testAuth, Log: slog.New(slog.NewTextHandler(e.logs, nil)),
		WriteBooking: func(w http.ResponseWriter, _ *http.Request, id string) {
			e.written = id
			respond.JSON(w, http.StatusOK, map[string]string{"id": id, "status": "in_progress"})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	m.Register(r)
	e.h = r
	return e
}

func (e *tenv) do(method, path, user, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-User", user)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("cuerpo no es JSON: %v (%s)", err, rec.Body.String())
	}
	return m
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	e, _ := decodeBody(t, rec)["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func TestNewModuleRequiresDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	m, _ := NewModule(ModuleDeps{Service: &fakeService{}, Authenticate: testAuth, Log: slog.Default(), WriteBooking: func(http.ResponseWriter, *http.Request, string) {}})
	if m.Name() != "handover" {
		t.Errorf("Name = %q", m.Name())
	}
}

func TestAllRoutesNeedSession(t *testing.T) {
	e := newEnv(t)
	for _, k := range []string{"delivery", "return"} {
		for method, suffix := range map[string]string{"PUT": "/evidence", "POST": "/qr"} {
			if rec := e.do(method, "/bookings/"+bid+"/"+k+suffix, "", "{}"); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s%s sin token = %d", method, k, suffix, rec.Code)
			}
		}
		if rec := e.do("POST", "/bookings/"+bid+"/"+k+"/confirm", "", "{}"); rec.Code != http.StatusUnauthorized {
			t.Errorf("confirm %s sin token = %d", k, rec.Code)
		}
		if rec := e.do("GET", "/bookings/"+bid+"/"+k, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s sin token = %d", k, rec.Code)
		}
	}
	if e.svc.op != "" {
		t.Error("ningún caso de uso debe ejecutarse sin sesión")
	}
}

func TestEvidence(t *testing.T) {
	e := newEnv(t)
	by, at := "owner", t0
	e.svc.view = usecase.StageView{
		Kind: domain.KindDelivery,
		Stage: bookings.Handover{RecordedAt: &at, RecordedByID: &by, ConditionNotes: "Sin daños", EvidenceVersion: 1,
			Photos: []images.Image{{PublicID: "p1", Width: 10, Height: 20, Format: "jpg", Bytes: 5, Delivery: images.DeliveryAuthenticated}}},
		URLs: []images.URLs{{URL: "https://img/p1", ThumbURL: "https://img/t/p1"}},
		QR:   domain.QRNone,
	}
	rec := e.do("PUT", "/bookings/"+bid+"/delivery/evidence", "owner", `{"conditionNotes":"Sin daños","photos":[{"publicId":"p1"},{"publicId":"p2"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("estado %d: %s", rec.Code, rec.Body)
	}
	s := e.svc
	if s.user != "owner" || s.id != bid || s.kind != domain.KindDelivery || s.notes != "Sin daños" || len(s.refs) != 2 || s.refs[1].PublicID != "p2" {
		t.Errorf("el caso de uso recibió %+v", s)
	}
	body := decodeBody(t, rec)
	photos := body["photos"].([]any)
	if body["evidenceVersion"] != float64(1) || len(photos) != 1 || photos[0].(map[string]any)["url"] != "https://img/p1" ||
		body["qr"].(map[string]any)["state"] != "none" || body["qr"].(map[string]any)["expiresAt"] != nil || body["confirmedAt"] != nil {
		t.Errorf("Stage = %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "tokenHash") {
		t.Error("la respuesta no debe incluir el hash del token")
	}

	// La devolución usa la misma ruta con la otra etapa.
	e.do("PUT", "/bookings/"+bid+"/return/evidence", "req", `{"conditionNotes":"abc","photos":[{"publicId":"r"}]}`)
	if e.svc.kind != domain.KindReturn || e.svc.user != "req" {
		t.Errorf("devolución: %+v", e.svc)
	}
}

func TestEvidenceBodyValidation(t *testing.T) {
	cases := map[string]string{
		"vacío":                   ``,
		"no es JSON":              `{`,
		"dos objetos":             `{"conditionNotes":"abc","photos":[]}{}`,
		"sin campos":              `{}`,
		"tipo inválido":           `{"conditionNotes":5,"photos":[]}`,
		"foto sin publicId":       `{"conditionNotes":"abc","photos":[{}]}`,
		"publicId vacío":          `{"conditionNotes":"abc","photos":[{"publicId":""}]}`,
		"photos no es un arreglo": `{"conditionNotes":"abc","photos":"x"}`,
	}
	for name, body := range cases {
		e := newEnv(t)
		rec := e.do("PUT", "/bookings/"+bid+"/delivery/evidence", "owner", body)
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != "validation_failed" {
			t.Errorf("%s: estado %d (%s)", name, rec.Code, rec.Body)
		}
		if e.svc.op != "" {
			t.Errorf("%s: no debe llegar al caso de uso", name)
		}
	}
	e := newEnv(t)
	rec := e.do("PUT", "/bookings/"+bid+"/delivery/evidence", "owner", `{"conditionNotes":"abc","photos":[{"publicId":"a"},{}]}`)
	if !strings.Contains(rec.Body.String(), "photos[1].publicId") {
		t.Errorf("el detalle debe nombrar photos[1].publicId: %s", rec.Body)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{bookings.ErrNotFound, 404, "not_found"},
		{domain.ErrForbidden, 403, "forbidden"},
		{domain.ErrWrongStatus, 409, "conflict"},
		{domain.ErrAlreadyConfirmed, 409, "conflict"},
		{domain.ErrQRExpired, 409, "conflict"},
		{domain.ErrQRUsed, 409, "conflict"},
		{domain.ErrQRInvalid, 409, "conflict"},
		{domain.ErrNoEvidence, 409, "conflict"},
		{domain.ErrOutsideWindow, 409, "conflict"},
		{domain.ErrConditionNotes, 400, "validation_failed"},
		{errors.New("mongo: conexión rota con secreto"), 500, "internal_error"},
	}
	for _, c := range cases {
		e := newEnv(t)
		e.svc.err = c.err
		for _, req := range []struct{ m, p, b string }{
			{"PUT", "/delivery/evidence", `{"conditionNotes":"abc","photos":[{"publicId":"a"}]}`},
			{"POST", "/delivery/qr", ``},
			{"POST", "/return/confirm", `{"token":"x"}`},
			{"GET", "/delivery", ``},
		} {
			rec := e.do(req.m, "/bookings/"+bid+req.p, "u1", req.b)
			if rec.Code != c.status || errCode(t, rec) != c.code {
				t.Errorf("%v en %s %s: %d %s, quiero %d %s", c.err, req.m, req.p, rec.Code, errCode(t, rec), c.status, c.code)
			}
			if c.status == 500 && strings.Contains(rec.Body.String(), "secreto") {
				t.Error("el error interno no debe filtrarse al cliente")
			}
		}
		if c.status == 500 && !strings.Contains(e.logs.String(), "error interno") {
			t.Error("el error interno debe registrarse")
		}
	}
}

func TestIssueQR(t *testing.T) {
	e := newEnv(t)
	e.svc.qr = usecase.QRIssued{Token: "secreto-del-qr", Payload: "vaiven://qr?b=" + bid + "&s=delivery&t=secreto-del-qr", ExpiresAt: t0.Add(15 * time.Minute)}
	rec := e.do("POST", "/bookings/"+bid+"/delivery/qr", "owner", "")
	if rec.Code != http.StatusCreated || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("estado %d, cache-control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	body := decodeBody(t, rec)
	if body["token"] != "secreto-del-qr" || body["qrPayload"] != e.svc.qr.Payload || body["expiresAt"] != "2026-11-20T18:15:00Z" {
		t.Errorf("respuesta = %s", rec.Body)
	}
	if strings.Contains(e.logs.String(), "secreto-del-qr") {
		t.Error("RNF-01: el token no puede aparecer en los logs")
	}
}

func TestConfirm(t *testing.T) {
	e := newEnv(t)
	rec := e.do("POST", "/bookings/"+bid+"/delivery/confirm", "req", `{"token":"abc"}`)
	if rec.Code != http.StatusOK || e.written != bid {
		t.Fatalf("estado %d, reserva escrita %q", rec.Code, e.written)
	}
	if e.svc.token != "abc" || e.svc.user != "req" || e.svc.kind != domain.KindDelivery {
		t.Errorf("el caso de uso recibió %+v", e.svc)
	}
	if decodeBody(t, rec)["status"] != "in_progress" {
		t.Errorf("respuesta = %s", rec.Body)
	}
	for name, body := range map[string]string{"sin token": `{}`, "vacío": ``, "no JSON": `x`} {
		e := newEnv(t)
		rec := e.do("POST", "/bookings/"+bid+"/return/confirm", "owner", body)
		if rec.Code != http.StatusBadRequest || e.svc.op != "" {
			t.Errorf("%s: estado %d, op %q", name, rec.Code, e.svc.op)
		}
	}
	e = newEnv(t)
	e.svc.err = domain.ErrQRExpired
	if rec := e.do("POST", "/bookings/"+bid+"/delivery/confirm", "req", `{"token":"abc"}`); rec.Code != http.StatusConflict || e.written != "" {
		t.Errorf("con error no debe escribir la reserva: %d %q", rec.Code, e.written)
	}
}

func TestGetStageEmpty(t *testing.T) {
	e := newEnv(t)
	e.svc.view = usecase.StageView{Kind: domain.KindReturn, QR: domain.QRNone}
	rec := e.do("GET", "/bookings/"+bid+"/return", "owner", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("estado %d", rec.Code)
	}
	body := decodeBody(t, rec)
	if body["photos"] == nil || len(body["photos"].([]any)) != 0 || body["recordedAt"] != nil || body["conditionNotes"] != "" || body["evidenceVersion"] != float64(0) {
		t.Errorf("etapa vacía = %s", rec.Body)
	}
	if e.svc.kind != domain.KindReturn || e.svc.op != "stage" {
		t.Errorf("%+v", e.svc)
	}
}
