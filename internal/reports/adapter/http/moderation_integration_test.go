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
	chathttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/http"
	chatmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/mongo"
	chatusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/usecase"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	itemshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/http"
	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
	itemsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	notificationsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	reportsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/adapter/mongo"
	reportsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/usecase"
	searchhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/adapter/http"
	searchmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/adapter/mongo"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
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

type modImages struct{}

func (modImages) Signature(context.Context, string, images.Purpose) (images.UploadSignature, error) {
	return images.UploadSignature{}, nil
}
func (modImages) Resolve(_ context.Context, _ string, _ images.Purpose, _ string, refs []images.Ref) ([]images.Image, error) {
	out := make([]images.Image, 0, len(refs))
	for _, r := range refs {
		out = append(out, images.Image{PublicID: r.PublicID, Width: 800, Height: 600, Format: "jpg", Bytes: 1000, Delivery: images.DeliveryPublic})
	}
	return out, nil
}
func (modImages) URLs(i images.Image) (images.URLs, error) {
	return images.URLs{URL: "https://img/" + i.PublicID, ThumbURL: "https://thumb/" + i.PublicID}, nil
}

func modPass(next http.Handler) http.Handler { return next }

type modWorld struct {
	h   http.Handler
	db  *drv.Database
	ids map[string]string
}

