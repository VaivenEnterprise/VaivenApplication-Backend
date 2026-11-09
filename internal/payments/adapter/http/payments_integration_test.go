//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	bookingshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/http"
	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	bookingsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	handoverhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/adapter/http"
	handovermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/adapter/mongo"
	handoverusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/usecase"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	paymentsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

var testClient *mongodb.Client

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "no se pudo iniciar MongoDB con Docker o Podman:", err)
		os.Exit(1)
	}
	uri, err := c.ConnectionString(ctx)
	if err == nil {
		testClient, err = mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "MongoDB:", err)
		_ = c.Terminate(ctx)
		os.Exit(1)
	}
	code := m.Run()
	_ = testClient.Disconnect(ctx)
	_ = c.Terminate(ctx)
	os.Exit(code)
}

type itImages struct{}

func (itImages) Signature(context.Context, string, images.Purpose) (images.UploadSignature, error) {
	return images.UploadSignature{}, nil
}
func (itImages) Resolve(_ context.Context, _ string, _ images.Purpose, _ string, refs []images.Ref) ([]images.Image, error) {
	out := make([]images.Image, 0, len(refs))
	for _, r := range refs {
		out = append(out, images.Image{PublicID: r.PublicID, Format: "jpg", Delivery: images.DeliveryAuthenticated})
	}
	return out, nil
}
func (itImages) URLs(i images.Image) (images.URLs, error) {
	return images.URLs{URL: "https://img/" + i.PublicID, ThumbURL: "https://thumb/" + i.PublicID}, nil
}

type itUsers struct{}

func (itUsers) PublicUsers(_ context.Context, ids []string) (map[string]users.PublicUser, error) {
	out := map[string]users.PublicUser{}
	for _, id := range ids {
		out[id] = users.PublicUser{ID: id, DisplayName: "Persona " + id}
	}
	return out, nil
}

type itNotifier struct {
	mu   sync.Mutex
	sent []string // "tipo:destinatario"
}

func (n *itNotifier) Notify(_ context.Context, to string, e notify.Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, e.Type+":"+to)
	return nil
}

func (n *itNotifier) count(s string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := 0
	for _, x := range n.sent {
		if x == s {
			c++
		}
	}
	return c
}

func itAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-User")
		if id == "" {
			respond.Error(w, r, shared.NewUnauthorized("sin sesión"))
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), auth.Identity{UserID: id})))
	})
}

type flow struct {
	t     *testing.T
	h     http.Handler
	repo  *bookingsmongo.Repo
	svc   *usecase.Service
	clock *time.Time
	not   *itNotifier
	loc   *time.Location
}

