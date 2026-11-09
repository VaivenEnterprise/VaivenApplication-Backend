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
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/usecase"
)

const (
	me   = "6730f1c2a9b3e4d5f6a7b8c1"
	id1  = "6731e0000000000000000001"
	id2  = "6731e0000000000000000002"
	tokA = "ExponentPushToken[abc]"
)

var at = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

type fakeService struct {
	err              error
	page             usecase.Page
	count            int64
	op, user, cursor string
	limit            int
	ids              []string
	all              bool
	token, platform  string
}

func (f *fakeService) List(_ context.Context, u, after string, limit int) (usecase.Page, error) {
	f.op, f.user, f.cursor, f.limit = "list", u, after, limit
	return f.page, f.err
}
func (f *fakeService) UnreadCount(_ context.Context, u string) (int64, error) {
	f.op, f.user = "unread", u
	return f.count, f.err
}
func (f *fakeService) MarkRead(_ context.Context, u string, ids []string, all bool) error {
	f.op, f.user, f.ids, f.all = "read", u, ids, all
	return f.err
}
func (f *fakeService) RegisterToken(_ context.Context, u, tok, p string) error {
	f.op, f.user, f.token, f.platform = "register", u, tok, p
	return f.err
}
func (f *fakeService) UnregisterToken(_ context.Context, u, tok string) error {
	f.op, f.user, f.token = "unregister", u, tok
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
	svc  *fakeService
	h    http.Handler
	logs *bytes.Buffer
}

