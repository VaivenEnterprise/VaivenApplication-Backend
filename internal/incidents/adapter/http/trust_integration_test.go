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

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	authhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/http"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	authdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	incidentsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/adapter/mongo"
	incidentsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	ratingshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/adapter/http"
	ratingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/adapter/mongo"
	ratingsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/usecase"
	sanctionshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/adapter/http"
	sanctionsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/adapter/mongo"
	sanctionsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/usecase"
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

// itAuth: X-User identifica y X-Role fija el rol, como lo haría Authenticate.
func itAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-User")
		if id == "" {
			respond.Error(w, r, shared.NewUnauthorized("sin sesión"))
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), auth.Identity{UserID: id, Role: r.Header.Get("X-Role")})))
	})
}

type trust struct {
	t     *testing.T
	h     http.Handler
	books *bookingsmongo.Repo
	users *authmongo.UserRepo
	not   *itNotifier
	clock *time.Time
	loc   *time.Location

	owner, requester, admin string
	insertUser              func(name, role string) string
	profile                 func(id string) (sum, count int64)
}

func newTrust(t *testing.T) *trust {
	t.Helper()
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("trust_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	userRepo := authmongo.NewUserRepo(db)
	bookRepo := bookingsmongo.NewRepo(db)
	ratingsRepo := ratingsmongo.NewRepo(db, userRepo)
	incRepo := incidentsmongo.NewRepo(db)
	sancRepo := sanctionsmongo.NewRepo(db)
	for _, e := range []interface{ EnsureIndexes(context.Context) error }{userRepo, bookRepo, ratingsRepo, incRepo, sancRepo} {
		if err := e.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
	}
	loc, err := bookings.Location()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	tr := &trust{t: t, books: bookRepo, users: userRepo, not: &itNotifier{}, clock: &now, loc: loc}
	clock := func() time.Time { return *tr.clock }
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	tr.insertUser = func(name, role string) string {
		oid := bson.NewObjectID()
		at := time.Now().UTC()
		if _, err := db.Collection(authmongo.UsersCollection).InsertOne(ctx, bson.M{
			"_id": oid, "email": oid.Hex() + "@example.test", "displayName": name, "status": "active", "role": role,
			"ratingSum": int64(0), "ratingCount": int64(0), "createdAt": at, "updatedAt": at, "version": int64(1),
		}); err != nil {
			t.Fatal(err)
		}
		return oid.Hex()
	}
	tr.profile = func(id string) (int64, int64) {
		var d struct {
			Sum   int64 `bson:"ratingSum"`
			Count int64 `bson:"ratingCount"`
		}
		oid, _ := bson.ObjectIDFromHex(id)
		if err := db.Collection(authmongo.UsersCollection).FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
			t.Fatal(err)
		}
		return d.Sum, d.Count
	}
	tr.owner, tr.requester, tr.admin = tr.insertUser("Beto", "user"), tr.insertUser("Ana", "user"), tr.insertUser("Admin", "admin")

	ratingsSvc, err := ratingsusecase.NewService(ratingsusecase.Deps{Repo: ratingsRepo, Bookings: bookRepo, Users: userRepo, Notifier: tr.not, Log: quiet, Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	ratingsMod, err := ratingshttp.NewModule(ratingshttp.ModuleDeps{Service: ratingsSvc, Authenticate: itAuth, Log: quiet})
	if err != nil {
		t.Fatal(err)
	}
	incSvc, err := incidentsusecase.NewService(incidentsusecase.Deps{
		Repo: incRepo, Bookings: bookRepo, Images: itImages{}, Users: userRepo, Notifier: tr.not, Log: quiet, Now: clock, Location: loc,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireAdmin := authhttp.RequireRole(authdomain.RoleAdmin)
	incMod, err := NewModule(ModuleDeps{Service: incSvc, Authenticate: itAuth, RequireAdmin: requireAdmin, Log: quiet})
	if err != nil {
		t.Fatal(err)
	}
	sancSvc, err := sanctionsusecase.NewService(sanctionsusecase.Deps{
		Repo: sancRepo, Incidents: incRepo, Directory: userRepo, Admin: userRepo, Notifier: tr.not, Log: quiet, Now: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	sancMod, err := sanctionshttp.NewModule(sanctionshttp.ModuleDeps{Service: sancSvc, Authenticate: itAuth, RequireAdmin: requireAdmin, Log: quiet})
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	ratingsMod.Register(r)
	incMod.Register(r)
	sancMod.Register(r)
	// Sonda de «crear objeto»: las rutas que crean objetos y solicitudes montan
	// RequireActive igual (spec 003, RF-13), así que una cuenta suspendida
	// recibe aquí el mismo 403 que recibiría en POST /items.
	r.With(itAuth, authhttp.RequireActive(userRepo.ByID, clock)).Post("/items-probe", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	tr.h = r
	return tr
}

func (tr *trust) call(user, role, method, path, body string) (int, map[string]any) {
	tr.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-User", user)
		req.Header.Set("X-Role", role)
	}
	rec := httptest.NewRecorder()
	tr.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (tr *trust) expect(want int, user, role, method, path, body string) map[string]any {
	tr.t.Helper()
	code, out := tr.call(user, role, method, path, body)
	if code != want {
		tr.t.Fatalf("%s %s como %q = %d, quiero %d: %v", method, path, role, code, want, out)
	}
	return out
}

// seed guarda una reserva del dueño y del solicitante con el estado dado; si es
// completed, se completó hace completedAgo.
func (tr *trust) seed(status bookings.Status, completedAgo time.Duration) string {
	tr.t.Helper()
	today := bookings.Today(*tr.clock, tr.loc)
	start := today.AddDays(-2)
	hist := []bookings.HistoryEntry{{Status: status, At: *tr.clock, By: bookings.RoleOwner}}
	if status == bookings.StatusCompleted {
		hist = []bookings.HistoryEntry{{Status: bookings.StatusCompleted, At: tr.clock.Add(-completedAgo), By: bookings.RoleSystem}}
	}
	b := bookings.Booking{
		ID: tr.books.NewID(), ItemID: "item1", OwnerID: tr.owner, RequesterID: tr.requester,
		Start: start, End: today.AddDays(-1), Days: 2,
		Item:        bookings.ItemSnapshot{Title: "Taladro", Modality: bookings.ModalityRent},
		Price:       bookings.PriceSnapshot{Modality: bookings.ModalityRent, PricePerDayCents: 3000, TotalCents: 6000, Currency: "MXN"},
		PaymentMode: bookings.PaymentFullOnDelivery, Status: status,
		Payments:  []bookings.BookingPayment{{Stage: bookings.StageDelivery, AmountCents: 6000, Status: bookings.PaymentPending}},
		History:   hist,
		CreatedAt: *tr.clock, UpdatedAt: *tr.clock, Version: 1,
	}
	if err := tr.books.Create(context.Background(), b); err != nil {
		tr.t.Fatal(err)
	}
	return b.ID
}

const incidentBody = `{"reason":"damaged","description":"El objeto regresó con la tapa rota.","evidence":[{"publicId":"vaiven/test/evidence/1"}]}`

func TestIntegrationRatingsFlow(t *testing.T) { // CA-01, CA-02, CA-03
	tr := newTrust(t)
	done := tr.seed(bookings.StatusCompleted, 24*time.Hour)
	open := tr.seed(bookings.StatusInProgress, 0)
	old := tr.seed(bookings.StatusCompleted, 15*24*time.Hour)

	// CA-02: solo completadas y dentro de la ventana de 14 días.
	tr.expect(409, tr.requester, "user", "POST", "/bookings/"+open+"/ratings", `{"score":5}`)
	tr.expect(409, tr.requester, "user", "POST", "/bookings/"+old+"/ratings", `{"score":5}`)
	tr.expect(404, tr.admin, "admin", "POST", "/bookings/"+done+"/ratings", `{"score":5}`) // no es parte
	tr.expect(400, tr.requester, "user", "POST", "/bookings/"+done+"/ratings", `{"score":6}`)

	st := tr.expect(200, tr.requester, "user", "GET", "/bookings/"+done+"/rating-status", "")
	if st["canRate"] != true || st["myRating"] != nil || st["expiresAt"] == nil {
		t.Fatalf("estado previo = %v", st)
	}

	// CA-01: califica; el promedio y la cantidad del perfil de la otra parte se actualizan.
	r := tr.expect(201, tr.requester, "user", "POST", "/bookings/"+done+"/ratings", `{"score":4,"comment":"Todo bien"}`)
	if r["ratedRole"] != "owner" || r["score"] != float64(4) {
		t.Fatalf("calificación = %v", r)
	}
	if sum, count := tr.profile(tr.owner); sum != 4 || count != 1 {
		t.Errorf("CA-01 perfil del dueño: suma=%d cantidad=%d, quiero 4 y 1", sum, count)
	}
	if tr.not.count("rating.received:"+tr.owner) != 1 {
		t.Errorf("avisos = %v", tr.not.sent)
	}
	st = tr.expect(200, tr.requester, "user", "GET", "/bookings/"+done+"/rating-status", "")
	if st["canRate"] != false || st["myRating"] == nil {
		t.Errorf("estado posterior = %v", st)
	}

	// CA-03: una segunda calificación propia es 409 y no cambia el perfil.
	tr.expect(409, tr.requester, "user", "POST", "/bookings/"+done+"/ratings", `{"score":1}`)
	if sum, count := tr.profile(tr.owner); sum != 4 || count != 1 {
		t.Errorf("CA-03 perfil tras duplicado: suma=%d cantidad=%d", sum, count)
	}
	// La otra parte sí puede calificar, y se ve en el listado de la persona calificada.
	tr.expect(201, tr.owner, "user", "POST", "/bookings/"+done+"/ratings", `{"score":5}`)
	list := tr.expect(200, tr.owner, "user", "GET", "/users/"+tr.owner+"/ratings", "")
	if items, _ := list["items"].([]any); len(items) != 1 {
		t.Errorf("calificaciones del dueño = %v", list)
	}

	// RNF-01: dos calificaciones iguales a la vez → solo una se guarda.
	other := tr.seed(bookings.StatusCompleted, time.Hour)
	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i], _ = tr.call(tr.requester, "user", "POST", "/bookings/"+other+"/ratings", `{"score":3}`)
		}()
	}
	wg.Wait()
	created := 0
	for _, c := range codes {
		if c == 201 {
			created++
		} else if c != 409 {
			t.Errorf("código inesperado %d", c)
		}
	}
	if created != 1 {
		t.Errorf("RNF-01: %d calificaciones guardadas, quiero 1 (%v)", created, codes)
	}
}

func TestIntegrationIncidentLifecycleAndSanction(t *testing.T) { // CA-04…CA-08
	tr := newTrust(t)
	b := tr.seed(bookings.StatusInProgress, 0)

	// CA-04: abrir no cambia la reserva y avisa a la otra parte.
	inc := tr.expect(201, tr.requester, "user", "POST", "/bookings/"+b+"/incidents", incidentBody)
	id, _ := inc["id"].(string)
	if inc["status"] != "open" || id == "" {
		t.Fatalf("incidencia = %v", inc)
	}
	if ev, _ := inc["evidence"].([]any); len(ev) != 1 || !strings.Contains(ev[0].(map[string]any)["url"].(string), "sig=") {
		t.Errorf("evidencia sin URL firmada: %v", inc["evidence"])
	}
	book, err := tr.books.ByID(context.Background(), b)
	if err != nil || book.Status != bookings.StatusInProgress {
		t.Errorf("CA-04 la reserva debe seguir en curso: %v (%v)", book.Status, err)
	}
	if tr.not.count("incident.opened:"+tr.owner) != 1 {
		t.Errorf("avisos = %v", tr.not.sent)
	}
	tr.expect(404, tr.admin, "user", "GET", "/incidents/"+id, "") // ajeno

	// CA-05: /admin/* solo para administradores.
	tr.expect(401, "", "", "GET", "/admin/incidents", "")
	tr.expect(403, tr.requester, "user", "GET", "/admin/incidents", "")
	tr.expect(403, tr.requester, "user", "POST", "/admin/incidents/"+id+"/close", `{"finalResponse":"Respuesta final.","outcome":"none"}`)
	tr.expect(403, tr.requester, "user", "POST", "/admin/users/"+tr.requester+"/sanctions", `{"type":"warning","reason":"Motivo válido."}`)

	// Administración: cola de abiertas, nota interna y nota visible.
	q := tr.expect(200, tr.admin, "admin", "GET", "/admin/incidents", "")
	if items, _ := q["items"].([]any); len(items) != 1 {
		t.Fatalf("cola de abiertas = %v", q)
	}
	tr.expect(200, tr.admin, "admin", "POST", "/admin/incidents/"+id+"/notes", `{"text":"Revisar historial.","visibleToParties":false}`)
	tr.expect(200, tr.admin, "admin", "POST", "/admin/incidents/"+id+"/notes", `{"text":"Envía otra foto, por favor.","visibleToParties":true}`)

	// CA-07: las partes solo ven las notas visibles.
	mine := tr.expect(200, tr.owner, "user", "GET", "/incidents/"+id, "")
	if notes, _ := mine["notes"].([]any); len(notes) != 1 {
		t.Errorf("CA-07 el dueño debe ver 1 nota: %v", mine["notes"])
	}
	adm := tr.expect(200, tr.admin, "admin", "GET", "/incidents/"+id, "")
	if notes, _ := adm["notes"].([]any); len(notes) != 2 {
		t.Errorf("el admin ve las 2 notas: %v", adm["notes"])
	}

	// Cierre con respuesta final visible a ambas partes.
	tr.expect(200, tr.admin, "admin", "POST", "/admin/incidents/"+id+"/close", `{"finalResponse":"Se acordó un ajuste entre las partes.","outcome":"monetary_adjustment"}`)
	tr.expect(409, tr.admin, "admin", "POST", "/admin/incidents/"+id+"/close", `{"finalResponse":"Otra vez.","outcome":"none"}`)
	tr.expect(409, tr.admin, "admin", "POST", "/admin/incidents/"+id+"/notes", `{"text":"Tarde.","visibleToParties":true}`)
	for _, who := range []string{tr.requester, tr.owner} {
		got := tr.expect(200, who, "user", "GET", "/incidents/"+id, "")
		fr, _ := got["finalResponse"].(map[string]any)
		if got["status"] != "closed" || fr["text"] == nil || fr["outcome"] != "monetary_adjustment" {
			t.Errorf("CA-07 %s ve %v", who, got)
		}
	}
	if tr.not.count("incident.closed:"+tr.owner) != 1 || tr.not.count("incident.closed:"+tr.requester) != 1 {
		t.Errorf("avisos de cierre = %v", tr.not.sent)
	}

	// CA-08: la 4.ª incidencia abierta de la misma reserva se rechaza.
	for n := 1; n <= 3; n++ {
		tr.expect(201, tr.requester, "user", "POST", "/bookings/"+b+"/incidents", incidentBody)
	}
	tr.expect(409, tr.requester, "user", "POST", "/bookings/"+b+"/incidents", incidentBody)
	mineList := tr.expect(200, tr.requester, "user", "GET", "/me/incidents?status=open", "")
	if items, _ := mineList["items"].([]any); len(items) != 3 {
		t.Errorf("abiertas propias = %d, quiero 3", len(items))
	}

	// CA-06: una suspensión impide crear objetos hasta que termina; una
	// advertencia no. Las calificaciones no cambian.
	tr.expect(201, tr.owner, "user", "POST", "/items-probe", "")
	tr.expect(201, tr.admin, "admin", "POST", "/admin/users/"+tr.owner+"/sanctions", `{"type":"warning","reason":"Primera advertencia.","incidentId":"`+id+`"}`)
	tr.expect(201, tr.owner, "user", "POST", "/items-probe", "")
	ends := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	s := tr.expect(201, tr.admin, "admin", "POST", "/admin/users/"+tr.owner+"/sanctions", `{"type":"suspension","reason":"Daño reiterado.","endsAt":"`+ends+`"}`)
	if s["type"] != "suspension" || s["endsAt"] == nil {
		t.Errorf("sanción = %v", s)
	}
	tr.expect(403, tr.owner, "user", "POST", "/items-probe", "")
	tr.expect(201, tr.requester, "user", "POST", "/items-probe", "")
	if sum, count := tr.profile(tr.owner); sum != 0 || count != 0 {
		t.Errorf("las sanciones no tocan las calificaciones: suma=%d cantidad=%d", sum, count)
	}
	// Datos inválidos de la sanción.
	tr.expect(400, tr.admin, "admin", "POST", "/admin/users/"+tr.owner+"/sanctions", `{"type":"suspension","reason":"Sin fecha."}`)
	tr.expect(404, tr.admin, "admin", "POST", "/admin/users/"+bson.NewObjectID().Hex()+"/sanctions", `{"type":"warning","reason":"Persona inexistente."}`)
	tr.expect(400, tr.admin, "admin", "POST", "/admin/users/"+tr.owner+"/sanctions", `{"type":"warning","reason":"Incidencia inexistente.","incidentId":"`+bson.NewObjectID().Hex()+`"}`)
}

func TestIntegrationIncidentWindowAfterCompletion(t *testing.T) { // POL-08: 72 h tras completarse
	tr := newTrust(t)
	recent := tr.seed(bookings.StatusCompleted, 24*time.Hour)
	stale := tr.seed(bookings.StatusCompleted, 73*time.Hour)
	tr.expect(201, tr.owner, "user", "POST", "/bookings/"+recent+"/incidents", incidentBody)
	tr.expect(409, tr.owner, "user", "POST", "/bookings/"+stale+"/incidents", incidentBody)
}