// newModWorld monta items (005), búsqueda (006), reservas (007), chat (008) y
// reportes (011) como en cmd/api, con el filtro real de contenido y el
// Notifier real (sin push), sobre MongoDB real.
func newModWorld(t *testing.T) modWorld {
	t.Helper()
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("moderation_flow_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	if _, err := itemsmongo.EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	itemRepo, bookingRepo := itemsmongo.NewItemRepo(db), bookingsmongo.NewRepo(db)
	convs, msgs := chatmongo.NewConversationRepo(db), chatmongo.NewMessageRepo(db)
	notifRepo, reportRepo := notificationsmongo.NewNotificationRepo(db), reportsmongo.NewRepo(db)
	for _, e := range []interface{ EnsureIndexes(context.Context) error }{itemRepo, searchmongo.NewIndexes(db), bookingRepo, convs, msgs, notifRepo, reportRepo} {
		if err := e.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.CreateCollection(ctx, bookingsmongo.BookingsCollection)

	w := modWorld{db: db, ids: map[string]string{}}
	now := time.Now().UTC()
	for _, name := range []string{"own", "req", "otro", "admin"} {
		res, err := db.Collection(authmongo.UsersCollection).InsertOne(ctx, bson.M{
			"displayName": name, "status": "active", "role": "user", "ratingSum": int64(0), "ratingCount": int64(0),
			"createdAt": now, "updatedAt": now, "version": int64(1),
		})
		if err != nil {
			t.Fatal(err)
		}
		w.ids[name] = res.InsertedID.(bson.ObjectID).Hex()
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userRepo := authmongo.NewUserRepo(db)
	var mod moderation.Checker = moderation.NewFilter()
	notifier, err := notify.NewService(notify.ServiceDeps{Store: notifRepo, Users: userRepo, Log: log})
	if err != nil {
		t.Fatal(err)
	}

	itemsSvc, err := itemsusecase.NewService(itemsusecase.Deps{Items: itemRepo, Images: modImages{}, Moderation: mod, Users: userRepo, Bookings: bookingsmongo.NewItemActivity(db)})
	if err != nil {
		t.Fatal(err)
	}
	itemsMod, _ := itemshttp.NewModule(itemshttp.ModuleDeps{Service: itemsSvc, Authenticate: testAuth, RequireActive: modPass, Log: log})
	searchSvc, _ := searchusecase.NewService(searchusecase.Deps{Repo: searchmongo.NewRepo(db, nil), Images: modImages{}})
	searchMod, _ := searchhttp.NewModule(searchhttp.ModuleDeps{Service: searchSvc, Authenticate: testAuth, Log: log})
	chatSvc, err := chatusecase.NewService(chatusecase.Deps{
		Conversations: convs, Messages: msgs, Sender: chatmongo.NewSender(db), Bookings: chatmongo.NewBookingReader(db), Accounts: chatmongo.NewAccountReader(db),
		Users: userRepo, Images: modImages{}, Moderation: mod, Notifier: notifier, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	chatMod, _ := chathttp.NewModule(chathttp.ModuleDeps{Service: chatSvc, Authenticate: testAuth, Log: log, Limiter: redis.NewMemoryLimiter()})
	bookSvc, err := bookingsusecase.NewService(bookingsusecase.Deps{Repo: bookingRepo, Images: modImages{}, Users: userRepo, Chat: chatSvc, Notifier: notifier, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	bookMod, _ := bookingshttp.NewModule(bookingshttp.ModuleDeps{Service: bookSvc, Authenticate: testAuth, RequireActive: modPass, Log: log})
	reportsSvc, err := reportsusecase.NewService(reportsusecase.Deps{
		Repo: reportRepo, Items: itemsSvc, Messages: chatmongo.NewMessageReader(db), Users: userRepo, Admin: userRepo, Notifier: notifier, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	reportsMod, err := NewModule(ModuleDeps{Service: reportsSvc, Authenticate: testAuth, RequireAdmin: testAdmin, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	for _, m := range []interface{ Register(chi.Router) }{itemsMod, searchMod, bookMod, chatMod, reportsMod} {
		m.Register(r)
	}
	w.h = r
	return w
}

func (w modWorld) call(t *testing.T, who, method, path, body string) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("X-User", w.ids[who])
	if who == "admin" {
		req.Header.Set("X-Role", "admin")
	} else {
		req.Header.Set("X-Role", "user")
	}
	rec := httptest.NewRecorder()
	w.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// errDetail devuelve el primer details[] del envelope de error (Art. IV).
func errDetail(body map[string]any) (field, issue string) {
	e, _ := body["error"].(map[string]any)
	ds, _ := e["details"].([]any)
	if len(ds) == 0 {
		return "", ""
	}
	d := ds[0].(map[string]any)
	field, _ = d["field"].(string)
	issue, _ = d["issue"].(string)
	return field, issue
}

func modDay(n int) string {
	loc, _ := time.LoadLocation("America/Mexico_City")
	return time.Now().In(loc).AddDate(0, 0, n).Format("2006-01-02")
}

func itemBody(title string) string {
	return fmt.Sprintf(`{"title":%q,"description":"En buen estado","category":"hogar","condition":"good",
		"modality":"rent","declaredValueCents":100000,"pricePerDayCents":3000,"priceSource":"manual",
		"zone":{"municipality":"Coyoacán"},"photos":[{"publicId":"p1"}]}`, title)
}

func (w modWorld) searchHas(t *testing.T, who, itemID string) bool {
	t.Helper()
	code, out := w.call(t, who, "GET", "/items?from="+modDay(2)+"&to="+modDay(3), "")
	if code != 200 {
		t.Fatalf("búsqueda = %d %v", code, out)
	}
	for _, it := range out["items"].([]any) {
		if it.(map[string]any)["id"] == itemID {
			return true
		}
	}
	return false
}

func (w modWorld) notices(t *testing.T, who, typ string) int64 {
	t.Helper()
	n, err := w.db.Collection(notificationsmongo.NotificationsCollection).CountDocuments(context.Background(),
		bson.D{{Key: "userId", Value: w.ids[who]}, {Key: "type", Value: typ}})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIntegrationForbiddenItemTitleIsRejected(t *testing.T) { // CA-03
	w := newModWorld(t)
	code, out := w.call(t, "own", "POST", "/items", itemBody("Vendo pistola calibre 9"))
	field, issue := errDetail(out)
	if code != 400 || field != "title" || !strings.Contains(issue, "armas") {
		t.Fatalf("CA-03 = %d %v (campo %q, motivo %q)", code, out, field, issue)
	}
	// Los acentos y mayúsculas no la esquivan y un título normal sí se publica.
	if code, _ := w.call(t, "own", "POST", "/items", itemBody("COCAÍNA de calidad")); code != 400 {
		t.Errorf("con acentos y mayúsculas = %d, quiero 400", code)
	}
	if code, out := w.call(t, "own", "POST", "/items", itemBody("Pistola de calor para manualidades")); code != 201 {
		t.Errorf("una herramienta legítima no debe bloquearse: %d %v", code, out)
	}
}

func TestIntegrationReportedItemIsHiddenFromSearch(t *testing.T) { // CA-05, RF-08, RF-10, RF-11
	w := newModWorld(t)
	ctx := context.Background()
	code, item := w.call(t, "own", "POST", "/items", itemBody("Taladro"))
	if code != 201 {
		t.Fatalf("crear objeto = %d %v", code, item)
	}
	itemID := item["id"].(string)
	if !w.searchHas(t, "req", itemID) {
		t.Fatal("el objeto debe aparecer en la búsqueda antes del reporte")
	}

	body := fmt.Sprintf(`{"targetType":"item","targetId":%q,"reason":"prohibited_content","details":"No debería publicarse"}`, itemID)
	code, rep := w.call(t, "req", "POST", "/reports", body)
	if code != 201 {
		t.Fatalf("reportar = %d %v", code, rep)
	}
	reportID := rep["id"].(string)
	// Repetir el reporte devuelve el existente (RF-08).
	if code, again := w.call(t, "req", "POST", "/reports", body); code != 200 || again["id"] != reportID {
		t.Errorf("reporte repetido = %d %v, quiero 200 con el mismo id", code, again)
	}
	// El dueño no reporta su propio objeto.
	if code, _ := w.call(t, "own", "POST", "/reports", body); code != 409 {
		t.Errorf("objeto propio = %d, quiero 409", code)
	}

	// Rutas de administración: solo admin (RF-10).
	if code, _ := w.call(t, "req", "GET", "/admin/reports", ""); code != 403 {
		t.Errorf("listar sin ser admin = %d, quiero 403", code)
	}
	if code, _ := w.call(t, "req", "POST", "/admin/reports/"+reportID+"/resolve", `{"action":"dismiss","note":"nota"}`); code != 403 {
		t.Errorf("resolver sin ser admin = %d, quiero 403", code)
	}
	code, queue := w.call(t, "admin", "GET", "/admin/reports", "")
	rows, _ := queue["items"].([]any)
	if code != 200 || len(rows) != 1 {
		t.Fatalf("cola = %d %v", code, queue)
	}
	row := rows[0].(map[string]any)
	if row["id"] != reportID || row["status"] != "open" || row["target"].(map[string]any)["summary"] != "Taladro" || row["reporter"].(map[string]any)["displayName"] != "req" {
		t.Errorf("fila de la cola = %v", row)
	}

	// hide_item bloquea el objeto, lo saca de la búsqueda y avisa al dueño.
	code, done := w.call(t, "admin", "POST", "/admin/reports/"+reportID+"/resolve", `{"action":"hide_item","note":"Incumple la política de contenido."}`)
	if code != 200 || done["status"] != "actioned" || done["resolution"].(map[string]any)["action"] != "hide_item" {
		t.Fatalf("resolver = %d %v", code, done)
	}
	if w.searchHas(t, "req", itemID) {
		t.Error("CA-05: el objeto bloqueado no debe aparecer en la búsqueda")
	}
	if w.notices(t, "own", "content_removed") != 1 {
		t.Error("el dueño debe recibir el aviso content_removed")
	}
	// RF-11: la decisión conserva quién la tomó (en la base; la API no lo expone).
	var stored struct {
		Resolution struct {
			ByAdminID string `bson:"byAdminId"`
			Note      string `bson:"note"`
		} `bson:"resolution"`
	}
	if err := w.db.Collection(reportsmongo.ReportsCollection).FindOne(ctx, bson.D{}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if stored.Resolution.ByAdminID != w.ids["admin"] || stored.Resolution.Note != "Incumple la política de contenido." {
		t.Errorf("resolución guardada = %+v", stored.Resolution)
	}
	// Ya resuelto: no se resuelve dos veces.
	if code, _ := w.call(t, "admin", "POST", "/admin/reports/"+reportID+"/resolve", `{"action":"dismiss","note":"otra vez"}`); code != 409 {
		t.Errorf("segunda resolución = %d, quiero 409", code)
	}
	if code, q := w.call(t, "admin", "GET", "/admin/reports?status=actioned", ""); code != 200 || len(q["items"].([]any)) != 1 {
		t.Errorf("actioned = %d %v", code, q)
	}
	if code, q := w.call(t, "admin", "GET", "/admin/reports", ""); code != 200 || len(q["items"].([]any)) != 0 {
		t.Errorf("la cola abierta debe quedar vacía: %d %v", code, q)
	}
}

func TestIntegrationChatForbiddenMessageAndReportRules(t *testing.T) { // CA-04, CA-06, RF-08, RF-09
	w := newModWorld(t)
	ctx := context.Background()
	code, item := w.call(t, "own", "POST", "/items", itemBody("Taladro"))
	if code != 201 {
		t.Fatalf("crear objeto = %d %v", code, item)
	}
	code, booking := w.call(t, "req", "POST", "/bookings", fmt.Sprintf(
		`{"itemId":%q,"startDate":%q,"endDate":%q,"paymentMode":"full_on_delivery","acceptedTotalCents":3000}`, item["id"], modDay(2), modDay(2)))
	conv, _ := booking["conversationId"].(string)
	if code != 201 || conv == "" {
		t.Fatalf("solicitud = %d %v", code, booking)
	}
	path := "/conversations/" + conv + "/messages"

	// CA-04: un mensaje prohibido se rechaza con 400 y no se guarda.
	code, out := w.call(t, "req", "POST", path, `{"body":"te vendo cocaína"}`)
	if field, issue := errDetail(out); code != 400 || field != "body" || !strings.Contains(issue, "drogas") {
		t.Fatalf("CA-04 = %d %v", code, out)
	}
	if n, _ := w.db.Collection(chatmongo.MessagesCollection).CountDocuments(ctx, bson.D{}); n != 0 {
		t.Errorf("el mensaje prohibido no debe guardarse, hay %d", n)
	}
	code, msg := w.call(t, "own", "POST", path, `{"body":"Hola, ¿para qué día lo necesitas?"}`)
	if code != 201 {
		t.Fatalf("mensaje normal = %d %v", code, msg)
	}
	msgBody := fmt.Sprintf(`{"targetType":"message","targetId":%q,"reason":"inappropriate"}`, msg["id"])

	// CA-06: quien no participa recibe 404; no se revela que el mensaje existe.
	if code, out := w.call(t, "otro", "POST", "/reports", msgBody); code != 404 {
		t.Errorf("CA-06 no participante = %d %v, quiero 404", code, out)
	}
	// Quien escribió el mensaje no lo reporta; la otra parte sí.
	if code, _ := w.call(t, "own", "POST", "/reports", msgBody); code != 409 {
		t.Errorf("mensaje propio = %d, quiero 409", code)
	}
	code, rep := w.call(t, "req", "POST", "/reports", msgBody)
	if code != 201 {
		t.Fatalf("participante reporta = %d %v", code, rep)
	}

	// suspend_user: suspende 7 días a quien escribió el mensaje y lo avisa.
	code, done := w.call(t, "admin", "POST", "/admin/reports/"+rep["id"].(string)+"/resolve", `{"action":"suspend_user","note":"Mensajes ofensivos reiterados."}`)
	if code != 200 || done["status"] != "actioned" {
		t.Fatalf("suspend_user = %d %v", code, done)
	}
	var user bson.M
	if err := w.db.Collection(authmongo.UsersCollection).FindOne(ctx, bson.D{{Key: "displayName", Value: "own"}}).Decode(&user); err != nil {
		t.Fatal(err)
	}
	until, ok := user["suspendedUntil"].(bson.DateTime)
	if !ok {
		t.Fatalf("la persona no quedó suspendida: %v", user)
	}
	if d := time.Until(until.Time()); d < 6*24*time.Hour || d > 7*24*time.Hour+time.Minute {
		t.Errorf("suspensión de %v, quiero ~7 días", d)
	}
	if w.notices(t, "own", "sanction_applied") != 1 {
		t.Error("la persona suspendida debe recibir sanction_applied")
	}

	// RF-09: se reporta un rechazo automático para seguimiento manual.
	code, dec := w.call(t, "req", "POST", "/reports", `{"targetType":"moderation_decision","targetId":"rechazo-titulo-1","reason":"other","details":"Mi título era una herramienta."}`)
	if code != 201 {
		t.Fatalf("moderation_decision = %d %v", code, dec)
	}
	if code, _ := w.call(t, "admin", "POST", "/admin/reports/"+dec["id"].(string)+"/resolve", `{"action":"suspend_user","note":"No aplica"}`); code != 409 {
		t.Errorf("suspend_user sobre decisión = %d, quiero 409", code)
	}
	if code, d := w.call(t, "admin", "POST", "/admin/reports/"+dec["id"].(string)+"/resolve", `{"action":"dismiss","note":"Revisado: es válido."}`); code != 200 || d["status"] != "dismissed" {
		t.Errorf("dismiss = %d %v", code, d)
	}
}
