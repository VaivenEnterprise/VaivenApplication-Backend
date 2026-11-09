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
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/usecase"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

const convID = "6731a0000000000000000001"

type fakeService struct {
	err     error
	page    usecase.Page
	msgs    usecase.MessagePage
	created bool
	user    string
	after   *domain.Cursor
	limit   int
	q       usecase.MessageQuery
	body    string
	cid     *string
	op      string
}

func (f *fakeService) List(_ context.Context, u string, after *domain.Cursor, limit int) (usecase.Page, error) {
	f.user, f.after, f.limit, f.op = u, after, limit, "list"
	return f.page, f.err
}
func (f *fakeService) UnreadCount(_ context.Context, u string) (int, error) {
	f.user, f.op = u, "unread"
	return 7, f.err
}
func (f *fakeService) Messages(_ context.Context, u, _ string, q usecase.MessageQuery) (usecase.MessagePage, error) {
	f.user, f.q, f.op = u, q, "messages"
	return f.msgs, f.err
}
func (f *fakeService) Send(_ context.Context, u, c, body string, cid *string) (domain.Message, bool, error) {
	f.user, f.body, f.cid, f.op = u, body, cid, "send"
	return domain.Message{ID: "m1", ConversationID: c, SenderID: u, Body: body, ClientID: cid}, f.created, f.err
}
func (f *fakeService) Read(_ context.Context, u, _ string) error {
	f.user, f.op = u, "read"
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
	svc := &fakeService{created: true}
	logs := &bytes.Buffer{}
	m, err := NewModule(ModuleDeps{Service: svc, Authenticate: testAuth, Log: slog.New(slog.NewJSONHandler(logs, nil)),
		Limiter: redis.NewMemoryLimiter(), LimiterFallback: redis.NewMemoryLimiter()})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "chat" {
		t.Error("Name")
	}
	r := chi.NewRouter()
	m.Register(r)
	return tenv{svc: svc, h: r, logs: logs}
}

func (e tenv) do(t *testing.T, method, path, body, user string) (int, map[string]any) {
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
	return rec.Code, out
}

func TestNewModuleDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias")
	}
}

func TestList(t *testing.T) {
	e := newTenv(t)
	at := time.Date(2026, 11, 9, 17, 0, 0, 5_000_000, time.UTC)
	thumb := "https://t/p1"
	e.svc.page = usecase.Page{
		Items: []usecase.ConversationView{{
			Conversation: domain.Conversation{ID: convID, BookingID: "b1", Unread: map[string]int{"own": 987654, "req": 3},
				LastMessage: &domain.LastMessage{Preview: "hola", At: at, SenderID: "own"}},
			Booking:     domain.BookingInfo{Status: "accepted", ItemID: "i1", ItemTitle: "Taladro"},
			Counterpart: users.PublicUser{ID: "own", DisplayName: "Ana"},
			ThumbURL:    &thumb, CanWrite: true, Unread: 3,
		}, {Conversation: domain.Conversation{ID: "x"}}},
		Next: &domain.Cursor{At: at, ID: convID},
	}
	code, out := e.do(t, "GET", "/conversations?limit=10", "", "req")
	if code != 200 || e.svc.limit != 10 || e.svc.user != "req" {
		t.Fatalf("list = %d %v", code, out)
	}
	c := out["items"].([]any)[0].(map[string]any)
	if c["unreadCount"] != float64(3) || c["canWrite"] != true || c["bookingStatus"] != "accepted" ||
		c["item"].(map[string]any)["thumbUrl"] != thumb || c["lastMessage"].(map[string]any)["preview"] != "hola" ||
		c["counterpart"].(map[string]any)["displayName"] != "Ana" {
		t.Errorf("conversación = %v", c)
	}
	// RF-08: nunca se expone el contador de la otra parte.
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "unread\"") || strings.Contains(string(raw), "987654") {
		t.Errorf("se filtró el contador ajeno: %s", raw)
	}
	if out["items"].([]any)[1].(map[string]any)["lastMessage"] != nil {
		t.Error("sin mensajes = null")
	}
	_, _ = e.do(t, "GET", "/conversations?cursor="+out["nextCursor"].(string), "", "req")
	if e.svc.after == nil || e.svc.after.ID != convID || !e.svc.after.At.Equal(at) {
		t.Errorf("cursor = %+v", e.svc.after)
	}
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for _, q := range []string{"limit=0", "limit=51", "cursor=***", "cursor=" + enc("1.zz"), "cursor=" + strings.Repeat("a", 129)} {
		if code, _ := e.do(t, "GET", "/conversations?"+q, "", "req"); code != 400 {
			t.Errorf("%s = %d", q, code)
		}
	}
	e.svc.page = usecase.Page{}
	if _, out := e.do(t, "GET", "/conversations", "", "req"); len(out["items"].([]any)) != 0 || out["nextCursor"] != nil {
		t.Error("vacía")
	}
}

