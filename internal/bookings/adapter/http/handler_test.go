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
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

const bid = "6731a2b3c4d5e6f7a8b9c0d1"

type fakeService struct {
	err      error
	view     usecase.BookingView
	page     usecase.Page
	viewer   usecase.Viewer
	op, id   string
	quoteIn  usecase.QuoteInput
	createIn usecase.CreateInput
	listIn   usecase.ListQuery
	text     *string
}

func (f *fakeService) rec(v usecase.Viewer, op, id string) { f.viewer, f.op, f.id = v, op, id }

func (f *fakeService) Quote(_ context.Context, v usecase.Viewer, in usecase.QuoteInput) (domain.Quote, error) {
	f.rec(v, "quote", in.ItemID)
	f.quoteIn = in
	return domain.Quote{ItemID: in.ItemID, Start: domain.MustDate(in.Start), End: domain.MustDate(in.End), Days: 3,
		Modality: domain.ModalityRent, PricePerDayCents: 3000, TotalCents: 9000, PaymentMode: in.PaymentMode,
		Payments: domain.SplitPayments(9000, in.PaymentMode)}, f.err
}
func (f *fakeService) Create(_ context.Context, v usecase.Viewer, in usecase.CreateInput) (usecase.BookingView, error) {
	f.rec(v, "create", "")
	f.createIn = in
	return f.view, f.err
}
func (f *fakeService) Get(_ context.Context, v usecase.Viewer, id string) (usecase.BookingView, error) {
	f.rec(v, "get", id)
	return f.view, f.err
}
func (f *fakeService) List(_ context.Context, v usecase.Viewer, q usecase.ListQuery) (usecase.Page, error) {
	f.rec(v, "list", "")
	f.listIn = q
	return f.page, f.err
}
func (f *fakeService) Accept(_ context.Context, v usecase.Viewer, id string, mp *string) (usecase.BookingView, error) {
	f.rec(v, "accept", id)
	f.text = mp
	return f.view, f.err
}
func (f *fakeService) Reject(_ context.Context, v usecase.Viewer, id string, r *string) (usecase.BookingView, error) {
	f.rec(v, "reject", id)
	f.text = r
	return f.view, f.err
}
func (f *fakeService) Cancel(_ context.Context, v usecase.Viewer, id string, r *string) (usecase.BookingView, error) {
	f.rec(v, "cancel", id)
	f.text = r
	return f.view, f.err
}
func (f *fakeService) SetMeetingPoint(_ context.Context, v usecase.Viewer, id, d string) (usecase.BookingView, error) {
	f.rec(v, "meeting", id)
	f.text = &d
	return f.view, f.err
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

func testActive(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Suspended") != "" {
			respond.Error(w, r, shared.NewForbidden("suspendida"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sampleView() usecase.BookingView {
	at := time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC)
	exp, thumb, mp, reason := at.Add(48*time.Hour), "https://t/p1", "Metro", "No"
	by := domain.RoleOwner
	return usecase.BookingView{
		Booking: domain.Booking{
			ID: bid, ItemID: "item1", OwnerID: "own", RequesterID: "req",
			Start: domain.MustDate("2026-11-10"), End: domain.MustDate("2026-11-12"), Days: 3,
			Item:        domain.ItemSnapshot{Title: "Taladro", Modality: domain.ModalityRent},
			Price:       domain.PriceSnapshot{Modality: domain.ModalityRent, PricePerDayCents: 3000, TotalCents: 9000, Currency: "MXN"},
			PaymentMode: domain.PaymentSplit5050,
			Payments:    []domain.BookingPayment{{Stage: domain.StageDelivery, AmountCents: 4500, Status: domain.PaymentPending}},
			Status:      domain.StatusPending, ExpiresAt: &exp, MeetingPoint: &mp, CancelReason: &reason, CancelledBy: &by,
			CreatedAt: at, UpdatedAt: at, Version: 1,
		},
		Owner:      users.PublicUser{ID: "own", DisplayName: "Ana", MemberSince: at},
		Requester:  users.PublicUser{ID: "req", DisplayName: "Beto", MemberSince: at},
		ThumbURL:   &thumb,
		ViewerRole: domain.RoleRequester, ShowMeetingPoint: true,
	}
}

type tenv struct {
	svc  *fakeService
	h    http.Handler
	logs *bytes.Buffer
}

func newTenv(t *testing.T) tenv {
	t.Helper()
	svc := &fakeService{view: sampleView()}
	logs := &bytes.Buffer{}
	m, err := NewModule(ModuleDeps{Service: svc, Authenticate: testAuth, RequireActive: testActive, Log: slog.New(slog.NewJSONHandler(logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "bookings" {
		t.Error("Name")
	}
	r := chi.NewRouter()
	m.Register(r)
	return tenv{svc: svc, h: r, logs: logs}
}

func (e tenv) do(t *testing.T, method, path, body string, hdr ...string) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("X-User", "req")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func fields(out map[string]any) []string {
	e, _ := out["error"].(map[string]any)
	ds, _ := e["details"].([]any)
	var f []string
	for _, d := range ds {
		f = append(f, d.(map[string]any)["field"].(string))
	}
	return f
}

func TestNewModuleDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("sin dependencias")
	}
}

func TestQuote(t *testing.T) {
	e := newTenv(t)
	code, out := e.do(t, "GET", "/items/item1/quote?startDate=2026-11-10&endDate=2026-11-12&paymentMode=split_50_50", "")
	if code != 200 || out["totalCents"] != float64(9000) || out["currency"] != "MXN" || len(out["payments"].([]any)) != 2 {
		t.Fatalf("quote = %d %v", code, out)
	}
	pol := out["policy"].(map[string]any)
	if pol["requestExpiresInHours"] != float64(48) || pol["lateCancellationNotice"] == "" {
		t.Errorf("policy = %v", pol)
	}
	if e.svc.quoteIn.ItemID != "item1" || e.svc.quoteIn.PaymentMode != domain.PaymentSplit5050 {
		t.Errorf("entrada = %+v", e.svc.quoteIn)
	}
	code, out = e.do(t, "GET", "/items/item1/quote", "")
	if code != 400 || len(fields(out)) != 3 {
		t.Errorf("sin parámetros = %d %v", code, out)
	}
	e.svc.err = domain.ErrOwnItem
	if code, _ := e.do(t, "GET", "/items/item1/quote?startDate=a&endDate=b&paymentMode=c", ""); code != 403 {
		t.Errorf("propio = %d", code)
	}
}

func TestCreate(t *testing.T) {
	e := newTenv(t)
	body := `{"itemId":"item1","startDate":"2026-11-10","endDate":"2026-11-12","paymentMode":"split_50_50","acceptedTotalCents":9000}`
	code, out := e.do(t, "POST", "/bookings", body)
	if code != 201 || out["id"] != bid || out["status"] != "pending" || out["viewerRole"] != "requester" {
		t.Fatalf("create = %d %v", code, out)
	}
	if e.svc.createIn.AcceptedTotalCents != 9000 || e.svc.createIn.Start != "2026-11-10" {
		t.Errorf("entrada = %+v", e.svc.createIn)
	}
	// Campos fijos del contrato (009 los completa).
	for _, k := range []string{"delivery", "return"} {
		if v, ok := out[k]; !ok || v != nil {
			t.Errorf("%s = %v", k, v)
		}
	}
	if out["isLate"] != false || len(out["adjustments"].([]any)) != 0 || out["item"].(map[string]any)["thumbUrl"] != "https://t/p1" {
		t.Errorf("contrato = %v", out)
	}
	if mp := out["meetingPoint"].(map[string]any); mp["description"] != "Metro" {
		t.Errorf("meetingPoint = %v", mp)
	}
	if code, out := e.do(t, "POST", "/bookings", `{}`); code != 400 || len(fields(out)) != 5 {
		t.Errorf("vacío = %d %v", code, fields(out))
	}
	if code, out := e.do(t, "POST", "/bookings", strings.Replace(body, "9000", "-1", 1)); code != 400 || fields(out)[0] != "acceptedTotalCents" {
		t.Errorf("negativo = %d", code)
	}
	for _, bad := range []string{`no`, `{} {}`, `{"acceptedTotalCents":"x"}`} {
		if code, _ := e.do(t, "POST", "/bookings", bad); code != 400 {
			t.Errorf("%s = %d", bad, code)
		}
	}
	if code, _ := e.do(t, "POST", "/bookings", body, "X-Suspended", "1"); code != 403 {
		t.Errorf("suspendida = %d", code)
	}
	e.svc.err = domain.ErrQuoteChanged
	code, out = e.do(t, "POST", "/bookings", body)
	if code != 409 || !strings.Contains(fields(out)[0], "acceptedTotalCents") {
		t.Errorf("quote_changed = %d %v", code, out)
	}
}

func TestGetHidesMeetingPointAndList(t *testing.T) {
	e := newTenv(t)
	e.svc.view.ShowMeetingPoint = false
	e.svc.view.ViewerRole = domain.RoleAdmin
	code, out := e.do(t, "GET", "/bookings/"+bid, "", "X-Role", "admin")
	if code != 200 || out["meetingPoint"] != nil || out["viewerRole"] != "admin" || !e.svc.viewer.Admin {
		t.Errorf("admin = %d %v", code, out["meetingPoint"])
	}
	next := &domain.Cursor{CreatedAt: time.Date(2026, 11, 8, 1, 2, 3, 4_000_000, time.UTC), ID: bid}
	e.svc.page = usecase.Page{Items: []usecase.BookingView{sampleView()}, Next: next}
	code, out = e.do(t, "GET", "/me/bookings?role=owner&status=pending&limit=5", "")
	if code != 200 || len(out["items"].([]any)) != 1 || e.svc.listIn.Role != domain.RoleOwner || *e.svc.listIn.Status != domain.StatusPending || e.svc.listIn.Limit != 5 {
		t.Fatalf("list = %d %+v", code, e.svc.listIn)
	}
	_, _ = e.do(t, "GET", "/me/bookings?role=owner&cursor="+out["nextCursor"].(string), "")
	if a := e.svc.listIn.After; a == nil || a.ID != bid || !a.CreatedAt.Equal(next.CreatedAt) {
		t.Errorf("cursor = %+v", a)
	}
	e.svc.page = usecase.Page{}
	if _, out := e.do(t, "GET", "/me/bookings?role=owner", ""); out["nextCursor"] != nil || len(out["items"].([]any)) != 0 {
		t.Errorf("vacío = %v", out)
	}
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for _, q := range []string{"limit=0", "limit=x", "cursor=***", "cursor=" + enc("1.zz"), "cursor=" + enc("x."+bid), "cursor=" + strings.Repeat("a", 129)} {
		if code, _ := e.do(t, "GET", "/me/bookings?role=owner&"+q, ""); code != 400 {
			t.Errorf("%s = %d", q, code)
		}
	}
	e.svc.err = shared.NewValidation("x")
	if code, _ := e.do(t, "GET", "/me/bookings?role=x", ""); code != 400 {
		t.Error("rol inválido")
	}
}

func TestActions(t *testing.T) {
	e := newTenv(t)
	cases := []struct {
		path, body, op string
		want           *string
	}{
		{"/accept", "", "accept", nil},
		{"/accept", `{"meetingPoint":{"description":"Metro"}}`, "accept", ptr("Metro")},
		{"/reject", `{"reason":"No"}`, "reject", ptr("No")},
		{"/reject", "", "reject", nil},
		{"/cancel", `{"reason":"Cambio de planes"}`, "cancel", ptr("Cambio de planes")},
		{"/meeting-point", `{"description":"Parque"}`, "meeting", ptr("Parque")},
	}
	for _, c := range cases {
		code, _ := e.do(t, "POST", "/bookings/"+bid+c.path, c.body)
		if code != 200 || e.svc.op != c.op || e.svc.id != bid || (c.want == nil) != (e.svc.text == nil) || (c.want != nil && *c.want != *e.svc.text) {
			t.Errorf("%s %s = %d %s %v", c.path, c.body, code, e.svc.op, e.svc.text)
		}
	}
	bad := map[string]string{
		"/accept":        `{"meetingPoint":{}}`,
		"/reject":        `{"reason":5}`,
		"/cancel":        `nada`,
		"/meeting-point": `{}`,
	}
	for path, body := range bad {
		if code, _ := e.do(t, "POST", "/bookings/"+bid+path, body); code != 400 {
			t.Errorf("%s %s = %d", path, body, code)
		}
	}
	if code, _ := e.do(t, "POST", "/bookings/"+bid+"/meeting-point", ""); code != 400 {
		t.Error("meeting-point sin cuerpo")
	}
	if code, _ := e.do(t, "POST", "/bookings/"+bid+"/accept", `{"x":1} {}`); code != 400 {
		t.Error("dos objetos")
	}
	e.svc.err = errors.New("mongo con datos")
	if code, _ := e.do(t, "POST", "/bookings/"+bid+"/accept", ""); code != 500 || !strings.Contains(e.logs.String(), "bookings: error interno") {
		t.Error("error interno")
	}
	e.svc.err = domain.ErrPeriodTaken
	if code, _ := e.do(t, "POST", "/bookings/"+bid+"/accept", ""); code != 409 {
		t.Error("CA-05 → 409")
	}
	for _, p := range []string{"/bookings/" + bid, "/me/bookings?role=owner", "/items/x/quote?startDate=a&endDate=b&paymentMode=c"} {
		if code, _ := e.do(t, "GET", p, ""); code != 409 {
			t.Errorf("%s con error = %d", p, code)
		}
	}
	req := httptest.NewRequest("GET", "/bookings/"+bid, nil)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Error("sin sesión")
	}
}

func ptr(s string) *string { return &s }

func TestDecodeTooBig(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"x":"`+strings.Repeat("a", 100)+`"}`))
	r.Body = http.MaxBytesReader(httptest.NewRecorder(), r.Body, 10)
	var v map[string]any
	if err := decode(r, &v, false); err == nil || !strings.Contains(err.Error(), "demasiado grande") {
		t.Errorf("err = %v", err)
	}
}