func newTenv(t *testing.T) tenv {
	t.Helper()
	svc := &fakeService{}
	logs := &bytes.Buffer{}
	m, err := NewModule(ModuleDeps{Service: svc, Authenticate: testAuth, Log: slog.New(slog.NewJSONHandler(logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "notifications" {
		t.Error("Name")
	}
	r := chi.NewRouter()
	m.Register(r)
	return tenv{svc: svc, h: r, logs: logs}
}

func (e tenv) do(t *testing.T, method, path, body, user string) (int, map[string]any, string) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if user != "" {
		req.Header.Set("X-User", user)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
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

func TestRoutesRequireSession(t *testing.T) {
	e := newTenv(t)
	for _, p := range [][2]string{
		{"GET", "/me/notifications"}, {"GET", "/me/notifications/unread-count"}, {"POST", "/me/notifications/read"},
		{"POST", "/me/push-tokens"}, {"DELETE", "/me/push-tokens"},
	} {
		if code, _, _ := e.do(t, p[0], p[1], `{"all":true}`, ""); code != 401 {
			t.Errorf("%s %s = %d, quiero 401", p[0], p[1], code)
		}
	}
	if e.svc.op != "" {
		t.Errorf("sin sesión no se debe llamar al servicio (%s)", e.svc.op)
	}
}

func TestListShape(t *testing.T) {
	e := newTenv(t)
	read := at.Add(time.Minute)
	next := id2
	e.svc.page = usecase.Page{Next: &next, Items: []domain.Notification{
		{ID: id1, Type: "booking_requested", Title: "T", Body: "B", Data: map[string]string{"bookingId": "b1"}, CreatedAt: at},
		{ID: id2, Type: "rating_received", Title: "T2", Body: "B2", ReadAt: &read, CreatedAt: at},
	}}
	code, body, _ := e.do(t, "GET", "/me/notifications?limit=2", "", me)
	if code != 200 || e.svc.op != "list" || e.svc.user != me || e.svc.limit != 2 || e.svc.cursor != "" {
		t.Fatalf("code=%d svc=%+v", code, e.svc)
	}
	items := body["items"].([]any)
	first := items[0].(map[string]any)
	if first["id"] != id1 || first["readAt"] != nil || first["data"].(map[string]any)["bookingId"] != "b1" || first["createdAt"] != "2026-11-17T10:00:00.000Z" {
		t.Errorf("primer aviso = %v", first)
	}
	second := items[1].(map[string]any)
	if second["readAt"] != "2026-11-17T10:01:00.000Z" || len(second["data"].(map[string]any)) != 0 {
		t.Errorf("segundo aviso = %v", second)
	}
	cur, _ := body["nextCursor"].(string)
	if raw, err := base64.RawURLEncoding.DecodeString(cur); err != nil || string(raw) != id2 {
		t.Errorf("nextCursor = %q", cur)
	}
}

func TestListEmptyAndLastPage(t *testing.T) {
	e := newTenv(t)
	code, body, _ := e.do(t, "GET", "/me/notifications", "", me)
	items, ok := body["items"].([]any)
	if code != 200 || !ok || len(items) != 0 || body["nextCursor"] != nil {
		t.Errorf("lista vacía = %d %v", code, body)
	}
}

func TestListCursorAndParams(t *testing.T) {
	e := newTenv(t)
	cur := base64.RawURLEncoding.EncodeToString([]byte(id1))
	if code, _, _ := e.do(t, "GET", "/me/notifications?cursor="+cur, "", me); code != 200 || e.svc.cursor != id1 {
		t.Errorf("cursor válido: code=%d cursor=%q", code, e.svc.cursor)
	}
	for _, q := range []string{"limit=0", "limit=51", "limit=abc", "cursor=!!!", "cursor=" + base64.RawURLEncoding.EncodeToString([]byte("nope")), "cursor=" + strings.Repeat("a", 70)} {
		e.svc.op = ""
		code, body, _ := e.do(t, "GET", "/me/notifications?"+q, "", me)
		if code != 400 || errCode(body) != "validation_failed" || e.svc.op != "" {
			t.Errorf("%s → %d %v", q, code, body)
		}
	}
}

func TestUnreadCount(t *testing.T) {
	e := newTenv(t)
	e.svc.count = 4
	code, body, _ := e.do(t, "GET", "/me/notifications/unread-count", "", me)
	if code != 200 || body["count"] != float64(4) || e.svc.user != me {
		t.Errorf("unread = %d %v", code, body)
	}
}

func TestMarkRead(t *testing.T) {
	e := newTenv(t)
	if code, _, raw := e.do(t, "POST", "/me/notifications/read", `{"all":true}`, me); code != 204 || raw != "" || !e.svc.all || len(e.svc.ids) != 0 || e.svc.user != me {
		t.Errorf("read all: code=%d svc=%+v", code, e.svc)
	}
	if code, _, _ := e.do(t, "POST", "/me/notifications/read", `{"ids":["`+id1+`","`+id2+`"]}`, me); code != 204 || e.svc.all || len(e.svc.ids) != 2 {
		t.Errorf("read ids: code=%d svc=%+v", code, e.svc)
	}
	if code, _, _ := e.do(t, "POST", "/me/notifications/read", `{"all":false,"ids":["`+id1+`"]}`, me); code != 204 || e.svc.all {
		t.Errorf("all=false con ids: code=%d", code)
	}
}

func TestMarkReadBadBodies(t *testing.T) {
	e := newTenv(t)
	for _, b := range []string{`no es json`, `{"ids":"x"}`, `{"all":"si"}`, `{"all":true} {}`, `{"otra":1}`} {
		e.svc.op = ""
		code, body, _ := e.do(t, "POST", "/me/notifications/read", b, me)
		if code != 400 || errCode(body) != "validation_failed" || e.svc.op != "" {
			t.Errorf("%q → %d %v", b, code, body)
		}
	}
	e.svc.err = shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: "ids", Issue: "x"})
	if code, body, _ := e.do(t, "POST", "/me/notifications/read", `{}`, me); code != 400 || errCode(body) != "validation_failed" {
		t.Errorf("el error de validación del servicio sube: %d %v", code, body)
	}
}

func TestRegisterToken(t *testing.T) {
	e := newTenv(t)
	if code, _, raw := e.do(t, "POST", "/me/push-tokens", `{"token":"`+tokA+`","platform":"ios"}`, me); code != 204 || raw != "" {
		t.Fatalf("code=%d", code)
	}
	if e.svc.op != "register" || e.svc.user != me || e.svc.token != tokA || e.svc.platform != "ios" {
		t.Errorf("svc = %+v", e.svc)
	}
	for _, b := range []string{`{}`, `{"token":"` + tokA + `"}`, `{"platform":"ios"}`, `{"token":1,"platform":"ios"}`, `nada`} {
		e.svc.op = ""
		code, body, _ := e.do(t, "POST", "/me/push-tokens", b, me)
		if code != 400 || errCode(body) != "validation_failed" || e.svc.op != "" {
			t.Errorf("%q → %d %v", b, code, body)
		}
	}
}

func TestDeleteToken(t *testing.T) {
	e := newTenv(t)
	if code, _, raw := e.do(t, "DELETE", "/me/push-tokens", `{"token":"`+tokA+`"}`, me); code != 204 || raw != "" {
		t.Fatalf("code=%d", code)
	}
	if e.svc.op != "unregister" || e.svc.user != me || e.svc.token != tokA {
		t.Errorf("svc = %+v", e.svc)
	}
	e.svc.op = ""
	for _, b := range []string{``, `{}`, `{"token":5}`} {
		code, body, _ := e.do(t, "DELETE", "/me/push-tokens", b, me)
		if code != 400 || errCode(body) != "validation_failed" || e.svc.op != "" {
			t.Errorf("%q → %d %v", b, code, body)
		}
	}
}

func TestInternalErrorIsLoggedAndHidden(t *testing.T) {
	e := newTenv(t)
	e.svc.err = errors.New("mongo caído: secreto")
	for _, c := range []struct{ m, p, b string }{
		{"GET", "/me/notifications", ""}, {"GET", "/me/notifications/unread-count", ""},
		{"POST", "/me/notifications/read", `{"all":true}`},
		{"POST", "/me/push-tokens", `{"token":"` + tokA + `","platform":"ios"}`}, {"DELETE", "/me/push-tokens", `{"token":"` + tokA + `"}`},
	} {
		code, body, raw := e.do(t, c.m, c.p, c.b, me)
		if code != 500 || errCode(body) != "internal_error" || strings.Contains(raw, "secreto") {
			t.Errorf("%s %s → %d %s", c.m, c.p, code, raw)
		}
	}
	if !strings.Contains(e.logs.String(), "notifications: error interno") {
		t.Error("el error interno debe registrarse")
	}
}
