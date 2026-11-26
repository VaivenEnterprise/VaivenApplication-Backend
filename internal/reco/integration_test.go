//go:build integration

// Prueba de integración de punta a punta de la spec 015 (T009): las rutas
// reales con JWT, el caso de uso real, los repositorios sobre MongoDB 7, la
// búsqueda de la spec 006 para las tarjetas y el límite de tasa en Redis 7.
// Cubre CA-01 a CA-09 y mide el p95 de GET /recommendations (RNF-01). Cada
// prueba usa su propia base y la borra al terminar.
package reco_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/contract"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/handler"
	authhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
	recohttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/adapter/http"
	recomongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/adapter/mongo"
	recosearch "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/adapter/search"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain/synth"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/usecase"
	searchmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/adapter/mongo"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

const tools = "herramientas_electricas"

type world struct {
	t     *testing.T
	db    *drv.Database
	api   *httptest.Server
	svc   *usecase.Service
	jwt   *security.JWT
	logs  *syncBuffer
	users map[string]string
	items map[string]string
}

// syncBuffer: el recálculo perezoso escribe en el registro desde otra gorrutina.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

var (
	sharedOnce  sync.Once
	sharedMongo *mongodb.Client
	sharedRedis *redis.Client
	sharedErr   error
	stopShared  []func()
)

func TestMain(m *testing.M) {
	code := m.Run()
	for _, f := range stopShared {
		f()
	}
	os.Exit(code)
}

// containers arranca MongoDB y Redis una sola vez para todas las pruebas.
func containers(t *testing.T) (*mongodb.Client, *redis.Client) {
	t.Helper()
	sharedOnce.Do(func() {
		ctx := context.Background()
		mc, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
		if err != nil {
			sharedErr = fmt.Errorf("no se pudo iniciar MongoDB con Docker o Podman: %w", err)
			return
		}
		stopShared = append(stopShared, func() { _ = mc.Terminate(context.Background()) })
		uri, err := mc.ConnectionString(ctx)
		if err == nil {
			sharedMongo, err = mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
		}
		if err != nil {
			sharedErr = err
			return
		}
		stopShared = append(stopShared, func() { _ = sharedMongo.Disconnect(context.Background()) })
		rcn, err := tcredis.Run(ctx, "redis:7")
		if err != nil {
			sharedErr = fmt.Errorf("no se pudo iniciar Redis con Docker o Podman: %w", err)
			return
		}
		stopShared = append(stopShared, func() { _ = rcn.Terminate(context.Background()) })
		rurl, err := rcn.ConnectionString(ctx)
		if err == nil {
			sharedRedis, err = redis.NewClient(rurl, "test")
		}
		if err != nil {
			sharedErr = err
			return
		}
		stopShared = append(stopShared, func() { _ = sharedRedis.Close() })
	})
	if sharedErr != nil {
		t.Fatal(sharedErr)
	}
	return sharedMongo, sharedRedis
}

