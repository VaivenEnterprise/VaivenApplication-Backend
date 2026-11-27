//go:build integration

package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/contract"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/handler"
	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	aimongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/adapter/mongo"
	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	assistantmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/adapter/mongo"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	authnotices "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/notices"
	authjobs "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/jobs"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	chatmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/mongo"
	chatdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	incidentsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/adapter/mongo"
	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
	notificationsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/mongo"
	notificationsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
	ratingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/adapter/mongo"
	ratingsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
	recomongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/adapter/mongo"
	recodomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	reportsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/adapter/mongo"
	sanctionsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/adapter/mongo"
)

// Spec 021, T023: la baja completa de punta a punta, con MongoDB (replica set) y
// Redis reales, los erasers reales de cada módulo, un Cloudinary falso y el
// router completo (cada respuesta se valida contra el contrato OpenAPI).

const (
	delPass  = "una-contrasena-larga"
	delPhone = "+525512345678"
)

// fakeCloudinary es el borrado de imágenes falso: puede estar caído.
type fakeCloudinary struct {
	mu        sync.Mutex
	down      bool
	destroyed []string
}

func (f *fakeCloudinary) Destroy(_ context.Context, r usecase.ImageRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return errors.New("cloudinary caído")
	}
	f.destroyed = append(f.destroyed, r.PublicID)
	return nil
}

func (f *fakeCloudinary) set(down bool) {
	f.mu.Lock()
	f.down = down
	f.mu.Unlock()
}

func (f *fakeCloudinary) list() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.destroyed)
	sort.Strings(out)
	return out
}

// toggleRevocations simula que Redis cae y vuelve para la revocación del caso de uso.
type toggleRevocations struct {
	real usecase.Revocations
	mu   sync.Mutex
	down bool
}

func (r *toggleRevocations) set(down bool) { r.mu.Lock(); r.down = down; r.mu.Unlock() }
func (r *toggleRevocations) isDown() bool  { r.mu.Lock(); defer r.mu.Unlock(); return r.down }

func (r *toggleRevocations) RevokeSession(ctx context.Context, sid string, ttl time.Duration) error {
	if r.isDown() {
		return errors.New("redis caído")
	}
	return r.real.RevokeSession(ctx, sid, ttl)
}

func (r *toggleRevocations) RevokeUserBefore(ctx context.Context, uid string, cutoff time.Time, ttl time.Duration) error {
	if r.isDown() {
		return errors.New("redis caído")
	}
	return r.real.RevokeUserBefore(ctx, uid, cutoff, ttl)
}

// hookEraser corre fn dentro de la transacción (entrelazado con Accept) o falla.
type hookEraser struct {
	name string
	fn   func() error
}

func (h hookEraser) Name() string { return h.name }
func (h hookEraser) Erase(context.Context, string, time.Time) (usecase.EraseResult, error) {
	return usecase.EraseResult{}, h.fn()
}

type delClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *delClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *delClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type delEnv struct {
	t        *testing.T
	db       *drv.Database
	api      *api
	svc      *usecase.Service
	rc       *redis.Client
	revs     *toggleRevocations
	cloud    *fakeCloudinary
	photoSeq int
	clk      *delClock
	bookings *bookingsmongo.Repo
	notifs   *notificationsmongo.NotificationRepo
	tokens   *notificationsmongo.TokenRepo
	users    *authmongo.UserRepo
}

func optionsSortID() *options.FindOptionsBuilder {
	return options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})
}
func optionsSortSeq() *options.FindOptionsBuilder {
	return options.Find().SetSort(bson.D{{Key: "seq", Value: 1}})
}

func newRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatalf("Redis: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := redis.NewClient(url, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rc.Close() })
	return rc
}

