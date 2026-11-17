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
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/usecase"
)

const (
	adminID = "6730f1c2a9b3e4d5f6a7b8c4"
	userID  = "6730f1c2a9b3e4d5f6a7b8c1"
)

type fakeService struct {
	err   error
	admin string
	user  string
	in    usecase.Input
	calls int
}

func (f *fakeService) Apply(_ context.Context, admin, user string, in usecase.Input) (domain.Sanction, error) {
	f.calls++
	f.admin, f.user, f.in = admin, user, in
	if f.err != nil {
		return domain.Sanction{}, f.err
	}
	return domain.Sanction{
		ID: "6732c0000000000000000001", UserID: user, Type: in.Type, Reason: in.Reason, EndsAt: in.EndsAt,
		IncidentID: in.IncidentID, CreatedByAdminID: admin, CreatedAt: time.Date(2026, 11, 21, 16, 0, 0, 0, time.UTC),
	}, nil
}

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

func testAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.Role(r.Context()) != "admin" {
			respond.Error(w, r, shared.NewForbidden("No tienes permiso para realizar esta acción."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func setup(t *testing.T) (*fakeService, http.Handler, *bytes.Buffer) {
	t.Helper()
	svc := &fakeService{}
	logs := &bytes.Buffer{}
	m, err := NewModule(ModuleDeps{Service: svc, Authenticate: testAuth, RequireAdmin: testAdmin, Log: slog.New(slog.NewJSONHandler(logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "sanctions" {
		t.Error("Name")
	}
	r := chi.NewRouter()
	m.Register(r)
	return svc, r, logs
}

func do(h http.Handler, body, user, role string) (int, map[string]any) {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest("POST", "/admin/users/"+userID+"/sanctions", rd)
	if user != "" {
		req.Header.Set("X-User", user)
	}
	req.Header.Set("X-Role", role)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func TestNewModuleDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias")
	}
}

func TestRequiresSessionAndAdminRole(t *testing.T) { // CA-05
	svc, h, _ := setup(t)
	if code, _ := do(h, `{"type":"warning","reason":"Motivo válido."}`, "", ""); code != 401 {
		t.Errorf("sin sesión = %d", code)
	}
	code, out := do(h, `{"type":"warning","reason":"Motivo válido."}`, userID, "user")
	if code != 403 || errCode(out) != "forbidden" {
		t.Errorf("usuario común = %d %v", code, out)
	}
	if svc.calls != 0 {
		t.Error("un no-admin no llega al caso de uso")
	}
}

func TestApplySuspension(t *testing.T) { // CA-06
	svc, h, _ := setup(t)
	code, out := do(h, `{"type":"suspension","reason":"Daño reiterado.","endsAt":"2026-12-01T00:00:00Z","incidentId":"6731c0000000000000000001"}`, adminID, "admin")
	if code != 201 {
		t.Fatalf("%d %v", code, out)
	}
	if svc.admin != adminID || svc.user != userID || svc.in.Type != domain.TypeSuspension || svc.in.EndsAt == nil || svc.in.IncidentID == nil {
		t.Errorf("entrada = %+v", svc)
	}
	if out["type"] != "suspension" || out["userId"] != userID || out["endsAt"] != "2026-12-01T00:00:00Z" || out["incidentId"] != "6731c0000000000000000001" ||
		out["createdAt"] != "2026-11-21T16:00:00Z" || out["reason"] != "Daño reiterado." {
		t.Errorf("respuesta = %v", out)
	}
	if _, has := out["createdByAdminId"]; has {
		t.Error("no se expone qué admin sancionó")
	}
}

func TestApplyWarningHasNullDates(t *testing.T) {
	_, h, _ := setup(t)
	code, out := do(h, `{"type":"warning","reason":"Primera advertencia."}`, adminID, "admin")
	if code != 201 || out["endsAt"] != nil || out["incidentId"] != nil {
		t.Errorf("%d %v", code, out)
	}
	if _, has := out["endsAt"]; !has {
		t.Error("endsAt debe venir como null")
	}
}

func TestApplyRejectsBadBodiesAndMapsErrors(t *testing.T) {
	svc, h, logs := setup(t)
	for name, body := range map[string]string{
		"no JSON": `x`, "tipo": `{"type":3}`, "fecha mala": `{"type":"suspension","reason":"Motivo válido.","endsAt":"mañana"}`, "dos": `{}{}`,
	} {
		if code, out := do(h, body, adminID, "admin"); code != 400 || errCode(out) != "validation_failed" {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if svc.calls != 0 {
		t.Error("un cuerpo inválido no llega al caso de uso")
	}
	svc.err = shared.NewNotFound("No encontramos a esa persona.")
	if code, _ := do(h, `{"type":"warning","reason":"Motivo válido."}`, adminID, "admin"); code != 404 {
		t.Errorf("persona inexistente = %d", code)
	}
	svc.err = errors.New("mongo caído")
	code, out := do(h, `{"type":"warning","reason":"Motivo válido."}`, adminID, "admin")
	if code != 500 || errCode(out) != "internal_error" || !strings.Contains(logs.String(), "sanctions: error interno") {
		t.Errorf("500: %d %v", code, out)
	}
}
