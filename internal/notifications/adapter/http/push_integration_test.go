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
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	bookingshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/http"
	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	bookingsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	notificationsexpo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/expo"
	notificationsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/mongo"
	notificationsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/worker"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
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
func (itImages) Resolve(context.Context, string, images.Purpose, string, []images.Ref) ([]images.Image, error) {
	return nil, nil
}
func (itImages) URLs(i images.Image) (images.URLs, error) {
	return images.URLs{ThumbURL: "https://thumb/" + i.PublicID}, nil
}

// fakeExpo es el servidor Expo simulado: guarda los mensajes que recibe y
// responde ok, o DeviceNotRegistered para los tokens de invalid.
type fakeExpo struct {
	srv     *httptest.Server
	mu      sync.Mutex
	got     []map[string]any
	invalid map[string]bool
	calls   int
}

func newFakeExpo(t *testing.T) *fakeExpo {
	t.Helper()
	f := &fakeExpo{invalid: map[string]bool{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msgs []map[string]any
		_ = json.NewDecoder(r.Body).Decode(&msgs)
		f.mu.Lock()
		f.calls++
		f.got = append(f.got, msgs...)
		tickets := make([]map[string]any, len(msgs))
		for i, m := range msgs {
			if f.invalid[m["to"].(string)] {
				tickets[i] = map[string]any{"status": "error", "message": "no", "details": map[string]any{"error": "DeviceNotRegistered"}}
			} else {
				tickets[i] = map[string]any{"status": "ok", "id": fmt.Sprintf("t%d", i)}
			}
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": tickets})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeExpo) messages() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.got...)
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no se cumplió a tiempo: %s", what)
}

func passThrough(next http.Handler) http.Handler { return next }

type pushWorld struct {
	h      http.Handler
	db     *drv.Database
	ids    map[string]string
	item   string
	expo   *fakeExpo
	tokens *notificationsmongo.TokenRepo
}

// newPushWorld monta reservas (007) y avisos (011) como en cmd/api —el
// Notifier real con el worker de push apuntando a Expo simulado— sobre
// MongoDB real.
func newPushWorld(t *testing.T) pushWorld {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	db := testClient.Database(fmt.Sprintf("push_flow_%d", time.Now().UnixNano()))
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
		_ = db.Drop(context.Background())
	})

	notifRepo, tokenRepo := notificationsmongo.NewNotificationRepo(db), notificationsmongo.NewTokenRepo(db)
	bookingRepo := bookingsmongo.NewRepo(db)
	for _, e := range []interface{ EnsureIndexes(context.Context) error }{notifRepo, tokenRepo, bookingRepo} {
		if err := e.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.CreateCollection(ctx, "items")
	w := pushWorld{db: db, ids: map[string]string{}, expo: newFakeExpo(t), tokens: tokenRepo}
	now := time.Now().UTC()
	for _, name := range []string{"own", "req"} {
		res, err := db.Collection(authmongo.UsersCollection).InsertOne(ctx, bson.M{
			"displayName": name, "status": "active", "role": "user", "ratingSum": int64(0), "ratingCount": int64(0),
			"createdAt": now, "updatedAt": now, "version": int64(1),
		})
		if err != nil {
			t.Fatal(err)
		}
		w.ids[name] = res.InsertedID.(bson.ObjectID).Hex()
	}
	res, err := db.Collection("items").InsertOne(ctx, bson.M{
		"ownerId": w.ids["own"], "status": "active", "title": "Taladro", "modality": "rent",
		"pricePerDayCents": int64(3000), "version": int64(1), "photos": bson.A{bson.M{"publicId": "p1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	w.item = res.InsertedID.(bson.ObjectID).Hex()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userRepo := authmongo.NewUserRepo(db)
	pushWorker, err := worker.New(worker.Deps{Tokens: tokenRepo, Sender: notificationsexpo.NewClient(w.expo.srv.URL, "", 0), Log: log})
	if err != nil {
		t.Fatal(err)
	}
	go func() { pushWorker.Run(ctx); close(done) }()
	notifier, err := notify.NewService(notify.ServiceDeps{Store: notifRepo, Users: userRepo, Pusher: pushWorker, Log: log})
	if err != nil {
		t.Fatal(err)
	}

	bookSvc, err := bookingsusecase.NewService(bookingsusecase.Deps{
		Repo: bookingRepo, Images: itImages{}, Users: userRepo, Chat: chat.Noop{}, Notifier: notifier, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	bookMod, err := bookingshttp.NewModule(bookingshttp.ModuleDeps{Service: bookSvc, Authenticate: testAuth, RequireActive: passThrough, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	notifSvc, err := notificationsusecase.NewService(notificationsusecase.Deps{Repo: notifRepo, Tokens: tokenRepo})
	if err != nil {
		t.Fatal(err)
	}
	notifMod, err := NewModule(ModuleDeps{Service: notifSvc, Authenticate: testAuth, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	bookMod.Register(r)
	notifMod.Register(r)
	w.h = r
	return w
}

func (w pushWorld) call(t *testing.T, who, method, path, body string) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("X-User", w.ids[who])
	rec := httptest.NewRecorder()
	w.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func dayStr(n int) string {
	loc, _ := time.LoadLocation("America/Mexico_City")
	return time.Now().In(loc).AddDate(0, 0, n).Format("2006-01-02")
}

func (w pushWorld) request(t *testing.T, start int) string {
	t.Helper()
	code, b := w.call(t, "req", "POST", "/bookings", fmt.Sprintf(
		`{"itemId":%q,"startDate":%q,"endDate":%q,"paymentMode":"full_on_delivery","acceptedTotalCents":3000}`, w.item, dayStr(start), dayStr(start)))
	if code != 201 {
		t.Fatalf("la solicitud debe crearse aunque falle el push: %d %v", code, b)
	}
	return b["id"].(string)
}

const ownerToken = "ExponentPushToken[owner-device]"

func TestIntegrationBookingRequestCreatesNoticeAndPush(t *testing.T) { // CA-01, RF-05, RF-06, CA-07
	w := newPushWorld(t)
	if code, b := w.call(t, "own", "POST", "/me/push-tokens", `{"token":"`+ownerToken+`","platform":"android"}`); code != 204 {
		t.Fatalf("registrar token = %d %v", code, b)
	}

	bookingID := w.request(t, 3)

	// Aviso guardado para el propietario, con ids y texto de plantilla.
	code, list := w.call(t, "own", "GET", "/me/notifications", "")
	items, _ := list["items"].([]any)
	if code != 200 || len(items) != 1 {
		t.Fatalf("avisos del propietario = %d %v", code, list)
	}
	n := items[0].(map[string]any)
	data := n["data"].(map[string]any)
	if n["type"] != "booking_requested" || n["readAt"] != nil || data["bookingId"] != bookingID || data["itemId"] != w.item {
		t.Errorf("aviso = %v", n)
	}
	if n["title"] == "" || n["body"] == "" {
		t.Errorf("el aviso debe llevar título y cuerpo en español: %v", n)
	}
	// Quien solicita no recibe aviso de su propia solicitud.
	if _, mine := w.call(t, "req", "GET", "/me/notifications", ""); len(mine["items"].([]any)) != 0 {
		t.Errorf("el solicitante no debe tener avisos: %v", mine)
	}
	if _, c := w.call(t, "own", "GET", "/me/notifications/unread-count", ""); c["count"] != float64(1) {
		t.Errorf("no leídos = %v, quiero 1", c)
	}

	// Push enviado a Expo con data.bookingId y sin datos personales.
	eventually(t, "push recibido por Expo simulado", func() bool { return len(w.expo.messages()) == 1 })
	m := w.expo.messages()[0]
	pd, _ := m["data"].(map[string]any)
	if m["to"] != ownerToken || pd["bookingId"] != bookingID || pd["itemId"] != w.item || m["title"] != n["title"] || m["body"] != n["body"] {
		t.Errorf("push = %v", m)
	}
	for k := range pd {
		if k != "bookingId" && k != "itemId" {
			t.Errorf("el push solo lleva ids, trae %q", k)
		}
	}

	// CA-07: read all → unread-count 0.
	if code, _ := w.call(t, "own", "POST", "/me/notifications/read", `{"all":true}`); code != 204 {
		t.Fatalf("read all = %d", code)
	}
	if _, c := w.call(t, "own", "GET", "/me/notifications/unread-count", ""); c["count"] != float64(0) {
		t.Errorf("tras read all, no leídos = %v", c)
	}
}

func TestIntegrationNoTokenNoPushButNoticeExists(t *testing.T) {
	w := newPushWorld(t)
	w.request(t, 3)
	if _, c := w.call(t, "own", "GET", "/me/notifications/unread-count", ""); c["count"] != float64(1) {
		t.Errorf("el aviso debe existir sin token: %v", c)
	}
	time.Sleep(300 * time.Millisecond)
	if len(w.expo.messages()) != 0 {
		t.Errorf("sin token no se envía push: %v", w.expo.messages())
	}
}

func TestIntegrationDeviceNotRegisteredDeletesTokenAndBusinessSucceeds(t *testing.T) { // CA-02
	w := newPushWorld(t)
	w.expo.invalid[ownerToken] = true
	if code, _ := w.call(t, "own", "POST", "/me/push-tokens", `{"token":"`+ownerToken+`","platform":"ios"}`); code != 204 {
		t.Fatal("registrar token")
	}
	w.request(t, 3) // el negocio no falla (request exige 201)
	eventually(t, "token eliminado tras DeviceNotRegistered", func() bool {
		got, err := w.tokens.TokensOf(context.Background(), w.ids["own"])
		return err == nil && len(got) == 0
	})
	if _, c := w.call(t, "own", "GET", "/me/notifications/unread-count", ""); c["count"] != float64(1) {
		t.Errorf("el aviso se conserva aunque el push falle: %v", c)
	}
}

func TestIntegrationTokenReassignedToNewUser(t *testing.T) { // RF-04
	w := newPushWorld(t)
	body := `{"token":"` + ownerToken + `","platform":"android"}`
	if code, _ := w.call(t, "req", "POST", "/me/push-tokens", body); code != 204 {
		t.Fatal("registrar como req")
	}
	if code, _ := w.call(t, "own", "POST", "/me/push-tokens", body); code != 204 {
		t.Fatal("registrar como own")
	}
	if got, _ := w.tokens.TokensOf(context.Background(), w.ids["req"]); len(got) != 0 {
		t.Errorf("req ya no debe tener el token: %v", got)
	}
	// Quien ya no es dueño del token no puede borrarlo.
	if code, _ := w.call(t, "req", "DELETE", "/me/push-tokens", `{"token":"`+ownerToken+`"}`); code != 204 {
		t.Fatal("delete idempotente")
	}
	if got, _ := w.tokens.TokensOf(context.Background(), w.ids["own"]); len(got) != 1 {
		t.Errorf("own debe conservar su token: %v", got)
	}
	if code, _ := w.call(t, "own", "DELETE", "/me/push-tokens", `{"token":"`+ownerToken+`"}`); code != 204 {
		t.Fatal("delete propio")
	}
	if got, _ := w.tokens.TokensOf(context.Background(), w.ids["own"]); len(got) != 0 {
		t.Errorf("el token debe borrarse: %v", got)
	}
}

func TestIntegrationDeletedAccountGetsNoNoticeNorPush(t *testing.T) { // RF-05 / T004
	w := newPushWorld(t)
	ctx := context.Background()
	if code, _ := w.call(t, "own", "POST", "/me/push-tokens", `{"token":"`+ownerToken+`","platform":"ios"}`); code != 204 {
		t.Fatal("registrar token")
	}
	// Baja de la cuenta del propietario: el nombre público pasa a «Cuenta eliminada».
	if _, err := w.db.Collection(authmongo.UsersCollection).UpdateOne(ctx,
		bson.D{{Key: "displayName", Value: "own"}}, bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "deleted"}}}}); err != nil {
		t.Fatal(err)
	}
	w.request(t, 3)
	time.Sleep(300 * time.Millisecond)
	if len(w.expo.messages()) != 0 {
		t.Errorf("no se debe enviar push a una cuenta dada de baja: %v", w.expo.messages())
	}
	if _, c := w.call(t, "own", "GET", "/me/notifications/unread-count", ""); c["count"] != float64(0) {
		t.Errorf("tampoco se guarda aviso: %v", c)
	}
}
