//go:build integration

package http

import (
	"bytes"
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
	handovermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/usecase"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
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
	return images.URLs{URL: "https://img/" + i.PublicID + "?sig=10min", ThumbURL: "https://thumb/" + i.PublicID}, nil
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
	clock *time.Time
	not   *itNotifier
	logs  *bytes.Buffer
}

func newFlow(t *testing.T) *flow {
	t.Helper()
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("handover_flow_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := bookingsmongo.NewRepo(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	f := &flow{t: t, repo: repo, not: &itNotifier{}, logs: &bytes.Buffer{}}
	now := time.Now().UTC().Truncate(time.Millisecond)
	f.clock = &now
	clock := func() time.Time { return *f.clock }
	log := slog.New(slog.NewTextHandler(f.logs, nil))

	bookSvc, err := bookingsusecase.NewService(bookingsusecase.Deps{
		Repo: repo, Images: itImages{}, Users: itUsers{}, Chat: chat.Noop{}, Notifier: f.not,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	bookMod, err := bookingshttp.NewModule(bookingshttp.ModuleDeps{
		Service: bookSvc, Authenticate: itAuth, RequireActive: func(h http.Handler) http.Handler { return h },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := usecase.NewService(usecase.Deps{
		Repo: handovermongo.NewStageRepo(db), Bookings: repo, Images: itImages{}, Notifier: f.not, Log: log, Now: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	mod, err := NewModule(ModuleDeps{Service: svc, Authenticate: itAuth, WriteBooking: bookMod.WriteBooking, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	bookMod.Register(r)
	mod.Register(r)
	f.h = r
	return f
}

// accepted siembra una reserva aceptada cuyo periodo incluye hoy (México).
func (f *flow) accepted() string {
	f.t.Helper()
	loc, _ := bookings.Location()
	today := bookings.Today(*f.clock, loc)
	b := bookings.Booking{
		ID: f.repo.NewID(), ItemID: "item1", OwnerID: "own", RequesterID: "req",
		Start: today, End: today.AddDays(1), Days: 2,
		Item:        bookings.ItemSnapshot{Title: "Taladro", Modality: bookings.ModalityRent},
		Price:       bookings.PriceSnapshot{Modality: bookings.ModalityRent, PricePerDayCents: 3000, TotalCents: 6000, Currency: "MXN"},
		PaymentMode: bookings.PaymentFullOnDelivery, Status: bookings.StatusAccepted,
		Payments:  []bookings.BookingPayment{{Stage: bookings.StageDelivery, AmountCents: 6000, Status: bookings.PaymentPending}},
		History:   []bookings.HistoryEntry{{Status: bookings.StatusAccepted, At: *f.clock, By: bookings.RoleOwner}},
		CreatedAt: *f.clock, UpdatedAt: *f.clock, Version: 1,
	}
	if err := f.repo.Create(context.Background(), b); err != nil {
		f.t.Fatal(err)
	}
	return b.ID
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

const evidenceBody = `{"conditionNotes":"Limpio, con estuche y dos brocas","photos":[{"publicId":"ev/1"},{"publicId":"ev/2"}]}`

func (f *flow) expect(want int, user, method, path, body string) map[string]any {
	f.t.Helper()
	code, out := f.call(user, method, path, body)
	if code != want {
		f.t.Fatalf("%s %s como %q = %d, quiero %d: %v", method, path, user, code, want, out)
	}
	return out
}

func TestIntegrationDeliveryAndReturnFlow(t *testing.T) { // CA-01, CA-05, CA-09
	f := newFlow(t)
	id := f.accepted()
	base := "/bookings/" + id

	// CA-09 y roles: una persona ajena recibe 404; el solicitante no registra la entrega.
	f.expect(404, "otra", "GET", base+"/delivery", "")
	f.expect(404, "otra", "PUT", base+"/delivery/evidence", evidenceBody)
	f.expect(403, "req", "PUT", base+"/delivery/evidence", evidenceBody)
	f.expect(409, "own", "POST", base+"/delivery/qr", "") // sin evidencia

	st := f.expect(200, "own", "PUT", base+"/delivery/evidence", evidenceBody)
	if st["evidenceVersion"] != float64(1) || len(st["photos"].([]any)) != 2 || st["qr"].(map[string]any)["state"] != "none" {
		t.Fatalf("Stage = %v", st)
	}
	q := f.expect(201, "own", "POST", base+"/delivery/qr", "")
	token := q["token"].(string)
	if q["qrPayload"] != fmt.Sprintf("vaiven://qr?b=%s&s=delivery&t=%s", id, token) {
		t.Errorf("qrPayload = %v", q["qrPayload"])
	}
	// Las dos partes ven la etapa con URL firmadas y el QR activo; sin token ni hash.
	for _, who := range []string{"own", "req"} {
		got := f.expect(200, who, "GET", base+"/delivery", "")
		photos := got["photos"].([]any)
		if len(photos) != 2 || !strings.Contains(photos[0].(map[string]any)["url"].(string), "sig=10min") ||
			got["qr"].(map[string]any)["state"] != "active" {
			t.Errorf("%s ve %v", who, got)
		}
	}
	raw, _ := json.Marshal(f.expect(200, "req", "GET", base+"/delivery", ""))
	if strings.Contains(string(raw), token) || strings.Contains(string(raw), "tokenHash") {
		t.Error("la etapa no puede exponer el token ni su hash")
	}

	f.expect(409, "req", "POST", base+"/delivery/confirm", `{"token":"otro"}`)
	f.expect(403, "own", "POST", base+"/delivery/confirm", fmt.Sprintf(`{"token":%q}`, token))
	b := f.expect(200, "req", "POST", base+"/delivery/confirm", fmt.Sprintf(`{"token":%q}`, token))
	if b["status"] != "in_progress" || b["delivery"].(map[string]any)["confirmedAt"] == nil || b["delivery"].(map[string]any)["qr"].(map[string]any)["state"] != "used" {
		t.Fatalf("CA-01, Booking = %v", b)
	}
	f.expect(409, "req", "POST", base+"/delivery/confirm", fmt.Sprintf(`{"token":%q}`, token)) // CA-03
	f.expect(409, "own", "PUT", base+"/delivery/evidence", evidenceBody)                       // RF-13

	// Devolución: roles invertidos.
	f.expect(403, "own", "PUT", base+"/return/evidence", evidenceBody)
	f.expect(200, "req", "PUT", base+"/return/evidence", `{"conditionNotes":"Devuelto igual","photos":[{"publicId":"ret/1"}]}`)
	rq := f.expect(201, "req", "POST", base+"/return/qr", "")
	f.expect(403, "req", "POST", base+"/return/confirm", fmt.Sprintf(`{"token":%q}`, rq["token"]))
	done := f.expect(200, "own", "POST", base+"/return/confirm", fmt.Sprintf(`{"token":%q}`, rq["token"]))
	if done["status"] != "completed" || done["return"].(map[string]any)["confirmedAt"] == nil {
		t.Fatalf("CA-05, Booking = %v", done)
	}

	// GET /bookings/{id} (007) muestra las dos etapas a las partes.
	got := f.expect(200, "own", "GET", base, "")
	if got["delivery"] == nil || got["return"] == nil || got["status"] != "completed" {
		t.Errorf("Booking = %v", got)
	}
	// RF-12: cada etapa confirmada avisó a la otra parte.
	if strings.Join(f.not.sent, ",") != "delivery_confirmed:own,return_confirmed:req" {
		t.Errorf("avisos = %v", f.not.sent)
	}
	if strings.Contains(f.logs.String(), token) || strings.Contains(f.logs.String(), fmt.Sprint(rq["token"])) {
		t.Error("RNF-01: el token no puede aparecer en los logs")
	}
}

func TestIntegrationQRExpiryRenewalAndEvidenceChange(t *testing.T) { // CA-02, CA-04
	f := newFlow(t)
	id := f.accepted()
	base := "/bookings/" + id
	f.expect(200, "own", "PUT", base+"/delivery/evidence", evidenceBody)
	old := f.expect(201, "own", "POST", base+"/delivery/qr", "")["token"].(string)

	*f.clock = f.clock.Add(16 * time.Minute) // CA-02
	f.expect(409, "req", "POST", base+"/delivery/confirm", fmt.Sprintf(`{"token":%q}`, old))
	if st := f.expect(200, "req", "GET", base+"/delivery", ""); st["qr"].(map[string]any)["state"] != "expired" {
		t.Errorf("estado del QR = %v", st["qr"])
	}
	renewed := f.expect(201, "own", "POST", base+"/delivery/qr", "")["token"].(string)

	// CA-04: cambiar la evidencia invalida el QR vigente.
	st := f.expect(200, "own", "PUT", base+"/delivery/evidence", `{"conditionNotes":"Corrección del estado","photos":[{"publicId":"ev/9"}]}`)
	if st["evidenceVersion"] != float64(2) || st["qr"].(map[string]any)["state"] != "none" {
		t.Fatalf("Stage = %v", st)
	}
	f.expect(409, "req", "POST", base+"/delivery/confirm", fmt.Sprintf(`{"token":%q}`, renewed))
	fresh := f.expect(201, "own", "POST", base+"/delivery/qr", "")["token"].(string)
	f.expect(200, "req", "POST", base+"/delivery/confirm", fmt.Sprintf(`{"token":%q}`, fresh))
}

func TestIntegrationConfirmWindowAndStatus(t *testing.T) {
	f := newFlow(t)
	// Reserva pendiente: no admite evidencia.
	loc, _ := bookings.Location()
	today := bookings.Today(*f.clock, loc)
	pend := bookings.Booking{
		ID: f.repo.NewID(), ItemID: "i", OwnerID: "own", RequesterID: "req", Start: today, End: today, Days: 1,
		Item: bookings.ItemSnapshot{Modality: bookings.ModalityLoan}, PaymentMode: bookings.PaymentNone,
		Status: bookings.StatusPending, CreatedAt: *f.clock, UpdatedAt: *f.clock, Version: 1,
	}
	if err := f.repo.Create(context.Background(), pend); err != nil {
		t.Fatal(err)
	}
	f.expect(409, "own", "PUT", "/bookings/"+pend.ID+"/delivery/evidence", evidenceBody)

	// Reserva aceptada en el futuro lejano: el QR existe pero aún no se puede confirmar la entrega.
	future := bookings.Booking{
		ID: f.repo.NewID(), ItemID: "i", OwnerID: "own", RequesterID: "req", Start: today.AddDays(10), End: today.AddDays(11), Days: 2,
		Item: bookings.ItemSnapshot{Modality: bookings.ModalityLoan}, PaymentMode: bookings.PaymentNone,
		Status: bookings.StatusAccepted, CreatedAt: *f.clock, UpdatedAt: *f.clock, Version: 1,
	}
	if err := f.repo.Create(context.Background(), future); err != nil {
		t.Fatal(err)
	}
	base := "/bookings/" + future.ID
	f.expect(200, "own", "PUT", base+"/delivery/evidence", evidenceBody)
	tok := f.expect(201, "own", "POST", base+"/delivery/qr", "")["token"].(string)
	f.expect(409, "req", "POST", base+"/delivery/confirm", fmt.Sprintf(`{"token":%q}`, tok))
}

// RNF-02: con el mismo QR a la vez, exactamente una confirmación tiene éxito.
func TestIntegrationConcurrentConfirmationsOnlyOneWins(t *testing.T) {
	f := newFlow(t)
	id := f.accepted()
	base := "/bookings/" + id
	f.expect(200, "own", "PUT", base+"/delivery/evidence", evidenceBody)
	tok := f.expect(201, "own", "POST", base+"/delivery/qr", "")["token"].(string)

	const n = 12
	codes := make([]int, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			codes[i], _ = f.call("req", "POST", base+"/delivery/confirm", fmt.Sprintf(`{"token":%q}`, tok))
		}()
	}
	close(start)
	wg.Wait()
	wins, conflicts := 0, 0
	for _, c := range codes {
		switch c {
		case 200:
			wins++
		case 409:
			conflicts++
		default:
			t.Errorf("código inesperado %d", c)
		}
	}
	if wins != 1 || conflicts != n-1 {
		t.Fatalf("200 = %d, 409 = %d; quiero 1 y %d", wins, conflicts, n-1)
	}
	if got := f.expect(200, "own", "GET", base, ""); got["status"] != "in_progress" {
		t.Errorf("status = %v", got["status"])
	}
	// Una sola notificación de entrega confirmada.
	count := 0
	for _, s := range f.not.sent {
		if strings.HasPrefix(s, "delivery_confirmed") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("avisos de entrega = %d, quiero 1", count)
	}
}

func TestIntegrationRoutesNeedSession(t *testing.T) {
	f := newFlow(t)
	id := f.accepted()
	for _, c := range []struct{ m, p string }{
		{"PUT", "/delivery/evidence"}, {"POST", "/delivery/qr"}, {"POST", "/delivery/confirm"}, {"GET", "/delivery"},
		{"PUT", "/return/evidence"}, {"POST", "/return/qr"}, {"POST", "/return/confirm"}, {"GET", "/return"},
	} {
		f.expect(401, "", c.m, "/bookings/"+id+c.p, "{}")
	}
}
