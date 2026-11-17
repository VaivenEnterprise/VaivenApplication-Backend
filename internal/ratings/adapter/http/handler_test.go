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
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/usecase"
)

const (
	bookingID = "6731a0000000000000000001"
	ratingID  = "6731b0000000000000000010"
	rater     = "6730f1c2a9b3e4d5f6a7b8c2"
	ratee     = "6730f1c2a9b3e4d5f6a7b8c1"
)

var at = time.Date(2026, 11, 21, 16, 0, 0, 0, time.UTC)

type fakeService struct {
	err                      error
	view                     usecase.RatingView
	status                   usecase.StatusView
	page                     usecase.Page
	op, user, target, cursor string
	score                    int
	comment                  *string
	limit                    int
}

func (f *fakeService) Rate(_ context.Context, u, b string, score int, c *string) (usecase.RatingView, error) {
	f.op, f.user, f.target, f.score, f.comment = "rate", u, b, score, c
	return f.view, f.err
}
func (f *fakeService) Status(_ context.Context, u, b string) (usecase.StatusView, error) {
	f.op, f.user, f.target = "status", u, b
	return f.status, f.err
}
func (f *fakeService) List(_ context.Context, r, after string, limit int) (usecase.Page, error) {
	f.op, f.target, f.cursor, f.limit = "list", r, after, limit
	return f.page, f.err
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
	if m.Name() != "ratings" {
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

// errCode devuelve error.code del envelope de error (Art. IV).
func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func sampleView() usecase.RatingView {
	c := "Todo bien"
	return usecase.RatingView{
		Rating:    domain.Rating{ID: ratingID, BookingID: bookingID, RaterID: rater, RateeID: ratee, RatedRole: bookings.RoleOwner, Score: 5, Comment: &c, CreatedAt: at},
		RaterName: "Ana",
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
		{"POST", "/bookings/" + bookingID + "/ratings"}, {"GET", "/bookings/" + bookingID + "/rating-status"}, {"GET", "/users/" + ratee + "/ratings"},
	} {
		if code, _ := e.do(t, p[0], p[1], `{"score":5}`, ""); code != 401 {
			t.Errorf("%s %s = %d, quiero 401", p[0], p[1], code)
		}
	}
	if e.svc.op != "" {
		t.Error("sin sesión no se llama al caso de uso")
	}
}

func TestRateCreated(t *testing.T) { // CA-01
	e := newTenv(t)
	e.svc.view = sampleView()
	code, body := e.do(t, "POST", "/bookings/"+bookingID+"/ratings", `{"score":5,"comment":"Todo bien"}`, rater)
	if code != 201 {
		t.Fatalf("code = %d %v", code, body)
	}
	if e.svc.user != rater || e.svc.target != bookingID || e.svc.score != 5 || e.svc.comment == nil || *e.svc.comment != "Todo bien" {
		t.Errorf("llamada = %+v", e.svc)
	}
	rr, _ := body["rater"].(map[string]any)
	if body["id"] != ratingID || body["ratedRole"] != "owner" || body["score"] != float64(5) || body["createdAt"] != "2026-11-21T16:00:00Z" ||
		rr["displayName"] != "Ana" || rr["id"] != rater {
		t.Errorf("body = %v", body)
	}
	for _, k := range []string{"rateeId", "raterId"} {
		if _, has := body[k]; has {
			t.Errorf("no debe exponer %s", k)
		}
	}
}

func TestRateRejectsBadBodies(t *testing.T) {
	cases := map[string]string{
		"sin score":     `{}`,
		"score texto":   `{"score":"cinco"}`,
		"score decimal": `{"score":4.5}`,
		"no es JSON":    `no`,
		"dos objetos":   `{"score":5}{"score":4}`,
	}
	for name, body := range cases {
		e := newTenv(t)
		code, out := e.do(t, "POST", "/bookings/"+bookingID+"/ratings", body, rater)
		if code != 400 || errCode(out) != "validation_failed" {
			t.Errorf("%s: %d %v", name, code, out)
		}
		if e.svc.op != "" {
			t.Errorf("%s: no debe llegar al caso de uso", name)
		}
	}
}

func TestRateDomainErrorsMapToStatus(t *testing.T) { // CA-02, CA-03
	cases := []struct {
		err  error
		code int
	}{
		{domain.ErrNotCompleted, 409}, {domain.ErrWindowClosed, 409}, {domain.ErrAlreadyRated, 409},
		{bookings.ErrNotFound, 404},
		{shared.NewValidation("Revisa los datos marcados.", shared.Detail{Field: "score", Issue: "x"}), 400},
	}
	for _, c := range cases {
		e := newTenv(t)
		e.svc.err = c.err
		if code, _ := e.do(t, "POST", "/bookings/"+bookingID+"/ratings", `{"score":3}`, rater); code != c.code {
			t.Errorf("%v → %d, quiero %d", c.err, code, c.code)
		}
	}
	e := newTenv(t)
	e.svc.err = errors.New("mongo caído")
	code, out := e.do(t, "POST", "/bookings/"+bookingID+"/ratings", `{"score":3}`, rater)
	if code != 500 || errCode(out) != "internal_error" || !strings.Contains(e.logs.String(), "ratings: error interno") {
		t.Errorf("500: %d %v logs=%s", code, out, e.logs.String())
	}
}

func TestStatus(t *testing.T) {
	e := newTenv(t)
	exp := at.Add(14 * 24 * time.Hour)
	v := sampleView()
	e.svc.status = usecase.StatusView{Status: domain.Status{CanRate: false, MyRating: &v.Rating, ExpiresAt: &exp}, MyRaterName: "Ana"}
	code, body := e.do(t, "GET", "/bookings/"+bookingID+"/rating-status", "", rater)
	mine, _ := body["myRating"].(map[string]any)
	if code != 200 || body["canRate"] != false || body["expiresAt"] != "2026-12-05T16:00:00Z" || mine["id"] != ratingID {
		t.Errorf("%d %v", code, body)
	}
	e.svc.status = usecase.StatusView{Status: domain.Status{CanRate: true}}
	_, body = e.do(t, "GET", "/bookings/"+bookingID+"/rating-status", "", rater)
	if body["canRate"] != true || body["myRating"] != nil || body["expiresAt"] != nil {
		t.Errorf("sin calificar: %v", body)
	}
	if _, has := body["myRating"]; !has {
		t.Error("myRating debe venir como null, no omitirse")
	}
	e.svc.err = bookings.ErrNotFound
	if code, _ := e.do(t, "GET", "/bookings/"+bookingID+"/rating-status", "", "6730f1c2a9b3e4d5f6a7b8c9"); code != 404 {
		t.Errorf("ajeno = %d", code)
	}
}

func TestListPagination(t *testing.T) {
	e := newTenv(t)
	next := ratingID
	e.svc.page = usecase.Page{Items: []usecase.RatingView{sampleView()}, Next: &next}
	code, body := e.do(t, "GET", "/users/"+ratee+"/ratings?limit=10", "", rater)
	items, _ := body["items"].([]any)
	if code != 200 || len(items) != 1 || e.svc.target != ratee || e.svc.limit != 10 || e.svc.cursor != "" {
		t.Fatalf("%d %v %+v", code, body, e.svc)
	}
	cur, _ := body["nextCursor"].(string)
	if cur != base64.RawURLEncoding.EncodeToString([]byte(ratingID)) {
		t.Errorf("nextCursor = %q", cur)
	}
	if code, _ = e.do(t, "GET", "/users/"+ratee+"/ratings?cursor="+cur, "", rater); code != 200 || e.svc.cursor != ratingID {
		t.Errorf("segunda página: %d cursor=%q", code, e.svc.cursor)
	}
	e.svc.page = usecase.Page{}
	_, body = e.do(t, "GET", "/users/"+ratee+"/ratings", "", rater)
	if items, ok := body["items"].([]any); !ok || len(items) != 0 || body["nextCursor"] != nil {
		t.Errorf("vacía: %v", body)
	}
	for _, q := range []string{"limit=0", "limit=51", "limit=x", "cursor=!!!", "cursor=" + base64.RawURLEncoding.EncodeToString([]byte("no-id")), "cursor=" + strings.Repeat("a", 70)} {
		if code, out := e.do(t, "GET", "/users/"+ratee+"/ratings?"+q, "", rater); code != 400 || errCode(out) != "validation_failed" {
			t.Errorf("%s → %d %v", q, code, out)
		}
	}
	e.svc.err = domain.ErrUserNotFound
	if code, _ := e.do(t, "GET", "/users/"+ratee+"/ratings", "", rater); code != 404 {
		t.Errorf("usuario inexistente = %d", code)
	}
}
