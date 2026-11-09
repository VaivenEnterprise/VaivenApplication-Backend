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

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/usecase"
)

const bid = "6731a0000000000000000001"

type fakeService struct {
	err   error
	user  string
	id    string
	stage bookings.Stage
	note  *string
	op    string
}

func (f *fakeService) Report(_ context.Context, v usecase.Viewer, id string, st bookings.Stage, note *string) error {
	f.user, f.id, f.stage, f.note, f.op = v.ID, id, st, note, "report"
	return f.err
}
func (f *fakeService) Confirm(_ context.Context, v usecase.Viewer, id string, st bookings.Stage) error {
	f.user, f.id, f.stage, f.op = v.ID, id, st, "confirm"
	return f.err
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
			respond.JSON(w, http.StatusOK, map[string]string{"id": id})
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

func (e *tenv) do(path, user, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-User", user)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("cuerpo no es JSON: %v (%s)", err, rec.Body)
	}
	e, _ := m["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func TestNewModuleRequiresDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	m, _ := NewModule(ModuleDeps{Service: &fakeService{}, Authenticate: testAuth, Log: slog.Default(), WriteBooking: func(http.ResponseWriter, *http.Request, string) {}})
	if m.Name() != "payments" {
		t.Errorf("Name = %q", m.Name())
	}
}

func TestRoutesNeedSession(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"report", "confirm"} {
		if rec := e.do("/bookings/"+bid+"/payments/delivery/"+p, "", "{}"); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s sin token = %d", p, rec.Code)
		}
	}
	if e.svc.op != "" {
		t.Error("sin sesión no debe ejecutarse nada")
	}
}

func TestReport(t *testing.T) {
	e := newEnv(t)
	rec := e.do("/bookings/"+bid+"/payments/return/report", "req", `{"note":"SPEI"}`)
	if rec.Code != http.StatusOK || e.written != bid {
		t.Fatalf("estado %d, reserva escrita %q", rec.Code, e.written)
	}
	s := e.svc
	if s.op != "report" || s.user != "req" || s.id != bid || s.stage != bookings.StageReturn || s.note == nil || *s.note != "SPEI" {
		t.Errorf("el caso de uso recibió %+v", s)
	}
	// La nota es opcional: cuerpo vacío, objeto vacío o nota nula.
	for _, body := range []string{``, `{}`, `{"note":null}`} {
		e := newEnv(t)
		if rec := e.do("/bookings/"+bid+"/payments/delivery/report", "req", body); rec.Code != http.StatusOK || e.svc.note != nil {
			t.Errorf("cuerpo %q: estado %d, nota %v", body, rec.Code, e.svc.note)
		}
	}
	for name, body := range map[string]string{"no JSON": `{`, "tipo inválido": `{"note":5}`, "dos objetos": `{}{}`} {
		e := newEnv(t)
		if rec := e.do("/bookings/"+bid+"/payments/delivery/report", "req", body); rec.Code != http.StatusBadRequest || e.svc.op != "" {
			t.Errorf("%s: estado %d, op %q", name, rec.Code, e.svc.op)
		}
	}
}

func TestConfirm(t *testing.T) {
	e := newEnv(t)
	rec := e.do("/bookings/"+bid+"/payments/delivery/confirm", "owner", "")
	if rec.Code != http.StatusOK || e.written != bid {
		t.Fatalf("estado %d, reserva escrita %q", rec.Code, e.written)
	}
	if e.svc.op != "confirm" || e.svc.user != "owner" || e.svc.stage != bookings.StageDelivery {
		t.Errorf("el caso de uso recibió %+v", e.svc)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{bookings.ErrNotFound, 404, "not_found"},
		{domain.ErrNoPayment, 404, "not_found"},
		{domain.ErrForbidden, 403, "forbidden"},
		{domain.ErrNotPending, 409, "conflict"},
		{domain.ErrNotReported, 409, "conflict"},
		{domain.ErrWrongBookingStatus, 409, "conflict"},
		{shared.NewValidation("Revisa los datos marcados."), 400, "validation_failed"},
		{errors.New("mongo: conexión rota con secreto"), 500, "internal_error"},
	}
	for _, c := range cases {
		for _, p := range []string{"report", "confirm"} {
			e := newEnv(t)
			e.svc.err = c.err
			rec := e.do("/bookings/"+bid+"/payments/delivery/"+p, "u1", "{}")
			if rec.Code != c.status || errCode(t, rec) != c.code || e.written != "" {
				t.Errorf("%v en %s: %d %s (escrita %q), quiero %d %s", c.err, p, rec.Code, errCode(t, rec), e.written, c.status, c.code)
			}
			if c.status == 500 {
				if strings.Contains(rec.Body.String(), "secreto") {
					t.Error("el error interno no debe filtrarse al cliente")
				}
				if !strings.Contains(e.logs.String(), "error interno") {
					t.Error("el error interno debe registrarse")
				}
			}
		}
	}
}