// newDelEnv arma la API como cmd/api: validadores en modo error, índices, los
// erasers reales de bookings, chat, ratings, notifications, ai, assistant y
// reco, la bandeja de salida y Redis real.
func newDelEnv(t *testing.T, rc *redis.Client, extra, last []usecase.Eraser) *delEnv {
	t.Helper()
	ctx := context.Background()
	db := flowClient.Database(fmt.Sprintf("del_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	// Modo ESTRICTO (T030: sin null, acción error), para probar que todo lo que
	// escribe la baja es válido.
	if _, err := adaptermongo.EnforceValidators(ctx, db, append(authmongo.ValidatorSpecs(), adaptermongo.OptionalFieldSpecs()...), true); err != nil {
		t.Fatal(err)
	}
	users, refresh, jobs := authmongo.NewUserRepo(db), authmongo.NewRefreshRepo(db), authmongo.NewDeletionJobs(db)
	bookingRepo := bookingsmongo.NewRepo(db)
	convs, msgs := chatmongo.NewConversationRepo(db), chatmongo.NewMessageRepo(db)
	notifs, tokens := notificationsmongo.NewNotificationRepo(db), notificationsmongo.NewTokenRepo(db)
	if err := adaptermongo.EnsureAll(ctx, users, refresh, jobs, bookingRepo, convs, msgs, ratingsmongo.NewRepo(db, users),
		notifs, tokens, aimongo.NewUsageRepo(db), assistantmongo.NewConversationRepo(db), recomongo.NewInteractionRepo(db)); err != nil {
		t.Fatal(err)
	}
	// Las transacciones necesitan las colecciones creadas de antemano.
	for _, c := range []string{bookingsmongo.BookingsCollection, bookingsmongo.ItemsCollection, recomongo.MetaCollection,
		incidentsmongo.IncidentsCollection, reportsmongo.ReportsCollection, sanctionsmongo.SanctionsCollection} {
		_ = db.CreateCollection(ctx, c)
	}

	jwt, err := security.NewJWT("clave-de-prueba-clave-de-prueba-0000", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := security.NewHasher(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	notifier, err := notify.NewService(notify.ServiceDeps{Store: notifs, Users: users, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	sender, err := authnotices.New(notifier)
	if err != nil {
		t.Fatal(err)
	}
	real := redis.NewRevocations(rc)
	env := &delEnv{
		t: t, db: db, rc: rc, revs: &toggleRevocations{real: real}, cloud: &fakeCloudinary{},
		clk: &delClock{t: time.Now().UTC()}, bookings: bookingRepo, notifs: notifs, tokens: tokens, users: users,
	}
	erasers := slices.Clone(extra)
	erasers = append(erasers,
		bookingsmongo.NewEraser(db), itemsmongo.NewEraser(db), chatmongo.NewEraser(db), ratingsmongo.NewEraser(db), notificationsmongo.NewEraser(db),
		aimongo.NewEraser(db), assistantmongo.NewEraser(db), recomongo.NewEraser(db),
		incidentsmongo.NewEraser(db), reportsmongo.NewEraser(db), sanctionsmongo.NewEraser(db))
	erasers = append(erasers, last...)
	svc, err := usecase.NewService(usecase.Deps{
		Users: users, Refresh: refresh, Hasher: hasher, Access: jwt,
		NewRefresh: security.NewRefreshToken, HashRefresh: security.HashToken, RefreshTTL: 720 * time.Hour,
		Bookings: bookingsmongo.NewUserActivity(db), Revocations: env.revs, Now: env.clk.Now,
		Tx: adaptermongo.NewTxRunner(db), Erasers: erasers, Jobs: jobs, Pending: users, Images: env.cloud, Notices: sender,
	})
	if err != nil {
		t.Fatal(err)
	}
	env.svc = svc
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	limiter := redis.NewMemoryLimiter()
	mod, err := NewModule(ModuleDeps{Service: svc, Tokens: jwt, Log: log, Limiter: limiter, LimitPerMinute: 1000, Revocations: real})
	if err != nil {
		t.Fatal(err)
	}
	router := apihttp.NewRouter(apihttp.Deps{
		Log: log, Pinger: flowClient, Meta: handler.MetaInfo{Name: "vaiven-backend"},
		Limiter: limiter, LimiterFallback: redis.NewMemoryLimiter(), Modules: []apihttp.Module{mod},
	})
	srv := httptest.NewServer(contract.Wrap(t, router, ""))
	t.Cleanup(srv.Close)
	env.api = &api{t: t, srv: srv}
	return env
}

type person struct {
	id, email, access, refresh string
}

func (e *delEnv) register(email, name string) person {
	e.t.Helper()
	var reg authBody
	body := fmt.Sprintf(`{"email":"%s","password":"%s","displayName":"%s","acceptTerms":true,"isAdult":true}`, email, delPass, name)
	if st, code := e.api.call(http.MethodPost, "/api/v1/auth/register", body, "", &reg); st != http.StatusCreated {
		e.t.Fatalf("registro de %s = %d %s", name, st, code)
	}
	return person{id: reg.User.ID, email: email, access: reg.Tokens.AccessToken, refresh: reg.Tokens.RefreshToken}
}

func (e *delEnv) login(p person) person {
	e.t.Helper()
	var s authBody
	if st, _ := e.api.call(http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"email":"%s","password":"%s"}`, p.email, delPass), "", &s); st != http.StatusOK {
		e.t.Fatalf("login de %s = %d", p.email, st)
	}
	p.access, p.refresh = s.Tokens.AccessToken, s.Tokens.RefreshToken
	return p
}

func (e *delEnv) deleteMe(p person, password string) (int, string) {
	return e.api.call(http.MethodDelete, "/api/v1/me", fmt.Sprintf(`{"password":"%s"}`, password), p.access, nil)
}

// item siembra un objeto con el modelo de la 005.
func (e *delEnv) item(owner string) string {
	e.t.Helper()
	e.photoSeq++ // cada objeto lleva una foto distinta: foto-1, foto-2…
	res, err := e.db.Collection(bookingsmongo.ItemsCollection).InsertOne(context.Background(), bson.M{
		"ownerId": owner, "status": "active", "title": "Taladro", "usageRules": "Limpio", "modality": "rent",
		"pricePerDayCents": int64(3000), "version": int64(1),
		"photos": bson.A{bson.M{"publicId": fmt.Sprintf("vaiven/test/foto-%d", e.photoSeq), "width": 10, "height": 20, "format": "jpg", "bytes": int64(5), "delivery": "public"}},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return res.InsertedID.(bson.ObjectID).Hex()
}

// request crea una solicitud pending de requester sobre itemID.
func (e *delEnv) request(itemID, requester string, fromDay, toDay int) bookings.Booking {
	e.t.Helper()
	ctx := context.Background()
	loc, err := bookings.Location()
	if err != nil {
		e.t.Fatal(err)
	}
	it, err := e.bookings.ItemForBooking(ctx, itemID)
	if err != nil {
		e.t.Fatal(err)
	}
	today := bookings.Today(e.clk.Now(), loc)
	q, err := bookings.NewQuote(bookings.QuoteInput{
		ItemID: itemID, Modality: it.Modality, PricePerDayCents: it.PricePerDayCents,
		Start: today.AddDays(fromDay).String(), End: today.AddDays(toDay).String(), PaymentMode: bookings.PaymentSplit5050, Today: today,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	b, err := bookings.NewRequest(e.bookings.NewID(), requester, it, q, e.clk.Now(), loc)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.bookings.Create(ctx, b); err != nil {
		e.t.Fatal(err)
	}
	return b
}

func (e *delEnv) accept(b bookings.Booking) bookings.Booking {
	e.t.Helper()
	if err := b.Transition(bookings.StatusAccepted, bookings.RoleOwner, e.clk.Now()); err != nil {
		e.t.Fatal(err)
	}
	acc, _, err := e.bookings.Accept(context.Background(), b, e.clk.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	return acc
}

type seeded struct {
	ana, beto, carla person
	phone            string
	passwordHash     string
	pendingOwned     bookings.Booking // Beto pide un objeto de Ana (Ana es dueña)
	pendingRequested bookings.Booking // Ana pide un objeto de Carla (Ana es solicitante)
	conv             string
}

// seed deja a Ana con datos en todas las colecciones: objetos, solicitudes
// pendientes, chat, calificaciones, avisos, token de push, IA y recomendaciones.
func (e *delEnv) seed() seeded {
	e.t.Helper()
	ctx := context.Background()
	s := seeded{ana: e.register("ana@example.com", "Ana"), beto: e.register("beto@example.com", "Beto"), carla: e.register("carla@example.com", "Carla"), phone: delPhone}
	s.ana = e.login(s.ana) // una segunda sesión: dos refresh tokens
	if st, code := e.api.call(http.MethodPatch, "/api/v1/me", `{"phone":"`+delPhone+`"}`, s.ana.access, nil); st != http.StatusOK {
		e.t.Fatalf("PATCH /me = %d %s", st, code)
	}
	oid, _ := bson.ObjectIDFromHex(s.ana.id)
	var u struct {
		PasswordHash string `bson:"passwordHash"`
	}
	if err := e.db.Collection(authmongo.UsersCollection).FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&u); err != nil {
		e.t.Fatal(err)
	}
	s.passwordHash = u.PasswordHash

	anaItem, anaItem2, carlaItem := e.item(s.ana.id), e.item(s.ana.id), e.item(s.carla.id)
	_ = anaItem2
	s.pendingOwned = e.request(anaItem, s.beto.id, 5, 6)
	s.pendingRequested = e.request(carlaItem, s.ana.id, 5, 6)

	// Chat Ana–Beto.
	convs, sender := chatmongo.NewConversationRepo(e.db), chatmongo.NewSender(e.db)
	conv, err := convs.Open(ctx, s.pendingOwned.ID, s.ana.id, s.beto.id, e.clk.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	s.conv = conv.ID
	for i, m := range []struct{ from, to, body string }{
		{s.ana.id, s.beto.id, "hola, mi correo es ana@example.com"}, {s.beto.id, s.ana.id, "hola Ana"}, {s.ana.id, s.beto.id, "nos vemos mañana"},
	} {
		at := e.clk.Now().Add(time.Duration(i) * time.Minute)
		if _, _, err := sender.Send(ctx, chatdomain.Message{ConversationID: conv.ID, SenderID: m.from, Body: m.body, CreatedAt: at},
			m.to, chatdomain.LastMessage{Preview: m.body, At: at, SenderID: m.from}); err != nil {
			e.t.Fatal(err)
		}
	}

	// Calificaciones: Ana a Beto (con comentario) y Carla a Ana.
	ratings := ratingsmongo.NewRepo(e.db, e.users)
	comment := "Muy amable, mi tel es " + delPhone
	if _, err := ratings.Create(ctx, ratingsdomain.Rating{ID: ratings.NewID(), BookingID: "r-b1", RaterID: s.ana.id, RateeID: s.beto.id,
		RatedRole: bookings.RoleRequester, Score: 5, Comment: &comment, CreatedAt: e.clk.Now().Truncate(time.Millisecond)}); err != nil {
		e.t.Fatal(err)
	}
	if _, err := ratings.Create(ctx, ratingsdomain.Rating{ID: ratings.NewID(), BookingID: "r-b2", RaterID: s.carla.id, RateeID: s.ana.id,
		RatedRole: bookings.RoleOwner, Score: 4, CreatedAt: e.clk.Now().Truncate(time.Millisecond)}); err != nil {
		e.t.Fatal(err)
	}

	// Avisos, token de push, IA y recomendaciones.
	for _, to := range []string{s.ana.id, s.ana.id, s.beto.id} {
		if err := e.notifs.Save(ctx, notify.Record{UserID: to, Type: "booking_requested", Title: "T", Body: "B", Data: map[string]string{"bookingId": "x"}, CreatedAt: e.clk.Now()}); err != nil {
			e.t.Fatal(err)
		}
	}
	if err := e.tokens.Upsert(ctx, notificationsdomain.PushToken{Token: "ExponentPushToken[ana]", UserID: s.ana.id, Platform: "ios", LastSeenAt: e.clk.Now(), CreatedAt: e.clk.Now()}); err != nil {
		e.t.Fatal(err)
	}
	if ok, err := aimongo.NewUsageRepo(e.db).Take(ctx, s.ana.id, "assistant", "2026-11-26", 20, e.clk.Now()); err != nil || !ok {
		e.t.Fatalf("Take = %v, %v", ok, err)
	}
	if _, err := assistantmongo.NewConversationRepo(e.db).Create(ctx, s.ana.id, []aidomain.Turn{{Role: "user", Text: "quiero pintar mi casa"}}, e.clk.Now()); err != nil {
		e.t.Fatal(err)
	}
	for _, item := range []string{"i1", "i2"} {
		if _, err := recomongo.NewInteractionRepo(e.db).Record(ctx, recodomain.Event{UserID: s.ana.id, ItemID: item, Type: recodomain.EventView, Weight: recodomain.WeightView, At: e.clk.Now()}); err != nil {
			e.t.Fatal(err)
		}
	}
	if err := recomongo.NewMetaRepo(e.db).SetReset(ctx, s.ana.id, e.clk.Now()); err != nil {
		e.t.Fatal(err)
	}
	return s
}

// snapshot guarda el contenido exacto de todas las colecciones (JSON canónico).
func snapshot(t *testing.T, db *drv.Database, skip ...string) map[string][]string {
	t.Helper()
	ctx := context.Background()
	names, err := db.ListCollectionNames(ctx, bson.D{{Key: "type", Value: "collection"}})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, n := range names {
		if strings.HasPrefix(n, "system.") || slices.Contains(skip, n) {
			continue
		}
		cur, err := db.Collection(n).Find(ctx, bson.D{}, optionsSortID())
		if err != nil {
			t.Fatal(err)
		}
		var raws []bson.Raw
		if err := cur.All(ctx, &raws); err != nil {
			t.Fatal(err)
		}
		docs := make([]string, 0, len(raws))
		for _, raw := range raws {
			js, err := bson.MarshalExtJSON(raw, true, false)
			if err != nil {
				t.Fatal(err)
			}
			docs = append(docs, string(js))
		}
		out[n] = docs
	}
	return out
}

func sameSnapshot(t *testing.T, before, after map[string][]string, what string) {
	t.Helper()
	if reflect.DeepEqual(before, after) {
		return
	}
	for c, b := range before {
		if !reflect.DeepEqual(b, after[c]) {
			t.Errorf("%s: la colección %s cambió (%d → %d documentos)", what, c, len(b), len(after[c]))
		}
	}
	for c := range after {
		if _, ok := before[c]; !ok {
			t.Errorf("%s: apareció la colección %s", what, c)
		}
	}
}

// docsContaining devuelve en qué colecciones aparece alguno de los textos.
func docsContaining(t *testing.T, db *drv.Database, needles ...string) []string {
	t.Helper()
	var found []string
	for col, docs := range snapshot(t, db) {
		for _, d := range docs {
			for _, n := range needles {
				if n != "" && strings.Contains(d, n) {
					found = append(found, col+" contiene "+n[:min(len(n), 8)]+"…")
				}
			}
		}
	}
	return found
}

// nullsAnywhere cuenta los valores null en todas las colecciones (CA-02).
func nullsAnywhere(t *testing.T, db *drv.Database) []string {
	t.Helper()
	ctx := context.Background()
	names, _ := db.ListCollectionNames(ctx, bson.D{{Key: "type", Value: "collection"}})
	var out []string
	var walk func(col string, v bson.RawValue, path string)
	walk = func(col string, v bson.RawValue, path string) {
		switch v.Type {
		case bson.TypeNull:
			out = append(out, col+"."+path)
		case bson.TypeEmbeddedDocument:
			els, _ := v.Document().Elements()
			for _, el := range els {
				walk(col, el.Value(), path+"."+el.Key())
			}
		case bson.TypeArray:
			vals, _ := v.Array().Values()
			for _, x := range vals {
				walk(col, x, path+"[]")
			}
		}
	}
	for _, n := range names {
		if strings.HasPrefix(n, "system.") {
			continue
		}
		cur, err := db.Collection(n).Find(ctx, bson.D{})
		if err != nil {
			t.Fatal(err)
		}
		var raws []bson.Raw
		if err := cur.All(ctx, &raws); err != nil {
			t.Fatal(err)
		}
		for _, raw := range raws {
			els, _ := raw.Elements()
			for _, el := range els {
				walk(n, el.Value(), el.Key())
			}
		}
	}
	return out
}

func (e *delEnv) rawUser(id string) bson.Raw {
	e.t.Helper()
	oid, _ := bson.ObjectIDFromHex(id)
	var raw bson.Raw
	if err := e.db.Collection(authmongo.UsersCollection).FindOne(context.Background(), bson.D{{Key: "_id", Value: oid}}).Decode(&raw); err != nil {
		e.t.Fatal(err)
	}
	return raw
}

func (e *delEnv) count(col, field, value string) int64 {
	e.t.Helper()
	n, err := e.db.Collection(col).CountDocuments(context.Background(), bson.D{{Key: field, Value: value}})
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *delEnv) revoked(userID string, issuedBefore time.Time) bool {
	e.t.Helper()
	r, err := redis.NewRevocations(e.rc).IsRevoked(context.Background(), userID, "jti-de-prueba", issuedBefore)
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}

func TestIntegrationDeleteAccountEndToEnd(t *testing.T) {
	rc := newRedis(t)
	ctx := context.Background()

	t.Run("baja completa, worker, reanudación y repetición", func(t *testing.T) { // CA-01, CA-02, CA-06, CA-10, CA-12, CA-13, CA-16
		e := newDelEnv(t, rc, nil, nil)
		s := e.seed()
		deactivatedBefore := e.clk.Now().Add(-time.Second) // un acceso emitido antes de la baja

		// Cloudinary y Redis caídos justo en la baja: la baja sigue válida (CA-06).
		e.cloud.set(true)
		e.revs.set(true)
		if st, code := e.deleteMe(s.ana, "incorrecta"); st != http.StatusForbidden {
			t.Fatalf("contraseña incorrecta = %d %s", st, code)
		}
		if st, code := e.deleteMe(s.ana, delPass); st != http.StatusNoContent {
			t.Fatalf("baja = %d %s", st, code)
		}

		// users: solo los campos de CA-13, más deletionPending mientras hay pendientes.
		raw := e.rawUser(s.ana.id)
		els, _ := raw.Elements()
		var got []string
		for _, el := range els {
			got = append(got, el.Key())
		}
		sort.Strings(got)
		want := []string{"_id", "acceptedTermsAt", "createdAt", "deactivatedAt", "deletionPending", "displayName", "email", "status", "updatedAt", "version"}
		if !slices.Equal(got, want) {
			t.Fatalf("campos de users = %v, want %v", got, want)
		}
		if raw.Lookup("email").StringValue() != "deleted+"+s.ana.id+"@vaiven.invalid" || raw.Lookup("displayName").StringValue() != "Cuenta eliminada" || raw.Lookup("status").StringValue() != "deactivated" {
			t.Errorf("users = %v", raw)
		}
		if e.count("refresh_tokens", "userId", s.ana.id) != 0 || e.count("push_tokens", "userId", s.ana.id) != 0 ||
			e.count("notifications", "userId", s.ana.id) != 0 || e.count("ai_usage", "userId", s.ana.id) != 0 ||
			e.count("ai_conversations", "userId", s.ana.id) != 0 || e.count("reco_interactions", "userId", s.ana.id) != 0 {
			t.Error("deben borrarse sesiones, push, avisos, IA y recomendaciones de Ana")
		}
		if n, _ := e.db.Collection("reco_states").CountDocuments(ctx, bson.D{{Key: "_id", Value: "reset:" + s.ana.id}}); n != 0 {
			t.Error("reco_states reset:<id> debe borrarse")
		}
		// Solicitudes pendientes cerradas con el motivo (CA-10); aún sin aviso (después del commit).
		for id, st := range map[string]string{s.pendingOwned.ID: "rejected", s.pendingRequested.ID: "cancelled"} {
			bo, _ := bson.ObjectIDFromHex(id)
			var b bson.Raw
			if err := e.db.Collection("bookings").FindOne(ctx, bson.D{{Key: "_id", Value: bo}}).Decode(&b); err != nil {
				t.Fatal(err)
			}
			if b.Lookup("status").StringValue() != st {
				t.Errorf("reserva %s = %s, want %s", id, b.Lookup("status").StringValue(), st)
			}
		}
		if e.count("notifications", "userId", s.beto.id) != 1 || e.count("notifications", "userId", s.carla.id) != 0 {
			t.Error("los avisos a las contrapartes salen DESPUÉS del commit, no en la transacción")
		}
		if e.count("deletion_jobs", "userId", s.ana.id) != 4 { // 2 avisos + 2 fotos
			t.Errorf("jobs encolados = %d, want 4", e.count("deletion_jobs", "userId", s.ana.id))
		}

		// Un reintento con el mismo token es idempotente y no repite efectos.
		before := snapshot(t, e.db, "deletion_jobs")
		if st, _ := e.deleteMe(s.ana, delPass); st != http.StatusUnauthorized && st != http.StatusNoContent {
			t.Errorf("reintento con el mismo token = %d", st)
		}
		if err := e.svc.Deactivate(ctx, s.ana.id, delPass); err != nil {
			t.Errorf("segunda baja del caso de uso = %v, want nil", err)
		}
		sameSnapshot(t, before, snapshot(t, e.db, "deletion_jobs"), "segunda baja")

		// Primera pasada del worker con Redis y Cloudinary aún caídos: los avisos
		// salen, las fotos se reprograman y la cuenta sigue marcada.
		rep, err := e.svc.ResumeDeletions(ctx)
		if err != nil || rep.JobsDone != 2 || rep.JobsRetried != 2 || rep.Cleared != 0 {
			t.Fatalf("pasada 1 = %+v, %v", rep, err)
		}
		if e.count("notifications", "userId", s.beto.id) != 2 || e.count("notifications", "userId", s.carla.id) != 1 {
			t.Error("las contrapartes deben recibir su aviso")
		}
		if !e.rawUser(s.ana.id).Lookup("deletionPending").Boolean() {
			t.Error("con pendientes la marca deletionPending se conserva")
		}
		if e.revoked(s.ana.id, deactivatedBefore) {
			t.Error("con Redis caído el acceso aún no está revocado")
		}

		// Redis y Cloudinary de vuelta: una pasada después de la espera termina todo.
		e.cloud.set(false)
		e.revs.set(false)
		e.clk.advance(2 * time.Minute)
		rep, err = e.svc.ResumeDeletions(ctx)
		if err != nil || rep.JobsDone != 2 || rep.Cleared != 1 {
			t.Fatalf("pasada 2 = %+v, %v", rep, err)
		}
		if !slices.Equal(e.cloud.list(), []string{"vaiven/test/foto-1", "vaiven/test/foto-2"}) {
			t.Errorf("fotos borradas en Cloudinary = %v", e.cloud.list())
		}
		if !e.revoked(s.ana.id, deactivatedBefore) {
			t.Error("tras la reanudación el acceso debe estar revocado en Redis")
		}
		if _, err := e.rawUser(s.ana.id).LookupErr("deletionPending"); err == nil {
			t.Error("al terminar se quita deletionPending")
		}
		if n := e.count("deletion_jobs", "userId", s.ana.id); n != 0 {
			t.Errorf("la bandeja debe quedar vacía, hay %d", n)
		}

		// Repetir o reanudar no cambia nada (CA-12).
		final := snapshot(t, e.db)
		for i := 0; i < 2; i++ {
			e.clk.advance(time.Hour)
			if rep, err := e.svc.ResumeDeletions(ctx); err != nil || rep != (usecase.PassReport{}) {
				t.Errorf("pasada repetida = %+v, %v", rep, err)
			}
		}
		sameSnapshot(t, final, snapshot(t, e.db), "pasadas repetidas")
		if len(e.cloud.list()) != 2 {
			t.Errorf("no debe repetir borrados: %v", e.cloud.list())
		}

		// CA-01: nada identificable de Ana en ninguna colección.
		if hits := docsContaining(t, e.db, "ana@example.com", s.phone, s.passwordHash); len(hits) != 0 {
			t.Errorf("quedan datos de Ana: %v", hits)
		}
		// CA-02: ningún null en ninguna colección.
		if n := nullsAnywhere(t, e.db); len(n) != 0 {
			t.Errorf("hay null en %v", n)
		}
		// Mensajes (D-1), extracto y calificaciones (D-3).
		var texts []string
		cur, _ := e.db.Collection("messages").Find(ctx, bson.D{}, optionsSortSeq())
		var msgs []struct {
			Sender string `bson:"senderId"`
			Body   string `bson:"body"`
		}
		_ = cur.All(ctx, &msgs)
		for _, m := range msgs {
			texts = append(texts, m.Body)
		}
		if !slices.Equal(texts, []string{"Mensaje eliminado", "hola Ana", "Mensaje eliminado"}) {
			t.Errorf("mensajes = %v", texts)
		}
		var rr []struct {
			Rater   string  `bson:"raterId"`
			Score   int     `bson:"score"`
			Comment *string `bson:"comment"`
		}
		cur, _ = e.db.Collection("ratings").Find(ctx, bson.D{})
		_ = cur.All(ctx, &rr)
		if len(rr) != 2 {
			t.Fatalf("calificaciones = %+v", rr)
		}
		for _, r := range rr {
			if r.Rater == s.ana.id && (r.Comment != nil || r.Score != 5) {
				t.Errorf("la calificación de Ana: comentario %v estrellas %d", r.Comment, r.Score)
			}
		}
		// Promedios ajenos intactos (CA-08): Beto sigue con su 5.
		var pub map[string]any
		if st, _ := e.api.call(http.MethodGet, "/api/v1/users/"+s.beto.id, "", s.carla.access, &pub); st != http.StatusOK || pub["ratingAverage"] != float64(5) || pub["ratingCount"] != float64(1) {
			t.Errorf("perfil de Beto = %d %v", st, pub)
		}
		// CA-13: el perfil de Ana, con 200 y el contrato de N-2.
		pub = nil
		if st, code := e.api.call(http.MethodGet, "/api/v1/users/"+s.ana.id, "", s.beto.access, &pub); st != http.StatusOK ||
			pub["displayName"] != "Cuenta eliminada" || pub["ratingAverage"] != nil || pub["ratingCount"] != float64(0) {
			t.Errorf("GET /users/{ana} = %d %s %v", st, code, pub)
		}
		// Ya no inicia sesión (CA-16: el correo original vuelve a registrarse).
		if st, _ := e.api.call(http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"email":"ana@example.com","password":"%s"}`, delPass), "", nil); st != http.StatusUnauthorized {
			t.Errorf("login tras la baja = %d", st)
		}
		if st, _ := e.api.call(http.MethodGet, "/api/v1/me", "", s.ana.access, nil); st != http.StatusUnauthorized {
			t.Errorf("token anterior = %d, want 401", st)
		}
		_ = e.register("ana@example.com", "Ana de nuevo")
	})

	t.Run("409 con una reserva aceptada no cambia nada", func(t *testing.T) { // CA-03
		e := newDelEnv(t, rc, nil, nil)
		s := e.seed()
		e.accept(s.pendingOwned)
		before := snapshot(t, e.db)
		if st, code := e.deleteMe(s.ana, delPass); st != http.StatusConflict || code != "conflict" {
			t.Fatalf("baja con reserva aceptada = %d %s, want 409 conflict", st, code)
		}
		sameSnapshot(t, before, snapshot(t, e.db), "409")
		if e.revoked(s.ana.id, e.clk.Now()) {
			t.Error("un 409 no revoca nada en Redis")
		}
	})

	t.Run("entrelazado con Accept: 409 y nada cambia", func(t *testing.T) { // CA-04
		var attempts int
		var e *delEnv
		var s seeded
		racer := hookEraser{name: "racer", fn: func() error {
			attempts++
			if attempts == 1 {
				// Ana acepta la solicitud de Beto justo después de la primera
				// comprobación, fuera de la transacción.
				e.accept(s.pendingOwned)
			}
			return nil
		}}
		e = newDelEnv(t, rc, []usecase.Eraser{racer}, nil)
		s = e.seed()
		if st, code := e.deleteMe(s.ana, delPass); st != http.StatusConflict {
			t.Fatalf("baja entrelazada = %d %s, want 409", st, code)
		}
		if attempts != 1 {
			// El choque con Accept repite la función; en el segundo intento la
			// comprobación de reservas activas ya ve la aceptada y ningún
			// eraser llega a correr.
			t.Errorf("el eraser entrelazado corrió %d veces, want 1", attempts)
		}
		u := e.rawUser(s.ana.id)
		if u.Lookup("status").StringValue() != "active" || u.Lookup("passwordHash").StringValue() != s.passwordHash {
			t.Errorf("la cuenta debe seguir activa y completa: %v", u)
		}
		if e.count("refresh_tokens", "userId", s.ana.id) != 2 || e.count("notifications", "userId", s.ana.id) != 2 || e.count("deletion_jobs", "userId", s.ana.id) != 0 {
			t.Error("la baja revertida no debe dejar cambios")
		}
		bo, _ := bson.ObjectIDFromHex(s.pendingOwned.ID)
		var b bson.Raw
		_ = e.db.Collection("bookings").FindOne(ctx, bson.D{{Key: "_id", Value: bo}}).Decode(&b)
		if b.Lookup("status").StringValue() != "accepted" {
			t.Errorf("la reserva debe quedar aceptada: %s", b.Lookup("status").StringValue())
		}
		rb, _ := bson.ObjectIDFromHex(s.pendingRequested.ID)
		var other bson.Raw
		_ = e.db.Collection("bookings").FindOne(ctx, bson.D{{Key: "_id", Value: rb}}).Decode(&other)
		if other.Lookup("status").StringValue() != "pending" {
			t.Errorf("la otra solicitud debe seguir pendiente: %s", other.Lookup("status").StringValue())
		}
	})

	t.Run("falla a media transacción: todo o nada", func(t *testing.T) { // CA-05
		boom := hookEraser{name: "falla", fn: func() error { return errors.New("falla simulada") }}
		e := newDelEnv(t, rc, nil, []usecase.Eraser{boom})
		s := e.seed()
		before := snapshot(t, e.db)
		if st, _ := e.deleteMe(s.ana, delPass); st != http.StatusInternalServerError {
			t.Fatalf("baja con falla = %d, want 500", st)
		}
		sameSnapshot(t, before, snapshot(t, e.db), "falla a media transacción")
		if u := e.rawUser(s.ana.id); u.Lookup("status").StringValue() != "active" {
			t.Error("la cuenta debe seguir activa y completa")
		}
		// Sigue pudiendo iniciar sesión.
		if st, _ := e.api.call(http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"email":"ana@example.com","password":"%s"}`, delPass), "", nil); st != http.StatusOK {
			t.Errorf("login tras la falla = %d", st)
		}
	})

	t.Run("el worker periódico completa una baja interrumpida", func(t *testing.T) { // N-1, CA-12
		e := newDelEnv(t, rc, nil, nil)
		s := e.seed()
		e.cloud.set(true)
		if st, code := e.deleteMe(s.ana, delPass); st != http.StatusNoContent {
			t.Fatalf("baja = %d %s", st, code)
		}
		e.cloud.set(false)
		e.clk.advance(2 * time.Minute)
		tick := make(chan time.Time)
		wctx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			authjobs.RunDeletionWorker(wctx, e.svc, tick, slog.New(slog.NewTextHandler(io.Discard, nil)))
			close(done)
		}()
		// La pasada de arranque ya hace el trabajo; se espera a que termine.
		deadline := time.Now().Add(20 * time.Second)
		for e.count("deletion_jobs", "userId", s.ana.id) != 0 && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
		}
		for e.rawUser(s.ana.id).Lookup("deletionPending").Type != 0 && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
		}
		cancel()
		<-done
		if n := e.count("deletion_jobs", "userId", s.ana.id); n != 0 {
			t.Errorf("la bandeja debe quedar vacía, hay %d", n)
		}
		if _, err := e.rawUser(s.ana.id).LookupErr("deletionPending"); err == nil {
			t.Error("el worker debe quitar deletionPending")
		}
		if len(e.cloud.list()) != 2 {
			t.Errorf("fotos borradas = %v", e.cloud.list())
		}
	})
}

// Medición de RNF-02 / R-01: latencia de DELETE /me con 10 000 mensajes. Si el
// p95 pasa de 2 s se abre la tarea de lotes (plan §4.6); el tope duro es el
// límite de 60 s de las transacciones de MongoDB.
func TestIntegrationDeleteAccountLatencyWith10kMessages(t *testing.T) {
	rc := newRedis(t)
	ctx := context.Background()
	const runs, messages = 5, 10000
	var durations []time.Duration
	for run := 0; run < runs; run++ {
		e := newDelEnv(t, rc, nil, nil)
		ana := e.register("ana@example.com", "Ana")
		beto := e.register("beto@example.com", "Beto")
		conv, err := chatmongo.NewConversationRepo(e.db).Open(ctx, fmt.Sprintf("b-%d", run), ana.id, beto.id, e.clk.Now())
		if err != nil {
			t.Fatal(err)
		}
		docs := make([]any, 0, messages)
		for i := 0; i < messages; i++ {
			sender := ana.id
			if i%2 == 1 {
				sender = beto.id
			}
			docs = append(docs, bson.M{"_id": bson.NewObjectID(), "conversationId": conv.ID, "senderId": sender, "seq": int64(i + 1),
				"body": fmt.Sprintf("mensaje número %d con texto de ejemplo", i), "createdAt": e.clk.Now().Add(time.Duration(i) * time.Second)})
		}
		if _, err := e.db.Collection("messages").InsertMany(ctx, docs); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if st, code := e.deleteMe(ana, delPass); st != http.StatusNoContent {
			t.Fatalf("baja = %d %s", st, code)
		}
		durations = append(durations, time.Since(start))
		n, _ := e.db.Collection("messages").CountDocuments(ctx, bson.D{{Key: "body", Value: "Mensaje eliminado"}})
		if n != messages/2 {
			t.Fatalf("mensajes reemplazados = %d, want %d", n, messages/2)
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[len(durations)-1] // con 5 muestras el p95 es la mayor
	t.Logf("DELETE /me con %d mensajes (%d de la cuenta): muestras %v, p95 = %v (objetivo orientativo < 2 s)", messages, messages/2, durations, p95)
	if p95 > 60*time.Second {
		t.Errorf("p95 = %v supera el límite de 60 s de las transacciones", p95)
	}
}

// Spec 021, T026 (D-4, CA-11): la baja fija purgeAt = baja + 12 meses en
// incidencias, reportes y sanciones de la cuenta (sin tocar el texto) y deja la
// evidencia como job de retención con fecha de purga: no mantiene deletionPending
// (la baja termina) y el worker la destruye al llegar la fecha.
func TestIntegrationDeleteAccountRetention(t *testing.T) {
	rc := newRedis(t)
	ctx := context.Background()
	e := newDelEnv(t, rc, nil, nil)
	ana := e.register("ana@example.com", "Ana")
	beto := e.register("beto@example.com", "Beto")
	carla := e.register("carla@example.com", "Carla")

	ins := func(col string, doc bson.M) {
		if _, err := e.db.Collection(col).InsertOne(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	evidence := bson.A{bson.M{"publicId": "vaiven/test/evidencia-1", "width": 10, "height": 20, "format": "jpg", "bytes": int64(5), "delivery": "authenticated"}}
	ins("incidents", bson.M{"bookingId": "b1", "reporterId": ana.id, "reportedId": beto.id, "reason": "damaged", "description": "Texto de Ana.", "evidence": evidence, "status": "open", "notes": bson.A{}, "version": int64(1)})
	ins("incidents", bson.M{"bookingId": "b2", "reporterId": beto.id, "reportedId": carla.id, "reason": "damaged", "description": "Otra.", "evidence": evidence, "status": "open", "notes": bson.A{}, "version": int64(1)})
	ins("reports", bson.M{"reporterId": beto.id, "targetType": "user", "targetId": ana.id, "reason": "prohibited", "details": "Detalle.", "status": "open"})
	ins("sanctions", bson.M{"userId": ana.id, "type": "warning", "reason": "Llegó tarde.", "createdByAdminId": "adm"})

	deletedAt := e.clk.Now().UTC().Truncate(time.Millisecond)
	purge := deletedAt.AddDate(0, 12, 0)
	if st, code := e.deleteMe(ana, delPass); st != http.StatusNoContent {
		t.Fatalf("baja = %d %s", st, code)
	}
	purgeAt := func(col, field, value string) (time.Time, bool) {
		var raw bson.Raw
		if err := e.db.Collection(col).FindOne(ctx, bson.D{{Key: field, Value: value}}).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		v, err := raw.LookupErr("purgeAt")
		if err != nil {
			return time.Time{}, false
		}
		return v.Time().UTC(), true
	}
	for _, c := range []struct{ col, field, value string }{{"incidents", "bookingId", "b1"}, {"reports", "targetId", ana.id}, {"sanctions", "userId", ana.id}} {
		if p, ok := purgeAt(c.col, c.field, c.value); !ok || !p.Equal(purge) {
			t.Errorf("%s purgeAt = %v (%v), want %v", c.col, p, ok, purge)
		}
	}
	if _, ok := purgeAt("incidents", "bookingId", "b2"); ok {
		t.Error("una incidencia ajena no debe tener purgeAt")
	}
	var inc struct {
		Description string `bson:"description"`
	}
	_ = e.db.Collection("incidents").FindOne(ctx, bson.D{{Key: "bookingId", Value: "b1"}}).Decode(&inc)
	if inc.Description != "Texto de Ana." {
		t.Error("el texto de la incidencia no cambia antes de purgar (N-3)")
	}

	// La evidencia queda encolada con fecha de purga y como job de retención.
	var job struct {
		Next      time.Time `bson:"nextAttemptAt"`
		Retention bool      `bson:"retention"`
	}
	if n := e.count("deletion_jobs", "userId", ana.id); n != 1 {
		t.Fatalf("jobs encolados = %d, want 1 (la evidencia)", n)
	}
	if err := e.db.Collection("deletion_jobs").FindOne(ctx, bson.D{{Key: "userId", Value: ana.id}}).Decode(&job); err != nil || !job.Next.UTC().Equal(purge) || !job.Retention {
		t.Errorf("job de la evidencia = %+v, %v; quiero nextAttemptAt = %v", job, err, purge)
	}

	// La baja termina aunque la evidencia espere 12 meses: la reanudación en línea
	// ya quitó deletionPending y el worker no tiene nada que hacer.
	if _, err := e.rawUser(ana.id).LookupErr("deletionPending"); err == nil {
		t.Error("un job de retención no debe mantener deletionPending")
	}
	rep, err := e.svc.ResumeDeletions(ctx)
	if err != nil || rep != (usecase.PassReport{}) {
		t.Fatalf("pasada = %+v, %v", rep, err)
	}
	if len(e.cloud.list()) != 0 {
		t.Errorf("la evidencia no se borra antes de su fecha: %v", e.cloud.list())
	}
	if n := nullsAnywhere(t, e.db); len(n) != 0 {
		t.Errorf("hay null en %v", n)
	}

	// Pasados los 12 meses el worker destruye la evidencia.
	e.clk.advance(purge.Sub(e.clk.Now()) + time.Minute)
	if rep, err := e.svc.ResumeDeletions(ctx); err != nil || rep.JobsDone != 1 {
		t.Fatalf("pasada tras 12 meses = %+v, %v", rep, err)
	}
	if !slices.Equal(e.cloud.list(), []string{"vaiven/test/evidencia-1"}) {
		t.Errorf("evidencia destruida = %v", e.cloud.list())
	}
	if n := e.count("deletion_jobs", "userId", ana.id); n != 0 {
		t.Errorf("la bandeja debe quedar vacía, hay %d", n)
	}
}
