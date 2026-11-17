//go:build integration

package http

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	bookingshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/http"
	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	bookingsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	chatmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/usecase"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
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
func (itImages) Resolve(context.Context, string, images.Purpose, string, []images.Ref) ([]images.Image, error) {
	return nil, nil
}
func (itImages) URLs(i images.Image) (images.URLs, error) {
	return images.URLs{ThumbURL: "https://thumb/" + i.PublicID}, nil
}

type itUsers struct{}

func (itUsers) PublicUsers(_ context.Context, ids []string) (map[string]users.PublicUser, error) {
	out := map[string]users.PublicUser{}
	for _, id := range ids {
		out[id] = users.PublicUser{ID: id, DisplayName: "Persona"}
	}
	return out, nil
}

type itNotifier struct{ events []string }

func (n *itNotifier) Notify(_ context.Context, to string, e notify.Event) error {
	if e.Type == usecase.EventMessageReceived {
		n.events = append(n.events, to)
	}
	return nil
}

func passThrough(next http.Handler) http.Handler { return next }

type world struct {
	h    http.Handler
	db   *drv.Database
	ids  map[string]string
	not  *itNotifier
	item string
}

// newWorld monta reservas (007) y chat (008) como en cmd/api, con el chat
// como chat.Opener real, sobre MongoDB real.
func newWorld(t *testing.T) world {
	t.Helper()
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("chat_flow_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	bookingRepo := bookingsmongo.NewRepo(db)
	convs, msgs := chatmongo.NewConversationRepo(db), chatmongo.NewMessageRepo(db)
	for _, e := range []interface{ EnsureIndexes(context.Context) error }{bookingRepo, convs, msgs} {
		if err := e.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.CreateCollection(ctx, "items")
	w := world{db: db, ids: map[string]string{}, not: &itNotifier{}}
	for _, name := range []string{"own", "req", "otro"} {
		res, _ := db.Collection("users").InsertOne(ctx, bson.M{"displayName": name, "status": "active"})
		w.ids[name] = res.InsertedID.(bson.ObjectID).Hex()
	}
	res, _ := db.Collection("items").InsertOne(ctx, bson.M{
		"ownerId": w.ids["own"], "status": "active", "title": "Taladro", "modality": "rent",
		"pricePerDayCents": int64(3000), "version": int64(1), "photos": bson.A{bson.M{"publicId": "p1"}},
	})
	w.item = res.InsertedID.(bson.ObjectID).Hex()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	chatSvc, err := usecase.NewService(usecase.Deps{
		Conversations: convs, Messages: msgs, Sender: chatmongo.NewSender(db), Bookings: chatmongo.NewBookingReader(db), Accounts: chatmongo.NewAccountReader(db),
		Users: itUsers{}, Images: itImages{}, Moderation: moderation.AllowAll{}, Notifier: w.not, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	chatMod, _ := NewModule(ModuleDeps{Service: chatSvc, Authenticate: testAuth, Log: log, Limiter: redis.NewMemoryLimiter()})
	bookSvc, err := bookingsusecase.NewService(bookingsusecase.Deps{Repo: bookingRepo, Images: itImages{}, Users: itUsers{},
		Chat: chatSvc, Notifier: notify.Noop{}, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	bookMod, _ := bookingshttp.NewModule(bookingshttp.ModuleDeps{Service: bookSvc, Authenticate: testAuth, RequireActive: passThrough, Log: log})
	r := chi.NewRouter()
	chatMod.Register(r)
	bookMod.Register(r)
	w.h = r
	return w
}

func (w world) call(t *testing.T, who, method, path, body string) (int, map[string]any) {
	t.Helper()
	return tenv{h: w.h}.do(t, method, path, body, w.ids[who])
}

func day(n int) string {
	loc, _ := time.LoadLocation("America/Mexico_City")
	return time.Now().In(loc).AddDate(0, 0, n).Format("2006-01-02")
}

// request crea una solicitud de req y devuelve su id y el de su conversación.
func (w world) request(t *testing.T, who string, start int) (string, string) {
	t.Helper()
	code, b := w.call(t, who, "POST", "/bookings", fmt.Sprintf(
		`{"itemId":%q,"startDate":%q,"endDate":%q,"paymentMode":"full_on_delivery","acceptedTotalCents":3000}`, w.item, day(start), day(start)))
	if code != 201 {
		t.Fatalf("solicitud = %d %v", code, b)
	}
	conv, _ := b["conversationId"].(string)
	return b["id"].(string), conv
}

func TestIntegrationChatFlow(t *testing.T) {
	w := newWorld(t)
	bookingID, conv := w.request(t, "req", 3)
	// CA-01: la solicitud abre su conversación con 2 participantes.
	if conv == "" {
		t.Fatal("la solicitud no trae conversationId")
	}
	var doc bson.M
	_ = w.db.Collection(chatmongo.ConversationsCollection).FindOne(context.Background(), bson.D{{Key: "bookingId", Value: bookingID}}).Decode(&doc)
	if p, _ := doc["participantIds"].(bson.A); len(p) != 2 {
		t.Errorf("participantes = %v", doc["participantIds"])
	}
	code, list := w.call(t, "own", "GET", "/conversations", "")
	items := list["items"].([]any)
	if code != 200 || len(items) != 1 || items[0].(map[string]any)["canWrite"] != true || items[0].(map[string]any)["bookingStatus"] != "pending" {
		t.Fatalf("lista = %d %v", code, list)
	}

	// CA-06: 3 mensajes → el otro ve 3; tras read, 0; quien envía no cambia.
	path := "/conversations/" + conv
	var lastID string
	for i := range 3 {
		code, m := w.call(t, "req", "POST", path+"/messages", fmt.Sprintf(`{"body":"mensaje %d"}`, i))
		if code != 201 {
			t.Fatalf("enviar = %d %v", code, m)
		}
		lastID = m["id"].(string)
	}
	if _, u := w.call(t, "own", "GET", "/conversations/unread-count", ""); u["count"] != float64(3) {
		t.Errorf("own no leídos = %v", u)
	}
	if _, u := w.call(t, "req", "GET", "/conversations/unread-count", ""); u["count"] != float64(0) {
		t.Errorf("req no leídos = %v", u)
	}
	if len(w.not.events) != 1 || w.not.events[0] != w.ids["own"] { // RF-12
		t.Errorf("avisos = %v", w.not.events)
	}
	if code, _ := w.call(t, "own", "POST", path+"/read", ""); code != 204 {
		t.Error("read")
	}
	if _, u := w.call(t, "own", "GET", "/conversations/unread-count", ""); u["count"] != float64(0) {
		t.Errorf("tras leer = %v", u)
	}

	// Consulta periódica: el propietario responde y req pide after=<último>.
	_, reply := w.call(t, "own", "POST", path+"/messages", `{"body":"¿A las 5?"}`)
	code, after := w.call(t, "req", "GET", path+"/messages?after="+lastID, "")
	if code != 200 || len(after["items"].([]any)) != 1 || after["items"].([]any)[0].(map[string]any)["id"] != reply["id"] || after["nextCursor"] != nil {
		t.Errorf("after = %d %v", code, after)
	}
	code, hist := w.call(t, "req", "GET", path+"/messages?limit=2", "")
	if code != 200 || len(hist["items"].([]any)) != 2 || hist["nextCursor"] == nil {
		t.Errorf("historial = %v", hist)
	}
	_, older := w.call(t, "req", "GET", path+"/messages?limit=2&before="+hist["nextCursor"].(string), "")
	if len(older["items"].([]any)) != 2 || older["nextCursor"] != nil {
		t.Errorf("anteriores = %v", older)
	}

	// CA-02: una persona ajena recibe 404 en todo.
	for _, c := range []struct{ m, p, b string }{{"GET", path + "/messages", ""}, {"POST", path + "/messages", `{"body":"hola"}`}, {"POST", path + "/read", ""}} {
		if code, _ := w.call(t, "otro", c.m, c.p, c.b); code != 404 {
			t.Errorf("ajeno %s %s = %d", c.m, c.p, code)
		}
	}
	// CA-03: el mismo clientId dos veces → un solo mensaje.
	c1, m1 := w.call(t, "req", "POST", path+"/messages", `{"body":"hola","clientId":"k-1"}`)
	c2, m2 := w.call(t, "req", "POST", path+"/messages", `{"body":"hola","clientId":"k-1"}`)
	if c1 != 201 || c2 != 200 || m1["id"] != m2["id"] {
		t.Errorf("clientId = %d %d", c1, c2)
	}
	// CA-05: 1001 caracteres → 400.
	if code, _ := w.call(t, "req", "POST", path+"/messages", `{"body":"`+strings.Repeat("a", 1001)+`"}`); code != 400 {
		t.Errorf("CA-05 = %d", code)
	}
	// CA-04: la solicitud se cancela → escribir 409, el historial se lee.
	if code, _ := w.call(t, "req", "POST", "/bookings/"+bookingID+"/cancel", ""); code != 200 {
		t.Fatal("cancelar")
	}
	if code, out := w.call(t, "own", "POST", path+"/messages", `{"body":"hola"}`); code != 409 || !strings.Contains(fmt.Sprint(out), "La conversación está cerrada.") {
		t.Errorf("CA-04 = %d %v", code, out)
	}
	if code, h := w.call(t, "own", "GET", path+"/messages", ""); code != 200 || len(h["items"].([]any)) != 5 {
		t.Errorf("historial tras cancelar = %d", len(h["items"].([]any)))
	}
	if _, l := w.call(t, "own", "GET", "/conversations", ""); l["items"].([]any)[0].(map[string]any)["canWrite"] != false {
		t.Error("canWrite tras cancelar")
	}
}

// RNF-01: after responde en < 100 ms con 5,000 mensajes.
func TestIntegrationPollingPerformance(t *testing.T) {
	w := newWorld(t)
	_, conv := w.request(t, "req", 3)
	docs := make([]any, 0, 5000)
	base := time.Now().Add(-time.Hour)
	for i := range 5000 {
		docs = append(docs, bson.M{"_id": bson.NewObjectIDFromTimestamp(base.Add(time.Duration(i) * time.Millisecond)),
			"conversationId": conv, "senderId": w.ids["req"], "seq": int64(i + 1), "body": fmt.Sprint(i), "createdAt": base})
	}
	if _, err := w.db.Collection(chatmongo.MessagesCollection).InsertMany(context.Background(), docs); err != nil {
		t.Fatal(err)
	}
	convOID, _ := bson.ObjectIDFromHex(conv)
	_, _ = w.db.Collection(chatmongo.ConversationsCollection).UpdateOne(context.Background(),
		bson.D{{Key: "_id", Value: convOID}}, bson.D{{Key: "$set", Value: bson.D{{Key: "seq", Value: int64(5000)}}}})
	_, last := w.call(t, "req", "POST", "/conversations/"+conv+"/messages", `{"body":"último"}`)
	var times []time.Duration
	for range 30 {
		start := time.Now()
		code, out := w.call(t, "own", "GET", "/conversations/"+conv+"/messages?after="+last["id"].(string), "")
		times = append(times, time.Since(start))
		if code != 200 || len(out["items"].([]any)) != 0 {
			t.Fatalf("after = %d", code)
		}
	}
	slices.Sort(times)
	p95 := times[len(times)*95/100]
	t.Logf("after con 5,000 mensajes: p50=%v p95=%v", times[len(times)/2].Round(time.Microsecond), p95.Round(time.Microsecond))
	if p95 > 100*time.Millisecond {
		t.Errorf("p95 = %v (RNF-01: < 100 ms)", p95)
	}
}