func TestUnreadAndRead(t *testing.T) {
	e := newTenv(t)
	if code, out := e.do(t, "GET", "/conversations/unread-count", "", "own"); code != 200 || out["count"] != float64(7) || e.svc.op != "unread" {
		t.Errorf("unread = %d %v (la ruta estática va antes de {id})", code, out)
	}
	if code, _ := e.do(t, "POST", "/conversations/"+convID+"/read", "", "own"); code != 204 || e.svc.op != "read" {
		t.Errorf("read = %d", code)
	}
	e.svc.err = domain.ErrNotFound
	if code, _ := e.do(t, "POST", "/conversations/"+convID+"/read", "", "otro"); code != 404 {
		t.Error("read ajena")
	}
	if code, _ := e.do(t, "GET", "/conversations/unread-count", "", "own"); code != 404 {
		t.Error("error en unread")
	}
	if code, _ := e.do(t, "GET", "/conversations", "", ""); code != 401 {
		t.Error("sin sesión")
	}
}

func TestMessages(t *testing.T) {
	e := newTenv(t)
	next := "6731b0000000000000000003"
	e.svc.msgs = usecase.MessagePage{Items: []domain.Message{{ID: next, Body: "hola"}}, Next: &next}
	code, out := e.do(t, "GET", "/conversations/"+convID+"/messages?limit=5&before="+next, "", "req")
	if code != 200 || out["nextCursor"] != next || e.svc.q.Limit != 5 || e.svc.q.Before != next {
		t.Fatalf("messages = %d %v", code, out)
	}
	if m := out["items"].([]any)[0].(map[string]any); m["body"] != "hola" || m["clientId"] != nil {
		t.Errorf("mensaje = %v", m)
	}
	_, _ = e.do(t, "GET", "/conversations/"+convID+"/messages?after="+next, "", "req")
	if e.svc.q.After != next {
		t.Error("after")
	}
	for _, q := range []string{"limit=0", "limit=x", "before=zz", "after=123"} {
		if code, _ := e.do(t, "GET", "/conversations/"+convID+"/messages?"+q, "", "req"); code != 400 {
			t.Errorf("%s = %d", q, code)
		}
	}
	e.svc.err = domain.ErrNotFound // CA-02
	if code, _ := e.do(t, "GET", "/conversations/"+convID+"/messages", "", "otro"); code != 404 {
		t.Error("ajeno")
	}
}

func TestSend(t *testing.T) {
	e := newTenv(t)
	code, out := e.do(t, "POST", "/conversations/"+convID+"/messages", `{"body":"hola","clientId":"k1"}`, "req")
	if code != 201 || out["body"] != "hola" || out["clientId"] != "k1" || e.svc.body != "hola" || *e.svc.cid != "k1" {
		t.Fatalf("send = %d %v", code, out)
	}
	e.svc.created = false // CA-03
	if code, _ := e.do(t, "POST", "/conversations/"+convID+"/messages", `{"body":"hola","clientId":"k1"}`, "req"); code != 200 {
		t.Errorf("repetido = %d", code)
	}
	for _, b := range []string{`{}`, `no`, `{"body":5}`, `{"body":"a"} {}`} {
		if code, _ := e.do(t, "POST", "/conversations/"+convID+"/messages", b, "req"); code != 400 {
			t.Errorf("%s = %d", b, code)
		}
	}
	for err, want := range map[error]int{
		shared.NewValidation("x"): 400, // CA-05 (lo valida el dominio)
		domain.ErrClosed:          409, // CA-04
		domain.ErrNotFound:        404,
		errors.New("mongo"):       500,
	} {
		e.svc.err = err
		if code, _ := e.do(t, "POST", "/conversations/"+convID+"/messages", `{"body":"hola"}`, "u-"+want2s(want)); code != want {
			t.Errorf("%v = %d", err, code)
		}
	}
	if !strings.Contains(e.logs.String(), "chat: error interno") {
		t.Error("error interno registrado")
	}
}

func want2s(n int) string { return string(rune('a'+n%26)) + string(rune('a'+n/26%26)) }

// RF-11: 30 mensajes por minuto por persona; el 31.º → 429.
func TestRateLimit(t *testing.T) {
	e := newTenv(t)
	for i := range 30 {
		if code, _ := e.do(t, "POST", "/conversations/"+convID+"/messages", `{"body":"hola"}`, "spammer"); code != 201 {
			t.Fatalf("mensaje %d = %d", i+1, code)
		}
	}
	if code, out := e.do(t, "POST", "/conversations/"+convID+"/messages", `{"body":"hola"}`, "spammer"); code != 429 {
		t.Errorf("31.º = %d %v", code, out)
	}
	// Otra persona no se ve afectada; leer no cuenta para el límite.
	if code, _ := e.do(t, "POST", "/conversations/"+convID+"/messages", `{"body":"hola"}`, "otra"); code != 201 {
		t.Error("otra persona")
	}
	if code, _ := e.do(t, "GET", "/conversations/"+convID+"/messages", "", "spammer"); code != 200 {
		t.Error("leer no se limita")
	}
}
