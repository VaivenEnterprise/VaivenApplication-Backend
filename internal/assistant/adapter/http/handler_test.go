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

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	searchdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

type fakeSvc struct {
	reply   usecase.Reply
	err     error
	in      usecase.SendInput
	user    string
	deleted string
	delErr  error
}

func (f *fakeSvc) Send(_ context.Context, userID string, in usecase.SendInput) (usecase.Reply, error) {
	f.user, f.in = userID, in
	return f.reply, f.err
}

func (f *fakeSvc) Delete(_ context.Context, _, id string) error {
	f.deleted = id
	return f.delErr
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
	m, err := NewModule(ModuleDeps{
		Service: svc, Authenticate: fakeAuth, Log: slog.New(slog.NewJSONHandler(&logBuf, nil)),
		Limiter: redis.NewMemoryLimiter(),
		Now:     func() time.Time { return time.Date(2026, 11, 16, 23, 30, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "assistant" {
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

func post(h http.Handler, body, user string) *httptest.ResponseRecorder {
	return do(h, http.MethodPost, "/api/v1/ai/assistant/messages", body, user)
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"requestId"`
			Details   []struct {
				Field string `json:"field"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || b.Error.RequestID == "" {
		t.Fatalf("no es el envoltorio del Art. IV: %s", rec.Body)
	}
	if len(b.Error.Details) > 0 {
		return b.Error.Code + ":" + b.Error.Details[0].Field
	}
	return b.Error.Code
}

func sampleReply() usecase.Reply {
	loc := &searchdomain.Point{Lat: 25.67, Lng: -100.31}
	thumb := "https://res.cloudinary.com/n/image/upload/t/x.jpg"
	return usecase.Reply{
		ConversationID: "6735a0c1b2d3e4f5a6b7c8d9",
		Summary:        "Necesitas escalera y lona. Por ahora nadie presta cerca de ti: lona.",
		Needs: []usecase.NeedResult{
			{Need: aidomain.Need{Name: "Escalera", Category: "hogar"}, Matches: []searchusecase.CardView{{
				Card: searchdomain.Card{ID: "i1", Title: "Escalera de aluminio", Category: "hogar", Modality: "rent",
					PricePerDayCents: 5000, Location: loc, Zone: searchdomain.Zone{Municipality: "Monterrey"}},
				ThumbURL: &thumb,
			}}},
			{Need: aidomain.Need{Name: "Lona", Category: "hogar", Optional: true}, Matches: []searchusecase.CardView{}},
		},
		Steps: []string{"Cubre el piso."},
	}
}

func TestMessageOKShapesTheContract(t *testing.T) {
	svc := &fakeSvc{reply: sampleReply()}
	h, logs := newRouter(t, svc)
	rec := post(h, `{"message":"Quiero pintar mi cuarto secreto","location":{"lat":25.6714,"lng":-100.3097}}`, "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	reply := got["reply"].(map[string]any)
	needs := reply["needs"].([]any)
	first := needs[0].(map[string]any)
	card := first["matches"].([]any)[0].(map[string]any)
	if got["conversationId"] != "6735a0c1b2d3e4f5a6b7c8d9" || card["id"] != "i1" || card["pricePerDayCents"] != float64(5000) ||
		card["approxLocation"] == nil || card["thumbUrl"] == nil {
		t.Errorf("respuesta = %s", rec.Body)
	}
	second := needs[1].(map[string]any)
	if !strings.Contains(rec.Body.String(), `"matches":[]`) || second["optional"] != true {
		t.Errorf("sin coincidencias debe ser [] (CA-08): %s", rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"safetyNotes":[]`) {
		t.Errorf("listas vacías deben ser []: %s", rec.Body)
	}
	if svc.user != "u1" || svc.in.Location == nil || svc.in.Location.Lat != 25.6714 || svc.in.Message != "Quiero pintar mi cuarto secreto" {
		t.Errorf("entrada al caso de uso = %+v", svc.in)
	}
	if strings.Contains(logs.String(), "secreto") || strings.Contains(logs.String(), "25.67") {
		t.Errorf("los registros no llevan el mensaje ni la ubicación: %s", logs)
	}
}

func TestMessageValidation(t *testing.T) {
	h, _ := newRouter(t, &fakeSvc{})
	cases := map[string]string{
		`no json`:         "validation_failed",
		`{}`:              "validation_failed:message",
		`{"message":"a"}`: "validation_failed:message",
		`{"message":"` + strings.Repeat("ñ", 501) + `"}`:   "validation_failed:message",
		`{"message":"hola","extra":1}`:                     "validation_failed",
		`{"message":"hola","conversationId":"xyz"}`:        "validation_failed:conversationId",
		`{"message":"hola","location":{"lat":91,"lng":0}}`: "validation_failed:location",
		`{"message":"hola","location":{"lat":0}}`:          "validation_failed:location",
		`{"message":"hola"}{}`:                             "validation_failed",
	}
	for body, want := range cases {
		rec := post(h, body, "u1")
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != want {
			t.Errorf("%.60s: %d %s (want %s)", body, rec.Code, rec.Body, want)
		}
	}
	if rec := post(h, `{"message":"`+strings.Repeat("ñ", 500)+`"}`, "u1"); rec.Code != http.StatusOK {
		t.Errorf("500 caracteres (con acentos) son válidos: %d", rec.Code)
	}
}

func TestMessageErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{usecase.ErrDisabled, http.StatusServiceUnavailable, "service_unavailable"},
		{usecase.ErrNoReply.Wrap(aidomain.ErrUnavailable), http.StatusServiceUnavailable, "service_unavailable"},
		{domain.ErrConversationNotFound, http.StatusNotFound, "not_found"},
		{usecase.ErrDailyLimit, http.StatusTooManyRequests, "rate_limited"},
		{errors.New("mongo caído"), http.StatusInternalServerError, "internal_error"},
	}
	for _, c := range cases {
		h, _ := newRouter(t, &fakeSvc{err: c.err})
		rec := post(h, `{"message":"quiero pintar"}`, "u1")
		if rec.Code != c.status || errCode(t, rec) != c.code {
			t.Errorf("%v: %d %s", c.err, rec.Code, rec.Body)
		}
		if c.err == usecase.ErrDailyLimit && rec.Header().Get("Retry-After") != "1800" {
			t.Errorf("Retry-After = %q", rec.Header().Get("Retry-After"))
		}
	}
}

func TestDeleteConversation(t *testing.T) {
	svc := &fakeSvc{}
	h, _ := newRouter(t, svc)
	rec := do(h, http.MethodDelete, "/api/v1/ai/assistant/conversations/6735a0c1b2d3e4f5a6b7c8d9", "", "u1")
	if rec.Code != http.StatusNoContent || svc.deleted != "6735a0c1b2d3e4f5a6b7c8d9" {
		t.Errorf("%d %s", rec.Code, svc.deleted)
	}
	svc.delErr = domain.ErrConversationNotFound
	if rec := do(h, http.MethodDelete, "/api/v1/ai/assistant/conversations/otro", "", "u1"); rec.Code != http.StatusNotFound {
		t.Errorf("ajena o inexistente: %d", rec.Code)
	}
}

func TestSessionAndPerMinuteLimit(t *testing.T) {
	h, _ := newRouter(t, &fakeSvc{reply: usecase.Reply{ConversationID: "c"}})
	if rec := post(h, `{"message":"hola"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("sin sesión: %d", rec.Code)
	}
	var last int
	for i := 0; i < perMinute+1; i++ {
		last = post(h, `{"message":"hola"}`, "u1").Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("la petición %d debe ser 429: %d", perMinute+1, last)
	}
}

func TestNewModuleRequiresDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
}
