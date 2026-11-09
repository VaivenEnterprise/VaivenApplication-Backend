//go:build integration

package mongo

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
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

var baseTime = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// seed prepara una base con los índices de items (005) y de búsqueda (006).
type seed struct {
	t     *testing.T
	db    *drv.Database
	repo  *Repo
	users map[string]string // nombre → id
	n     int
}

func newSeed(t *testing.T) *seed {
	t.Helper()
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("search_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	if err := itemsmongo.NewItemRepo(db).EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := NewIndexes(db).EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	return &seed{t: t, db: db, repo: NewRepo(db, func() time.Time { return baseTime.Add(240 * time.Hour) }), users: map[string]string{}}
}

// user crea una cuenta con el estado y la suspensión indicados.
func (s *seed) user(name, status string, suspendedUntil *time.Time) string {
	s.t.Helper()
	res, err := s.db.Collection(UsersCollection).InsertOne(context.Background(), bson.M{
		"email": name + "@vaiven.invalid", "displayName": name, "status": status,
		"suspendedUntil": suspendedUntil, "ratingSum": int64(9), "ratingCount": int64(2),
	})
	if err != nil {
		s.t.Fatal(err)
	}
	id := res.InsertedID.(bson.ObjectID).Hex()
	s.users[name] = id
	return id
}

type itemOpt func(bson.M)

func withText(title, desc string) itemOpt {
	return func(m bson.M) { m["title"], m["description"] = title, desc }
}
func withStatus(st string) itemOpt { return func(m bson.M) { m["status"] = st } }
func withPoint(lat, lng float64) itemOpt {
	return func(m bson.M) { m["location"] = bson.M{"type": "Point", "coordinates": bson.A{lng, lat}} }
}
func withCategory(c string) itemOpt { return func(m bson.M) { m["category"] = c } }

// item crea un objeto activo de owner; cada uno un minuto más reciente.
func (s *seed) item(owner string, opts ...itemOpt) string {
	s.t.Helper()
	s.n++
	doc := bson.M{
		"ownerId": owner, "title": fmt.Sprintf("Objeto %d", s.n), "description": "algo",
		"category": "hogar", "condition": "good", "modality": "rent",
		"declaredValueCents": int64(100000), "pricePerDayCents": int64(3000), "priceSource": "manual",
		"zone":   bson.M{"municipality": "Coyoacán", "neighborhood": nil},
		"photos": bson.A{bson.M{"publicId": "p", "width": 1, "height": 1, "format": "jpg", "bytes": int64(1), "delivery": "public"}},
		"status": "active", "createdAt": baseTime.Add(time.Duration(s.n) * time.Minute), "version": int64(1),
	}
	for _, o := range opts {
		o(doc)
	}
	res, err := s.db.Collection(ItemsCollection).InsertOne(context.Background(), doc)
	if err != nil {
		s.t.Fatal(err)
	}
	return res.InsertedID.(bson.ObjectID).Hex()
}

func (s *seed) search(q domain.Query) domain.Page {
	s.t.Helper()
	if q.Limit == 0 {
		q.Limit = 50
	}
	p, err := s.repo.Search(context.Background(), q)
	if err != nil {
		s.t.Fatalf("Search: %v", err)
	}
	return p
}

func ids(p domain.Page) []string {
	out := make([]string, 0, len(p.Cards))
	for _, c := range p.Cards {
		out = append(out, c.ID)
	}
	return out
}

func TestIntegrationPaginationCA01(t *testing.T) {
	s := newSeed(t)
	ana := s.user("ana", "active", nil)
	for range 30 {
		s.item(ana)
	}
	seen := map[string]bool{}
	q := domain.Query{ViewerID: "yo", Limit: 10}
	pages := 0
	for {
		p := s.search(q)
		pages++
		for _, id := range ids(p) {
			if seen[id] {
				t.Fatalf("repetido %s", id)
			}
			seen[id] = true
		}
		if p.Next == nil {
			break
		}
		if len(p.Cards) != 10 {
			t.Errorf("página %d con %d", pages, len(p.Cards))
		}
		// El cursor viaja codificado como en la API.
		c, err := domain.DecodeCursor(p.Next.Encode())
		if err != nil {
			t.Fatal(err)
		}
		q.After = &c
	}
	if len(seen) != 30 || pages != 3 {
		t.Errorf("vistos %d en %d páginas", len(seen), pages)
	}
	first := s.search(domain.Query{ViewerID: "yo", Limit: 1}).Cards[0]
	if first.Title != "Objeto 30" || first.Owner.DisplayName != "ana" || *first.Owner.RatingAverage != 4.5 ||
		first.PhotoCount != 1 || first.FirstPhoto.PublicID != "p" || first.DistanceKm() != nil {
		t.Errorf("tarjeta = %+v", first)
	}
}

func TestIntegrationTextCA02(t *testing.T) {
	s := newSeed(t)
	ana := s.user("ana", "active", nil)
	exact := s.item(ana, withText("Taladro", "Percutor 600 W"))
	plural := s.item(ana, withText("Taladros eléctricos", "Dos piezas"))
	far := s.item(ana, withText("Caja de herramientas", "Incluye llaves y un taladro pequeño"))
	s.item(ana, withText("Bicicleta", "Rodada 26"))

	got := ids(s.search(domain.Query{ViewerID: "yo", Text: "taladro"}))
	if len(got) != 3 || got[2] != far || (got[0] != exact && got[0] != plural) {
		t.Errorf("orden = %v; want título primero y %s al final", got, far)
	}
	// Sin acentos ni mayúsculas.
	if got := ids(s.search(domain.Query{ViewerID: "yo", Text: "ELECTRICOS"})); len(got) != 1 || got[0] != plural {
		t.Errorf("sin acentos = %v", got)
	}
	// Paginación por relevancia sin repetidos.
	p := s.search(domain.Query{ViewerID: "yo", Text: "taladro", Limit: 2})
	c, _ := domain.DecodeCursor(p.Next.Encode())
	p2 := s.search(domain.Query{ViewerID: "yo", Text: "taladro", Limit: 2, After: &c})
	if len(p.Cards) != 2 || len(p2.Cards) != 1 || p2.Next != nil || p2.Cards[0].ID != far {
		t.Errorf("páginas de texto = %v / %v", ids(p), ids(p2))
	}
}

func TestIntegrationNearCA04(t *testing.T) {
	s := newSeed(t)
	ana := s.user("ana", "active", nil)
	origin := domain.Point{Lat: 19.35, Lng: -99.16}
	far := s.item(ana, withPoint(19.40, -99.16), withText("Bici de montaña", "x"))   // ~5.6 km
	near := s.item(ana, withPoint(19.36, -99.16), withText("Bicicleta urbana", "x")) // ~1.1 km
	mid := s.item(ana, withPoint(19.37, -99.16), withText("Patines", "x"))           // ~2.2 km
	s.item(ana, withPoint(20.35, -99.16))                                            // ~111 km
	s.item(ana)                                                                      // sin ubicación

	p := s.search(domain.Query{ViewerID: "yo", Near: &origin, RadiusKm: 25})
	if got := ids(p); fmt.Sprint(got) != fmt.Sprint([]string{near, mid, far}) {
		t.Fatalf("orden por distancia = %v", got)
	}
	if d := *p.Cards[0].DistanceKm(); d != 1.1 {
		t.Errorf("distanceKm = %v", d)
	}
	if p.Cards[0].Location == nil || p.Cards[0].Location.Lat != 19.36 {
		t.Errorf("ubicación = %+v", p.Cards[0].Location)
	}
	// Radio menor.
	if got := ids(s.search(domain.Query{ViewerID: "yo", Near: &origin, RadiusKm: 3})); len(got) != 2 {
		t.Errorf("radio 3 km = %v", got)
	}
	// Texto con cercanía: palabras sin acentos.
	if got := ids(s.search(domain.Query{ViewerID: "yo", Near: &origin, RadiusKm: 25, Text: "bici MONTAÑA"})); len(got) != 1 || got[0] != far {
		t.Errorf("texto + cercanía = %v", got)
	}
	// Paginación por distancia.
	p = s.search(domain.Query{ViewerID: "yo", Near: &origin, RadiusKm: 25, Limit: 2})
	c, _ := domain.DecodeCursor(p.Next.Encode())
	p2 := s.search(domain.Query{ViewerID: "yo", Near: &origin, RadiusKm: 25, Limit: 2, After: &c})
	if fmt.Sprint(append(ids(p), ids(p2)...)) != fmt.Sprint([]string{near, mid, far}) || p2.Next != nil {
		t.Errorf("páginas por distancia = %v / %v", ids(p), ids(p2))
	}
}

func TestIntegrationExclusionsCA05(t *testing.T) {
	s := newSeed(t)
	future := baseTime.Add(1000 * time.Hour)
	past := baseTime.Add(-1000 * time.Hour)
	ana := s.user("ana", "active", nil)
	yo := s.user("yo", "active", nil)
	baja := s.user("baja", "deactivated", nil)
	susp := s.user("susp", "active", &future)
	exSusp := s.user("exsusp", "active", &past)

	ok := s.item(ana, withCategory("deportes"))
	s.item(ana, withStatus("paused"))  // CA-05
	s.item(ana, withStatus("blocked")) // moderación
	s.item(ana, withStatus("removed"))
	s.item(yo)                  // propio (RF-02)
	s.item(baja)                // cuenta dada de baja
	s.item(susp)                // cuenta suspendida
	s.item("no-es-un-objectid") // dueño mal formado: no rompe la consulta
	again := s.item(exSusp)     // la suspensión ya venció
	other := s.item(ana)        // categoría hogar

	got := ids(s.search(domain.Query{ViewerID: yo}))
	if fmt.Sprint(got) != fmt.Sprint([]string{other, again, ok}) {
		t.Errorf("visibles = %v, want %v", got, []string{other, again, ok})
	}
	if got := ids(s.search(domain.Query{ViewerID: yo, Category: "deportes"})); len(got) != 1 || got[0] != ok {
		t.Errorf("categoría = %v", got)
	}
	if got := ids(s.search(domain.Query{ViewerID: yo, Modality: "loan"})); len(got) != 0 {
		t.Errorf("modalidad = %v", got)
	}
	if got := ids(s.search(domain.Query{ViewerID: yo, Municipality: "COYOACÁN"})); len(got) != 3 {
		t.Errorf("municipio sin mayúsculas = %v", got)
	}
	if got := ids(s.search(domain.Query{ViewerID: yo, Municipality: "coyoacan"})); len(got) != 3 {
		t.Errorf("municipio sin acentos = %v", got)
	}
	if got := ids(s.search(domain.Query{ViewerID: yo, Municipality: "Coyo"})); len(got) != 0 {
		t.Errorf("municipio parcial = %v", got)
	}
}

func TestIntegrationSearchIndexes(t *testing.T) {
	s := newSeed(t)
	// Idempotente.
	if err := NewIndexes(s.db).EnsureIndexes(context.Background()); err != nil {
		t.Fatalf("segunda vez: %v", err)
	}
	cur, err := s.db.Collection(ItemsCollection).Indexes().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var idx []bson.M
	_ = cur.All(context.Background(), &idx)
	found := map[string]bson.M{}
	for _, i := range idx {
		found[fmt.Sprint(i["name"])] = i
	}
	text, ok := found[TextIndexName]
	if !ok || text["default_language"] != "spanish" {
		t.Errorf("índice de texto = %v", text)
	}
	var w struct {
		Title       int `bson:"title"`
		Description int `bson:"description"`
	}
	raw, _ := bson.Marshal(text["weights"])
	if err := bson.Unmarshal(raw, &w); err != nil || w.Title != 5 || w.Description != 1 {
		t.Errorf("pesos = %v", text["weights"])
	}
	for _, n := range []string{FiltersIndexName, RecentIndexName} {
		if _, ok := found[n]; !ok {
			t.Errorf("falta %s", n)
		}
	}
	// Una búsqueda de texto sin índice falla con error (no en silencio).
	empty := testClient.Database(fmt.Sprintf("search_noidx_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = empty.Drop(context.Background()) })
	if _, err := NewRepo(empty, nil).Search(context.Background(), domain.Query{ViewerID: "x", Text: "taladro", Limit: 5}); err == nil {
		t.Error("sin índice de texto debe fallar")
	}
}