func newFlow(t *testing.T) *flow {
	t.Helper()
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("payments_flow_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := bookingsmongo.NewRepo(db)
	payRepo := paymentsmongo.NewRepo(db)
	for _, e := range []interface{ EnsureIndexes(context.Context) error }{repo, payRepo} {
		if err := e.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
	}
	loc, err := bookings.Location()
	if err != nil {
		t.Fatal(err)
	}
	f := &flow{t: t, repo: repo, not: &itNotifier{}, loc: loc}
	now := time.Now().UTC().Truncate(time.Millisecond)
	f.clock = &now
	clock := func() time.Time { return *f.clock }
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	bookSvc, err := bookingsusecase.NewService(bookingsusecase.Deps{
		Repo: repo, Images: itImages{}, Users: itUsers{}, Chat: chat.Noop{}, Notifier: f.not, Log: quiet, Now: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	bookMod, err := bookingshttp.NewModule(bookingshttp.ModuleDeps{
		Service: bookSvc, Authenticate: itAuth, RequireActive: func(h http.Handler) http.Handler { return h }, Log: quiet,
	})
	if err != nil {
		t.Fatal(err)
	}
	late := domain.NewLatenessIn(loc)
	hoSvc, err := handoverusecase.NewService(handoverusecase.Deps{
		Repo: handovermongo.NewStageRepo(db), Bookings: repo, Images: itImages{}, Notifier: f.not, Log: quiet,
		Lateness: late, Now: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	hoMod, err := handoverhttp.NewModule(handoverhttp.ModuleDeps{Service: hoSvc, Authenticate: itAuth, WriteBooking: bookMod.WriteBooking, Log: quiet})
	if err != nil {
		t.Fatal(err)
	}
	f.svc, err = usecase.NewService(usecase.Deps{Repo: payRepo, Bookings: repo, Notifier: f.not, Log: quiet, Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	mod, err := NewModule(ModuleDeps{Service: f.svc, Authenticate: itAuth, WriteBooking: bookMod.WriteBooking, Log: quiet})
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	bookMod.Register(r)
	hoMod.Register(r)
	mod.Register(r)
	f.h = r
	return f
}

func (f *flow) call(user, method, path, body string) (int, map[string]any) {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-User", user)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (f *flow) expect(want int, user, method, path, body string) map[string]any {
	f.t.Helper()
	code, out := f.call(user, method, path, body)
	if code != want {
		f.t.Fatalf("%s %s como %q = %d, quiero %d: %v", method, path, user, code, want, out)
	}
	return out
}

// seed guarda una reserva de renta de $30/día con el estado y el periodo dados.
func (f *flow) seed(item string, status bookings.Status, start, end bookings.Date, mutate func(*bookings.Booking)) string {
	f.t.Helper()
	days := 1
	for d := start; d.Before(end); d = d.AddDays(1) {
		days++
	}
	b := bookings.Booking{
		ID: f.repo.NewID(), ItemID: item, OwnerID: "own", RequesterID: "req",
		Start: start, End: end, Days: days,
		Item:        bookings.ItemSnapshot{Title: "Taladro", Modality: bookings.ModalityRent},
		Price:       bookings.PriceSnapshot{Modality: bookings.ModalityRent, PricePerDayCents: 3000, TotalCents: int64(days) * 3000, Currency: "MXN"},
		PaymentMode: bookings.PaymentFullOnDelivery, Status: status,
		Payments:  []bookings.BookingPayment{{Stage: bookings.StageDelivery, AmountCents: int64(days) * 3000, Status: bookings.PaymentPending}},
		History:   []bookings.HistoryEntry{{Status: status, At: *f.clock, By: bookings.RoleOwner}},
		CreatedAt: *f.clock, UpdatedAt: *f.clock, Version: 1,
	}
	if mutate != nil {
		mutate(&b)
	}
	if err := f.repo.Create(context.Background(), b); err != nil {
		f.t.Fatal(err)
	}
	return b.ID
}

// inProgress es una reserva en curso con la entrega ya confirmada.
// lateAt se fija como al confirmar la entrega (domain.Lateness.LateAt).
func (f *flow) inProgress(item string, start, end bookings.Date) string {
	return f.seed(item, bookings.StatusInProgress, start, end, func(b *bookings.Booking) {
		la := domain.NewLatenessIn(f.loc).LateAt(*b)
		b.LateAt = &la
		at, own, req := f.clock.Add(-48*time.Hour), "own", "req"
		b.Delivery = &bookings.Handover{
			RecordedAt: &at, RecordedByID: &own, ConditionNotes: "Entregado en buen estado",
			EvidenceVersion: 1, ConfirmedAt: &at, ConfirmedByID: &req,
		}
	})
}

func (f *flow) today() bookings.Date { return bookings.Today(*f.clock, f.loc) }

func TestIntegrationPaymentFlow(t *testing.T) { // CA-06
	f := newFlow(t)
	today := f.today()
	id := f.seed("item1", bookings.StatusAccepted, today, today.AddDays(1), nil)
	p := "/bookings/" + id + "/payments/delivery"

	f.expect(404, "otra", "POST", p+"/report", `{}`)                              // no es parte
	f.expect(404, "req", "POST", "/bookings/"+id+"/payments/return/report", `{}`) // no existe esa etapa
	f.expect(409, "own", "POST", p+"/confirm", "")                                // CA-06: sin reporte previo
	f.expect(403, "own", "POST", p+"/report", `{}`)                               // el propietario no reporta

	b := f.expect(200, "req", "POST", p+"/report", `{"note":"SPEI 123"}`)
	pay := b["payments"].([]any)[0].(map[string]any)
	if pay["status"] != "reported" || b["status"] != "accepted" {
		t.Fatalf("Booking = %v", b)
	}
	f.expect(409, "req", "POST", p+"/report", `{}`) // ya reportado
	f.expect(403, "req", "POST", p+"/confirm", "")  // el solicitante no confirma

	b = f.expect(200, "own", "POST", p+"/confirm", "")
	if b["payments"].([]any)[0].(map[string]any)["status"] != "confirmed" {
		t.Fatalf("CA-06, Booking = %v", b)
	}
	if b["status"] != "accepted" {
		t.Errorf("el pago no cambia el estado de la reserva: %v", b["status"])
	}
	f.expect(409, "own", "POST", p+"/confirm", "")
	if f.not.count("payment_reported:own") != 1 || f.not.count("payment_confirmed:req") != 1 {
		t.Errorf("avisos = %v", f.not.sent)
	}

	// El QR no depende del pago: la entrega se confirma con el pago ya confirmado
	// y también se habría podido confirmar sin él.
	base := "/bookings/" + id
	f.expect(200, "own", "PUT", base+"/delivery/evidence", `{"conditionNotes":"Limpio y completo","photos":[{"publicId":"ev/1"}]}`)
	tok := f.expect(201, "own", "POST", base+"/delivery/qr", "")["token"].(string)
	got := f.expect(200, "req", "POST", base+"/delivery/confirm", fmt.Sprintf(`{"token":%q}`, tok))
	if got["status"] != "in_progress" {
		t.Errorf("status = %v", got["status"])
	}
}

func TestIntegrationPaymentRules(t *testing.T) {
	f := newFlow(t)
	today := f.today()
	// Pendiente, rechazada o cancelada: no admite pagos.
	for _, st := range []bookings.Status{bookings.StatusPending, bookings.StatusRejected, bookings.StatusCancelled} {
		id := f.seed("i", st, today, today, nil)
		f.expect(409, "req", "POST", "/bookings/"+id+"/payments/delivery/report", `{}`)
	}
	// Préstamo: sin pagos.
	loan := f.seed("i", bookings.StatusAccepted, today, today, func(b *bookings.Booking) {
		b.Item.Modality, b.Price.Modality, b.PaymentMode, b.Payments = bookings.ModalityLoan, bookings.ModalityLoan, bookings.PaymentNone, nil
	})
	f.expect(404, "req", "POST", "/bookings/"+loan+"/payments/delivery/report", `{}`)
	// Nota demasiado larga.
	ok := f.seed("i", bookings.StatusAccepted, today, today, nil)
	f.expect(400, "req", "POST", "/bookings/"+ok+"/payments/delivery/report", fmt.Sprintf(`{"note":%q}`, strings.Repeat("x", 201)))
	// Sin sesión.
	f.expect(401, "", "POST", "/bookings/"+ok+"/payments/delivery/report", `{}`)
	f.expect(401, "", "POST", "/bookings/"+ok+"/payments/delivery/confirm", "")
}

// Dos reportes a la vez: exactamente uno se aplica (plan P-08).
func TestIntegrationConcurrentReportsOnlyOneWins(t *testing.T) {
	f := newFlow(t)
	today := f.today()
	id := f.seed("i", bookings.StatusAccepted, today, today, nil)
	const n = 10
	codes := make([]int, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			codes[i], _ = f.call("req", "POST", "/bookings/"+id+"/payments/delivery/report", `{}`)
		}()
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, c := range codes {
		switch c {
		case 200:
			wins++
		case 409:
		default:
			t.Errorf("código inesperado %d", c)
		}
	}
	if wins != 1 {
		t.Fatalf("200 = %d, quiero 1", wins)
	}
	if got := f.not.count("payment_reported:own"); got != 1 {
		t.Errorf("avisos = %d, quiero 1", got)
	}
}

func TestIntegrationLateReturnAdjustment(t *testing.T) { // CA-07
	f := newFlow(t)
	today := f.today()
	id := f.inProgress("item1", today.AddDays(-2), today)
	base := "/bookings/" + id

	// La devolución se registra 1 día 3 h después del fin de la reserva (27 h
	// después de que termina el último día en México): 2 días × 3000.
	lateness := domain.NewLatenessIn(f.loc)
	b, err := f.repo.ByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	*f.clock = lateness.EndOfDay(b).Add(27 * time.Hour).Truncate(time.Millisecond)

	f.expect(200, "req", "PUT", base+"/return/evidence", `{"conditionNotes":"Devuelto con retraso","photos":[{"publicId":"ret/1"}]}`)
	tok := f.expect(201, "req", "POST", base+"/return/qr", "")["token"].(string)
	got := f.expect(200, "own", "POST", base+"/return/confirm", fmt.Sprintf(`{"token":%q}`, tok))
	if got["status"] != "completed" {
		t.Fatalf("status = %v", got["status"])
	}
	adj, _ := got["adjustments"].([]any)
	if len(adj) != 1 {
		t.Fatalf("adjustments = %v", got["adjustments"])
	}
	a := adj[0].(map[string]any)
	if a["type"] != "late_return" || a["lateDays"] != float64(2) || a["amountCents"] != float64(6000) || a["status"] != "recorded" {
		t.Errorf("CA-07, ajuste = %v", a)
	}
	// Se registra, no se cobra: no aparece ningún pago nuevo.
	if n := len(got["payments"].([]any)); n != 1 {
		t.Errorf("payments = %d, quiero 1", n)
	}
}

func TestIntegrationOnTimeReturnHasNoAdjustment(t *testing.T) {
	f := newFlow(t)
	today := f.today()
	id := f.inProgress("item1", today.AddDays(-1), today)
	base := "/bookings/" + id
	b, _ := f.repo.ByID(context.Background(), id)
	*f.clock = domain.NewLatenessIn(f.loc).EndOfDay(b).Add(time.Hour) // dentro de la tolerancia de 2 h
	f.expect(200, "req", "PUT", base+"/return/evidence", `{"conditionNotes":"Devuelto a tiempo","photos":[{"publicId":"ret/1"}]}`)
	tok := f.expect(201, "req", "POST", base+"/return/qr", "")["token"].(string)
	got := f.expect(200, "own", "POST", base+"/return/confirm", fmt.Sprintf(`{"token":%q}`, tok))
	if adj, _ := got["adjustments"].([]any); len(adj) != 0 {
		t.Errorf("adjustments = %v, quiero ninguno", adj)
	}
}

func TestIntegrationDetectorMarksLateAndAffected(t *testing.T) { // CA-08
	f := newFlow(t)
	ctx := context.Background()
	today := f.today()
	// Reserva en curso que debía devolverse anteayer: su lateAt ya pasó a cualquier hora del día.
	late := f.inProgress("item1", today.AddDays(-4), today.AddDays(-2))
	// Otra aceptada del mismo objeto que empieza hoy (afectada) y otras que no.
	affected := f.seed("item1", bookings.StatusAccepted, today, today.AddDays(1), nil)
	future := f.seed("item1", bookings.StatusAccepted, today.AddDays(5), today.AddDays(6), nil) // aún no empieza
	otherItem := f.seed("item2", bookings.StatusAccepted, today, today.AddDays(1), nil)
	onTime := f.inProgress("item3", today.AddDays(-1), today.AddDays(5)) // sin retraso

	n, err := f.svc.DetectLate(ctx)
	if err != nil || n != 1 {
		t.Fatalf("DetectLate = %d, %v; quiero 1", n, err)
	}
	get := func(id, user string) map[string]any { return f.expect(200, user, "GET", "/bookings/"+id, "") }
	if got := get(late, "own"); got["isLate"] != true {
		t.Errorf("la tardía debe quedar isLate: %v", got["isLate"])
	}
	if got := get(affected, "req"); got["affectedBy"] != late {
		t.Errorf("CA-08, affectedBy = %v", got["affectedBy"])
	}
	for name, id := range map[string]string{"futura": future, "otro objeto": otherItem, "en curso a tiempo": onTime} {
		if got := get(id, "req"); got["affectedBy"] != nil || got["isLate"] == true {
			t.Errorf("%s no debe marcarse: %v", name, got)
		}
	}
	if f.not.count("booking_late:own") != 1 || f.not.count("booking_late:req") != 1 || f.not.count("booking_affected:req") != 1 {
		t.Errorf("avisos = %v", f.not.sent)
	}

	// Idempotente: otra pasada no marca ni avisa de nuevo.
	if n, err := f.svc.DetectLate(ctx); err != nil || n != 0 {
		t.Fatalf("segunda pasada = %d, %v; quiero 0", n, err)
	}
	if f.not.count("booking_affected:req") != 1 || f.not.count("booking_late:own") != 1 {
		t.Errorf("la segunda pasada repitió avisos: %v", f.not.sent)
	}
}
