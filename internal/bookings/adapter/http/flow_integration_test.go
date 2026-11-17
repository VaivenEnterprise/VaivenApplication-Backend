//go:build integration

package http

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	itemshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/http"
	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
	itemsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	searchhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/adapter/http"
	searchmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/adapter/mongo"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
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
	out := []images.Image{}
	for _, r := range refs {
		out = append(out, images.Image{PublicID: r.PublicID, Format: "jpg", Delivery: images.DeliveryPublic})
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
		out[id] = users.PublicUser{ID: id, DisplayName: "Persona"}
	}
	return out, nil
}

type itNotifier struct{ types []string }

func (n *itNotifier) Notify(_ context.Context, _ string, e notify.Event) error {
	n.types = append(n.types, e.Type)
	return nil
}

func passThrough(next http.Handler) http.Handler { return next }

// router monta items (005), búsqueda (006) y reservas (007) como en cmd/api,
// sobre MongoDB real. Devuelve los ids de las cuentas sembradas.
func router(t *testing.T) (http.Handler, map[string]string, *itNotifier) {
	t.Helper()
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("bookings_flow_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	if _, err := itemsmongo.EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	itemRepo := itemsmongo.NewItemRepo(db)
	bookingRepo := bookingsmongo.NewRepo(db)
	for _, e := range []interface{ EnsureIndexes(context.Context) error }{itemRepo, searchmongo.NewIndexes(db), bookingRepo} {
		if err := e.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.CreateCollection(ctx, bookingsmongo.BookingsCollection)
	ids := map[string]string{}
	for _, name := range []string{"own", "req", "r2", "r3"} {
		res, err := db.Collection("users").InsertOne(ctx, bson.M{"displayName": name, "status": "active", "ratingSum": int64(0), "ratingCount": int64(0)})
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = res.InsertedID.(bson.ObjectID).Hex()
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	not := &itNotifier{}

	itemsSvc, _ := itemsusecase.NewService(itemsusecase.Deps{Items: itemRepo, Images: itImages{}, Moderation: moderation.AllowAll{},
		Users: itUsers{}, Bookings: bookingsmongo.NewItemActivity(db)})
	itemsMod, _ := itemshttp.NewModule(itemshttp.ModuleDeps{Service: itemsSvc, Authenticate: testAuth, RequireActive: passThrough, Log: log})
	searchSvc, _ := searchusecase.NewService(searchusecase.Deps{Repo: searchmongo.NewRepo(db, nil), Images: itImages{}})
	searchMod, _ := searchhttp.NewModule(searchhttp.ModuleDeps{Service: searchSvc, Authenticate: testAuth, Log: log})
	bookSvc, err := usecase.NewService(usecase.Deps{Repo: bookingRepo, Images: itImages{}, Users: itUsers{}, Chat: chat.Noop{}, Notifier: not, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	bookMod, _ := NewModule(ModuleDeps{Service: bookSvc, Authenticate: testAuth, RequireActive: passThrough, Log: log})

	r := chi.NewRouter()
	for _, m := range []interface{ Register(chi.Router) }{itemsMod, searchMod, bookMod} {
		m.Register(r)
	}
	return r, ids, not
}

func call(t *testing.T, h http.Handler, user, method, path, body string) (int, map[string]any) {
	t.Helper()
	return tenv{h: h}.do(t, method, path, body, "X-User", user)
}

// day devuelve la fecha de hoy + n en Ciudad de México.
func day(n int) string {
	loc, _ := time.LoadLocation("America/Mexico_City")
	return time.Now().In(loc).AddDate(0, 0, n).Format("2006-01-02")
}

func searchHas(t *testing.T, h http.Handler, user, from, to, itemID string) bool {
	t.Helper()
	code, out := call(t, h, user, "GET", "/items?from="+from+"&to="+to, "")
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

func TestIntegrationBookingFlow(t *testing.T) {
	h, u, not := router(t)
	own, req, r2, r3 := u["own"], u["req"], u["r2"], u["r3"]

	code, item := call(t, h, own, "POST", "/items", `{"title":"Taladro","description":"x","category":"hogar","condition":"good",
		"modality":"rent","declaredValueCents":100000,"pricePerDayCents":3000,"priceSource":"manual",
		"zone":{"municipality":"Coyoacán"},"photos":[{"publicId":"p1"}]}`)
	if code != 201 {
		t.Fatalf("crear objeto = %d %v", code, item)
	}
	itemID := item["id"].(string)
	s, e := day(3), day(5)

	// CA-01: 3 días × $30 = $90; CA-02 (50/50).
	code, q := call(t, h, req, "GET", "/items/"+itemID+"/quote?startDate="+s+"&endDate="+e+"&paymentMode=split_50_50", "")
	if code != 200 || q["days"] != float64(3) || q["totalCents"] != float64(9000) {
		t.Fatalf("cotizar = %d %v", code, q)
	}
	// CA-04: el propio objeto no se solicita.
	if code, _ := call(t, h, own, "GET", "/items/"+itemID+"/quote?startDate="+s+"&endDate="+e+"&paymentMode=split_50_50", ""); code != 403 {
		t.Errorf("CA-04 = %d", code)
	}

	body := func(start, end string, total int) string {
		return fmt.Sprintf(`{"itemId":%q,"startDate":%q,"endDate":%q,"paymentMode":"split_50_50","acceptedTotalCents":%d}`, itemID, start, end, total)
	}
	code, b1 := call(t, h, req, "POST", "/bookings", body(s, e, 9000))
	if code != 201 || b1["status"] != "pending" {
		t.Fatalf("solicitar = %d %v", code, b1)
	}
	code, b2 := call(t, h, r2, "POST", "/bookings", body(e, day(6), 6000))
	if code != 201 {
		t.Fatalf("segunda solicitud = %d %v", code, b2)
	}
	if code, _ := call(t, h, req, "POST", "/bookings", body(s, s, 3000)); code != 409 {
		t.Errorf("pendiente propia repetida = %d", code)
	}

	// CA-03 y CA-07: el precio cambia; el total viejo es quote_changed y la
	// solicitud existente conserva la tarifa.
	if code, out := call(t, h, own, "PATCH", "/items/"+itemID, `{"version":1,"pricePerDayCents":3500}`); code != 200 {
		t.Fatalf("cambiar precio = %d %v", code, out)
	}
	code, out := call(t, h, r3, "POST", "/bookings", body(day(10), day(10), 3000))
	if code != 409 || !strings.Contains(fmt.Sprint(out), "quote_changed") {
		t.Errorf("CA-03 = %d %v", code, out)
	}
	if _, got := call(t, h, req, "GET", "/bookings/"+b1["id"].(string), ""); got["priceSnapshot"].(map[string]any)["pricePerDayCents"] != float64(3000) {
		t.Errorf("CA-07 = %v", got["priceSnapshot"])
	}

	// Disponible antes de aceptar (una pendiente no bloquea la búsqueda).
	if !searchHas(t, h, r3, s, e, itemID) {
		t.Error("una pendiente no debe bloquear")
	}
	// Aceptar: la cruzada pasa a rejected; el periodo deja de estar disponible.
	code, acc := call(t, h, own, "POST", "/bookings/"+b1["id"].(string)+"/accept", `{"meetingPoint":{"description":"Metro Coyoacán"}}`)
	if code != 200 || acc["status"] != "accepted" || acc["viewerRole"] != "owner" {
		t.Fatalf("aceptar = %d %v", code, acc)
	}
	if _, got := call(t, h, r2, "GET", "/bookings/"+b2["id"].(string), ""); got["status"] != "rejected" || got["rejectReason"] != "no_disponible" {
		t.Errorf("cruzada = %v %v", got["status"], got["rejectReason"])
	}
	if searchHas(t, h, r3, s, e, itemID) {
		t.Error("aceptada: no debe aparecer en esas fechas")
	}
	if !searchHas(t, h, r3, day(8), day(9), itemID) {
		t.Error("otras fechas siguen disponibles")
	}
	// Un tercero no ve la reserva; RF-14: no se elimina el objeto con reserva.
	if code, _ := call(t, h, r3, "GET", "/bookings/"+b1["id"].(string), ""); code != 404 {
		t.Errorf("tercero = %d", code)
	}
	if code, _ := call(t, h, own, "DELETE", "/items/"+itemID, ""); code != 409 {
		t.Errorf("RF-14 = %d", code)
	}

	// CA-08: cancelar con motivo libera el periodo.
	if code, _ := call(t, h, req, "POST", "/bookings/"+b1["id"].(string)+"/cancel", ""); code != 400 {
		t.Errorf("sin motivo = %d", code)
	}
	code, can := call(t, h, req, "POST", "/bookings/"+b1["id"].(string)+"/cancel", `{"reason":"Cambio de planes"}`)
	if code != 200 || can["status"] != "cancelled" || can["cancelledBy"] != "requester" {
		t.Fatalf("cancelar = %d %v", code, can)
	}
	if !searchHas(t, h, r3, s, e, itemID) {
		t.Error("CA-08: debe volver a estar disponible")
	}
	code, page := call(t, h, own, "GET", "/me/bookings?role=owner", "")
	if code != 200 || len(page["items"].([]any)) != 2 {
		t.Errorf("mis reservas = %d %v", code, page)
	}
	if code, _ := call(t, h, own, "DELETE", "/items/"+itemID, ""); code != 204 {
		t.Errorf("eliminar sin reservas activas = %d", code)
	}
	want := []string{"booking.requested", "booking.requested", "booking.accepted", "booking.rejected", "booking.cancelled"}
	if fmt.Sprint(not.types) != fmt.Sprint(want) {
		t.Errorf("avisos = %v", not.types)
	}
}