func newWorld(t *testing.T, enabled bool) *world {
	t.Helper()
	client, rc := containers(t)
	ctx := context.Background()
	db := client.Database(fmt.Sprintf("reco_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	if err := itemsmongo.NewItemRepo(db).EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := searchmongo.NewIndexes(db).EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	interactions := recomongo.NewInteractionRepo(db)
	if err := interactions.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	imgs, err := images.New(images.Config{CloudName: "nube", APIKey: "k", APISecret: "s", BaseFolder: "vaiven/test", MaxBytes: imagesdomain.DefaultMaxBytes})
	if err != nil {
		t.Fatal(err)
	}
	search, err := searchusecase.NewService(searchusecase.Deps{Repo: searchmongo.NewRepo(db, nil), Images: imgs})
	if err != nil {
		t.Fatal(err)
	}
	cards, err := recosearch.New(search)
	if err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, db: db, logs: &syncBuffer{}, users: map[string]string{}, items: map[string]string{}}
	log := slog.New(slog.NewJSONHandler(w.logs, nil))
	w.svc, err = usecase.NewService(usecase.Deps{
		Interactions: interactions, Requests: recomongo.NewRequestSource(db), Items: recomongo.NewItemSource(db),
		Model: recomongo.NewModelRepo(db), Meta: recomongo.NewMetaRepo(db), Cards: cards, Log: log, Enabled: enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.jwt, err = security.NewJWT("clave-de-prueba-clave-de-prueba-0000", time.Minute); err != nil {
		t.Fatal(err)
	}
	m, err := recohttp.NewModule(recohttp.ModuleDeps{
		Service: w.svc, Authenticate: authhttp.Authenticate(w.jwt), Log: log,
		Limiter: redis.NewLimiter(rc), LimiterFallback: redis.NewMemoryLimiter(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Spec 016, T008: cada respuesta se valida contra el contrato OpenAPI.
	w.api = httptest.NewServer(contract.Wrap(t, apihttp.NewRouter(apihttp.Deps{
		Log: log, Modules: []apihttp.Module{m},
		Meta: handler.MetaInfo{Name: "vaiven", Version: "test", Environment: "test", Features: handler.MetaFeatures{Recommendations: enabled}},
	}), ""))
	t.Cleanup(w.api.Close)
	w.seed()
	return w
}

func (w *world) user(name, status string, suspendedUntil *time.Time) string {
	res, err := w.db.Collection(searchmongo.UsersCollection).InsertOne(context.Background(), bson.M{
		"email": name + "@vaiven.invalid", "displayName": name, "status": status,
		"suspendedUntil": suspendedUntil, "ratingSum": int64(0), "ratingCount": int64(0),
	})
	if err != nil {
		w.t.Fatal(err)
	}
	w.users[name] = res.InsertedID.(bson.ObjectID).Hex()
	return w.users[name]
}

func (w *world) item(key, owner, category, municipality, status string) {
	res, err := w.db.Collection(searchmongo.ItemsCollection).InsertOne(context.Background(), bson.M{
		"ownerId": owner, "title": "Objeto " + key, "description": "En buen estado",
		"category": category, "condition": "good", "modality": "rent",
		"declaredValueCents": int64(100000), "pricePerDayCents": int64(8000), "priceSource": "manual",
		"zone":   bson.M{"municipality": municipality, "neighborhood": nil},
		"photos": bson.A{}, "status": status, "createdAt": time.Now().UTC(), "version": int64(1),
	})
	if err != nil {
		w.t.Fatal(err)
	}
	w.items[key] = res.InsertedID.(bson.ObjectID).Hex()
}

// seed: herramientas T1–T6 y objetos de hogar H1–H4 de ana y beto en Tula y
// Pachuca, más los que nunca deben recomendarse.
func (w *world) seed() {
	future := time.Now().Add(30 * 24 * time.Hour)
	ana, beto := w.user("ana", "active", nil), w.user("beto", "active", nil)
	yo := w.user("yo", "active", nil)
	w.user("nueva", "active", nil)
	susp := w.user("suspendido", "active", &future)
	baja := w.user("baja", "deactivated", nil)
	for i := 1; i <= 6; i++ {
		w.item(fmt.Sprintf("T%d", i), []string{ana, beto}[i%2], tools, "Tula", "active")
	}
	for i := 1; i <= 4; i++ {
		w.item(fmt.Sprintf("H%d", i), []string{ana, beto}[i%2], "hogar", []string{"Tula", "Pachuca"}[i%2], "active")
	}
	w.item("propio", yo, tools, "Tula", "active")
	w.item("pausado", ana, tools, "Tula", "paused")
	w.item("de-suspendido", susp, tools, "Tula", "active")
	w.item("de-baja", baja, tools, "Tula", "active")
}

// coViews siembra personas que ven juntas las herramientas: es la señal del
// colaborativo (CA-01). También ven objetos excluidos, que nunca deben salir.
func (w *world) coViews() {
	var events []domain.Event
	at := time.Now().Add(-time.Hour)
	groups := [][]string{{"T1", "T2", "T3", "propio"}, {"T1", "T3", "T4", "de-suspendido"}, {"T2", "T3", "T4", "pausado"}, {"T1", "T4", "T5", "de-baja"}, {"H1", "H2"}}
	for g, keys := range groups {
		for n := range 3 {
			for i, k := range keys {
				events = append(events, domain.Event{UserID: fmt.Sprintf("p%d-%d", g, n), ItemID: w.items[k], Type: domain.EventView, Weight: 1,
					At: at.Add(time.Duration(g*100+n*10+i) * time.Second)})
			}
		}
	}
	if _, err := recomongo.NewInteractionRepo(w.db).InsertDemo(context.Background(), events); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) call(method, path, user string, body any) (int, []byte, http.Header) {
	w.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, w.api.URL+path, rd)
	if user != "" {
		tok, err := w.jwt.Issue(w.users[user], "user")
		if err != nil {
			w.t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, raw, res.Header
}

func (w *world) view(user, key string) int {
	code, _, _ := w.call(http.MethodPost, "/api/v1/recommendations/events", user, map[string]string{"itemId": w.items[key], "type": "view"})
	return code
}

type recList struct {
	Items []struct {
		Item struct {
			ID       string `json:"id"`
			Category string `json:"category"`
		} `json:"item"`
		Reason struct {
			Code      string `json:"code"`
			ItemID    string `json:"itemId"`
			ItemTitle string `json:"itemTitle"`
		} `json:"reason"`
	} `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

func (w *world) recommend(user, query string) recList {
	w.t.Helper()
	code, raw, _ := w.call(http.MethodGet, "/api/v1/recommendations"+query, user, nil)
	if code != http.StatusOK {
		w.t.Fatalf("GET recommendations: %d %s", code, raw)
	}
	var out recList
	if err := json.Unmarshal(raw, &out); err != nil {
		w.t.Fatal(err)
	}
	return out
}

func (w *world) recompute() {
	w.t.Helper()
	res, err := w.svc.Recompute(context.Background())
	if err != nil || !res.Built {
		w.t.Fatalf("Recompute: %+v %v", res, err)
	}
}

func (w *world) key(id string) string {
	for k, v := range w.items {
		if v == id {
			return k
		}
	}
	return id
}

func (w *world) keys(l recList) []string {
	out := make([]string, len(l.Items))
	for i, it := range l.Items {
		out[i] = w.key(it.Item.ID)
	}
	return out
}

// CA-01 y CA-03: quien ve herramientas recibe sobre todo herramientas vistas
// por personas parecidas, con el motivo; nunca propios, pausados, de cuentas
// suspendidas o dadas de baja, del historial ni en reserva activa.
func TestIntegrationHybridAndExclusionsCA01CA03(t *testing.T) {
	w := newWorld(t, true)
	w.coViews()
	for _, k := range []string{"T1", "T2"} {
		if code := w.view("yo", k); code != http.StatusNoContent {
			t.Fatalf("vista %s: %d", k, code)
		}
	}
	// T5 en una solicitud pendiente de «yo»: no debe recomendarse.
	if _, err := w.db.Collection("bookings").InsertOne(context.Background(), bson.M{
		"requesterId": w.users["yo"], "itemId": w.items["T5"], "status": "pending", "createdAt": time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	w.recompute()
	got := w.recommend("yo", "?limit=10")
	keys := w.keys(got)
	if len(keys) == 0 || got.NextCursor != nil {
		t.Fatalf("recomendados = %v", keys)
	}
	if keys[0] != "T3" && keys[0] != "T4" {
		t.Errorf("lo primero debe ser una herramienta vista por personas parecidas: %v", keys)
	}
	// El motivo es un objeto del historial: sus vistas (T1, T2) o su solicitud
	// pendiente (T5, que pesa 3 y por eso suele ganar; RF-01).
	first := got.Items[0].Reason
	if first.Code != "similar_to" || !slices.Contains([]string{"T1", "T2", "T5"}, w.key(first.ItemID)) || first.ItemTitle != "Objeto "+w.key(first.ItemID) {
		t.Errorf("motivo = %+v", first)
	}
	toolsCount := 0
	for _, it := range got.Items {
		if it.Item.Category == tools {
			toolsCount++
		}
	}
	if toolsCount != domain.MaxPerCategory {
		t.Errorf("herramientas = %d, want %d (diversidad): %v", toolsCount, domain.MaxPerCategory, keys)
	}
	for _, never := range []string{"T1", "T2", "T5", "propio", "pausado", "de-suspendido", "de-baja"} {
		if slices.Contains(keys, never) {
			t.Errorf("%s nunca debe recomendarse: %v", never, keys)
		}
	}
}

// CA-02: una persona nueva recibe populares de su municipio, nunca una lista vacía.
func TestIntegrationColdStartCA02(t *testing.T) {
	w := newWorld(t, true)
	w.coViews()
	w.recompute()
	got := w.recommend("nueva", "?municipality=pachuca")
	if len(got.Items) == 0 || got.Items[0].Reason.Code != "popular_nearby" {
		t.Fatalf("arranque en frío = %+v", got.Items)
	}
	if k := w.key(got.Items[0].Item.ID); k != "H1" && k != "H3" {
		t.Errorf("lo primero debe ser de Pachuca: %s", k)
	}
	// Sin modelo todavía también responde, y el recálculo se lanza aparte.
	w2 := newWorld(t, true)
	start := time.Now()
	if l := w2.recommend("nueva", ""); len(l.Items) == 0 {
		t.Error("sin modelo la lista no debe estar vacía")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("la petición esperó el recálculo: %v", d)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, _, ok, _ := recomongo.NewMetaRepo(w2.db).Current(context.Background()); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("el recálculo perezoso no publicó un modelo")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// CA-04: el híbrido supera a la popularidad con los datos sintéticos.
func TestIntegrationEvaluationCA04(t *testing.T) {
	cfg := synth.Default()
	items := synth.Items(cfg)
	r := usecase.Evaluate(synth.Interactions(items, cfg), items, 10)
	if r.Hybrid.HitAt <= r.Popularity.HitAt {
		t.Errorf("híbrido %.3f vs popularidad %.3f", r.Hybrid.HitAt, r.Popularity.HitAt)
	}
}

// CA-05: borrar el historial deja a la persona en el arranque en frío.
func TestIntegrationDeleteHistoryCA05(t *testing.T) {
	w := newWorld(t, true)
	w.coViews()
	w.view("yo", "T1")
	w.view("yo", "T2")
	w.recompute()
	if got := w.recommend("yo", ""); got.Items[0].Reason.Code != "similar_to" {
		t.Fatalf("antes: %+v", got.Items[0])
	}
	if code, raw, _ := w.call(http.MethodDelete, "/api/v1/recommendations/history", "yo", nil); code != http.StatusNoContent {
		t.Fatalf("DELETE: %d %s", code, raw)
	}
	if n, _ := w.db.Collection(recomongo.InteractionsCollection).CountDocuments(context.Background(), bson.M{"userId": w.users["yo"]}); n != 0 {
		t.Errorf("quedan %d eventos", n)
	}
	for _, it := range w.recommend("yo", "").Items {
		if it.Reason.Code == "similar_to" {
			t.Errorf("después del borrado no debe haber similar_to: %+v", it)
		}
	}
}

// CA-06: apagado, /meta lo dice y las tres rutas responden 503.
func TestIntegrationDisabledCA06(t *testing.T) {
	w := newWorld(t, false)
	_, raw, _ := w.call(http.MethodGet, "/api/v1/meta", "", nil)
	if !strings.Contains(string(raw), `"recommendations":false`) {
		t.Errorf("meta = %s", raw)
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/recommendations/events"}, {http.MethodGet, "/api/v1/recommendations"}, {http.MethodDelete, "/api/v1/recommendations/history"},
	} {
		code, raw, _ := w.call(c.method, c.path, "yo", map[string]string{})
		if code != http.StatusServiceUnavailable || !strings.Contains(string(raw), "service_unavailable") {
			t.Errorf("%s %s: %d %s", c.method, c.path, code, raw)
		}
	}
}

// CA-07: los registros de las peticiones no llevan objetos ni personas, y el
// código de la spec no contiene secretos ni correos reales.
func TestIntegrationNoPersonalDataCA07(t *testing.T) {
	w := newWorld(t, true)
	w.coViews()
	w.view("yo", "T1")
	w.recompute()
	w.recommend("yo", "")
	w.call(http.MethodDelete, "/api/v1/recommendations/history", "yo", nil)
	logs := w.logs.String()
	for name, id := range w.items {
		if strings.Contains(logs, id) {
			t.Errorf("el registro contiene el objeto %s", name)
		}
	}
	if strings.Contains(logs, w.users["yo"]) {
		t.Error("el registro contiene a la persona")
	}

	out, err := exec.Command("git", "grep", "--untracked", "-n", "-E",
		`(gsk_[A-Za-z0-9]{10,}|sk-or-[A-Za-z0-9-]{10,}|mongodb(\+srv)?://[^/\s:]+:[^@\s]+@|[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+\.(com|mx|net|org)\b)`,
		"--", ":(top)internal/reco", ":(top)cmd/admin/reco_*.go", ":(top,exclude)internal/reco/integration_test.go").Output()
	var exitErr *exec.ExitError
	if err != nil && (!errors.As(err, &exitErr) || exitErr.ExitCode() != 1) {
		if _, statErr := os.Stat("../../.git"); statErr != nil {
			t.Skip("sin repositorio git")
		}
		t.Fatalf("git grep: %v", err)
	}
	if len(out) > 0 {
		t.Errorf("posibles secretos o correos en el código de la spec:\n%s", out)
	}
}

// CA-08: con el candado ocupado nadie recalcula, y una petición con el modelo
// vencido no espera el recálculo.
func TestIntegrationLockCA08(t *testing.T) {
	w := newWorld(t, true)
	meta := recomongo.NewMetaRepo(w.db)
	if ok, err := meta.TryLock(context.Background(), "otro-proceso", time.Now()); err != nil || !ok {
		t.Fatalf("TryLock: %v %v", ok, err)
	}
	if res, err := w.svc.Recompute(context.Background()); err != nil || res.Built {
		t.Errorf("con el candado ajeno no debe recalcular: %+v %v", res, err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var slowest time.Duration
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			w.recommend("nueva", "")
			mu.Lock()
			slowest = max(slowest, time.Since(start))
			mu.Unlock()
		}()
	}
	wg.Wait()
	if slowest > 2*time.Second {
		t.Errorf("una petición esperó: %v", slowest)
	}
	if _, _, ok, _ := meta.Current(context.Background()); ok {
		t.Error("nadie debía publicar mientras otro tiene el candado")
	}
}

// CA-09: una vista repetida en 10 min se guarda una vez, y la petición 61 en
// un minuto recibe 429 (límite en Redis).
func TestIntegrationViewsAndRateLimitCA09(t *testing.T) {
	w := newWorld(t, true)
	for range 2 { // la misma vista dos veces en el mismo bloque de 10 min
		if code := w.view("yo", "T1"); code != http.StatusNoContent {
			t.Fatalf("vista: %d", code)
		}
	}
	if n, _ := w.db.Collection(recomongo.InteractionsCollection).CountDocuments(context.Background(), bson.M{"userId": w.users["yo"]}); n != 1 {
		t.Errorf("vistas guardadas = %d, want 1", n)
	}
	if code := w.view("yo", "propio"); code != http.StatusNoContent {
		t.Errorf("objeto propio: %d", code)
	}
	code, _, _ := w.call(http.MethodPost, "/api/v1/recommendations/events", "yo", map[string]string{"itemId": "0123456789abcdef01234567", "type": "view"})
	if code != http.StatusNotFound {
		t.Errorf("objeto inexistente: %d", code)
	}
	for i := 4; i <= domain.EventsPerMinute; i++ {
		w.view("yo", "T2")
	}
	code, raw, h := w.call(http.MethodPost, "/api/v1/recommendations/events", "yo", map[string]string{"itemId": w.items["T2"], "type": "view"})
	if code != http.StatusTooManyRequests || h.Get("Retry-After") == "" || !strings.Contains(string(raw), "rate_limited") {
		t.Errorf("61.ª: %d %s", code, raw)
	}
}

// RNF-01: p95 de GET /recommendations < 400 ms con el modelo calculado.
func TestIntegrationLatencyRNF01(t *testing.T) {
	w := newWorld(t, true)
	w.coViews()
	w.view("yo", "T1")
	w.view("yo", "T2")
	w.recompute()
	var d []time.Duration
	for range 40 {
		start := time.Now()
		w.recommend("yo", "")
		d = append(d, time.Since(start))
	}
	slices.Sort(d)
	p95 := d[len(d)*95/100-1]
	t.Logf("p95 local = %v", p95)
	if p95 > 400*time.Millisecond {
		t.Errorf("p95 = %v", p95)
	}
}
