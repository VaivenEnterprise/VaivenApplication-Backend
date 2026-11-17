package mongo

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func spec(col string, keys bson.D, opts *options.IndexOptionsBuilder) IndexSpec {
	return IndexSpec{Collection: col, Model: drv.IndexModel{Keys: keys, Options: opts}}
}

// listed arma un documento como los de listIndexes.
func listed(name string, key bson.D, extra ...bson.E) bson.D {
	d := bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: key}, {Key: "name", Value: name}}
	return append(d, extra...)
}

var idIndex = listed("_id_", bson.D{{Key: "_id", Value: int32(1)}})

func findingsByName(t *testing.T, f []IndexFinding) map[string]IndexFinding {
	t.Helper()
	m := map[string]IndexFinding{}
	for _, x := range f {
		m[x.Collection+"."+x.Name] = x
	}
	return m
}

func mustCompare(t *testing.T, want []IndexSpec, have map[string][]bson.D) map[string]IndexFinding {
	t.Helper()
	f, err := CompareIndexes(want, have)
	if err != nil {
		t.Fatalf("CompareIndexes: %v", err)
	}
	return findingsByName(t, f)
}

func TestModelsKeepsOrder(t *testing.T) {
	a := spec("c", bson.D{{Key: "a", Value: 1}}, options.Index().SetName("a"))
	b := spec("c", bson.D{{Key: "b", Value: 1}}, options.Index().SetName("b"))
	got := Models([]IndexSpec{a, b})
	if len(got) != 2 || got[0].Options != a.Model.Options || got[1].Options != b.Model.Options {
		t.Fatalf("Models no conserva el orden ni los modelos: %+v", got)
	}
	if len(Models(nil)) != 0 {
		t.Fatal("Models(nil) debe ser vacío")
	}
}

func TestCompareOKIgnoresIDAndServerMetadata(t *testing.T) {
	want := []IndexSpec{spec("users", bson.D{{Key: "email", Value: 1}}, options.Index().SetName("email_unique").SetUnique(true))}
	have := map[string][]bson.D{"users": {idIndex, listed("email_unique", bson.D{{Key: "email", Value: int32(1)}},
		bson.E{Key: "unique", Value: true}, bson.E{Key: "ns", Value: "db.users"})}}
	got := mustCompare(t, want, have)
	if len(got) != 1 || got["users.email_unique"].Status != IndexOK {
		t.Fatalf("want solo OK, got %+v", got)
	}
}

func TestCompareNumberNormalization(t *testing.T) {
	// Prueba 11: int, int32, int64 y double con el mismo valor son iguales.
	want := []IndexSpec{spec("n", bson.D{{Key: "a", Value: 1}, {Key: "b", Value: -1}},
		options.Index().SetName("ab").SetExpireAfterSeconds(60))}
	have := map[string][]bson.D{"n": {listed("ab", bson.D{{Key: "a", Value: float64(1)}, {Key: "b", Value: int64(-1)}},
		bson.E{Key: "expireAfterSeconds", Value: int64(60)})}}
	if st := mustCompare(t, want, have)["n.ab"].Status; st != IndexOK {
		t.Fatalf("status = %s, want OK", st)
	}
	have["n"][0] = listed("ab", bson.D{{Key: "a", Value: float64(1.5)}, {Key: "b", Value: int64(-1)}},
		bson.E{Key: "expireAfterSeconds", Value: int64(60)})
	if st := mustCompare(t, want, have)["n.ab"].Status; st != IndexIncompatible {
		t.Fatalf("1 contra 1.5: status = %s, want INCOMPATIBLE", st)
	}
}

func TestCompareKeyOrderMatters(t *testing.T) {
	want := []IndexSpec{spec("b", bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}, options.Index().SetName("s"))}
	have := map[string][]bson.D{"b": {listed("s", bson.D{{Key: "createdAt", Value: int32(-1)}, {Key: "status", Value: int32(1)}})}}
	f := mustCompare(t, want, have)["b.s"]
	if f.Status != IndexIncompatible || !strings.Contains(f.Detail, "key") {
		t.Fatalf("got %+v, want INCOMPATIBLE en key", f)
	}
}

func TestCompareMissingExtraAndRenamed(t *testing.T) {
	// Pruebas 4, 5 y 7: el equivalente con otro nombre NO cuenta como correcto.
	want := []IndexSpec{
		spec("r", bson.D{{Key: "userId", Value: 1}}, options.Index().SetName("user")),
		spec("r", bson.D{{Key: "token", Value: 1}}, options.Index().SetName("token_unique").SetUnique(true)),
	}
	have := map[string][]bson.D{"r": {
		idIndex,
		listed("userId_1", bson.D{{Key: "userId", Value: int32(1)}}),
		listed("otro", bson.D{{Key: "x", Value: int32(1)}}),
	}}
	got := mustCompare(t, want, have)
	if f := got["r.user"]; f.Status != IndexMissing || f.SameShapeAs != "userId_1" {
		t.Errorf("user = %+v, want FALTANTE con NOMBRE DISTINTO userId_1", f)
	}
	if f := got["r.userId_1"]; f.Status != IndexExtra || f.SameShapeAs != "user" || !f.Discrepancy() {
		t.Errorf("userId_1 = %+v, want SOBRANTE con NOMBRE DISTINTO user", f)
	}
	if f := got["r.token_unique"]; f.Status != IndexMissing || f.SameShapeAs != "" {
		t.Errorf("token_unique = %+v, want FALTANTE sin nota", f)
	}
	if f := got["r.otro"]; f.Status != IndexExtra || f.SameShapeAs != "" {
		t.Errorf("otro = %+v, want SOBRANTE sin nota", f)
	}
}

func TestCompareMissingCollectionAndUndeclaredCollection(t *testing.T) {
	want := []IndexSpec{spec("nueva", bson.D{{Key: "a", Value: 1}}, options.Index().SetName("a"))}
	have := map[string][]bson.D{"ajena": {idIndex, listed("x", bson.D{{Key: "x", Value: int32(1)}})}}
	got := mustCompare(t, want, have)
	if got["nueva.a"].Status != IndexMissing || got["ajena.x"].Status != IndexExtra {
		t.Fatalf("got %+v", got)
	}
	if _, ok := got["ajena._id_"]; ok {
		t.Fatal("_id_ nunca se reporta")
	}
}

func TestCompareAbsentOptionIsNotAssumedFalse(t *testing.T) {
	// Declarar unique:false explícito no equivale a que la opción no exista.
	want := []IndexSpec{spec("u", bson.D{{Key: "a", Value: 1}}, options.Index().SetName("a").SetUnique(false))}
	have := map[string][]bson.D{"u": {listed("a", bson.D{{Key: "a", Value: int32(1)}})}}
	f := mustCompare(t, want, have)["u.a"]
	if f.Status != IndexIncompatible || !strings.Contains(f.Detail, "unique") {
		t.Fatalf("got %+v, want INCOMPATIBLE por unique", f)
	}
	// Y una opción real no declarada también se informa.
	want = []IndexSpec{spec("u", bson.D{{Key: "a", Value: 1}}, options.Index().SetName("a"))}
	have = map[string][]bson.D{"u": {listed("a", bson.D{{Key: "a", Value: int32(1)}}, bson.E{Key: "sparse", Value: true})}}
	if f := mustCompare(t, want, have)["u.a"]; f.Status != IndexIncompatible || !strings.Contains(f.Detail, "sparse") {
		t.Fatalf("got %+v, want INCOMPATIBLE por sparse", f)
	}
}

func TestComparePartialFilterKeepsTypes(t *testing.T) {
	want := []IndexSpec{spec("m", bson.D{{Key: "c", Value: 1}}, options.Index().SetName("c").SetUnique(true).
		SetPartialFilterExpression(bson.D{{Key: "c", Value: bson.D{{Key: "$type", Value: "string"}}}}))}
	ok := listed("c", bson.D{{Key: "c", Value: int32(1)}}, bson.E{Key: "unique", Value: true},
		bson.E{Key: "partialFilterExpression", Value: bson.D{{Key: "c", Value: bson.D{{Key: "$type", Value: "string"}}}}})
	if st := mustCompare(t, want, map[string][]bson.D{"m": {ok}})["m.c"].Status; st != IndexOK {
		t.Fatalf("status = %s, want OK", st)
	}
	// $type 2 (número de tipo) no se iguala a "string" sin una regla explícita.
	bad := listed("c", bson.D{{Key: "c", Value: int32(1)}}, bson.E{Key: "unique", Value: true},
		bson.E{Key: "partialFilterExpression", Value: bson.D{{Key: "c", Value: bson.D{{Key: "$type", Value: int32(2)}}}}})
	if st := mustCompare(t, want, map[string][]bson.D{"m": {bad}})["m.c"].Status; st != IndexIncompatible {
		t.Fatalf("status = %s, want INCOMPATIBLE", st)
	}
}

func TestCompareTextIndex(t *testing.T) {
	// Prueba 12: MongoDB guarda {_fts, _ftsx}, reordena los pesos y agrega
	// language_override y textIndexVersion.
	want := []IndexSpec{spec("items", bson.D{{Key: "title", Value: "text"}, {Key: "description", Value: "text"}},
		options.Index().SetName("search_text").SetDefaultLanguage("spanish").
			SetWeights(bson.D{{Key: "title", Value: 5}, {Key: "description", Value: 1}}))}
	server := func(lang string, titleWeight int32) bson.D {
		return listed("search_text", bson.D{{Key: "_fts", Value: "text"}, {Key: "_ftsx", Value: int32(1)}},
			bson.E{Key: "weights", Value: bson.D{{Key: "description", Value: int32(1)}, {Key: "title", Value: titleWeight}}},
			bson.E{Key: "default_language", Value: lang},
			bson.E{Key: "language_override", Value: "language"},
			bson.E{Key: "textIndexVersion", Value: int32(3)})
	}
	if f := mustCompare(t, want, map[string][]bson.D{"items": {server("spanish", 5)}})["items.search_text"]; f.Status != IndexOK {
		t.Fatalf("got %+v, want OK", f)
	}
	if f := mustCompare(t, want, map[string][]bson.D{"items": {server("english", 5)}})["items.search_text"]; f.Status != IndexIncompatible {
		t.Fatalf("idioma distinto: got %+v", f)
	}
	if f := mustCompare(t, want, map[string][]bson.D{"items": {server("spanish", 3)}})["items.search_text"]; f.Status != IndexIncompatible {
		t.Fatalf("peso distinto: got %+v", f)
	}
}

func TestCompareTextDefaultsOnlyForText(t *testing.T) {
	// Sin pesos ni idioma declarados: pesos 1 y los valores por defecto
	// documentados de MongoDB para índices de texto.
	want := []IndexSpec{spec("t", bson.D{{Key: "body", Value: "text"}}, options.Index().SetName("body_text"))}
	have := map[string][]bson.D{"t": {listed("body_text", bson.D{{Key: "_fts", Value: "text"}, {Key: "_ftsx", Value: int32(1)}},
		bson.E{Key: "weights", Value: bson.D{{Key: "body", Value: int32(1)}}},
		bson.E{Key: "default_language", Value: "english"},
		bson.E{Key: "language_override", Value: "language"},
		bson.E{Key: "textIndexVersion", Value: int32(3)})}}
	if f := mustCompare(t, want, have)["t.body_text"]; f.Status != IndexOK {
		t.Fatalf("got %+v, want OK", f)
	}
}

func TestCompare2dsphere(t *testing.T) {
	want := []IndexSpec{spec("items", bson.D{{Key: "location", Value: "2dsphere"}}, options.Index().SetName("location_2dsphere"))}
	have := map[string][]bson.D{"items": {listed("location_2dsphere", bson.D{{Key: "location", Value: "2dsphere"}},
		bson.E{Key: "2dsphereIndexVersion", Value: int32(3)})}}
	if f := mustCompare(t, want, have)["items.location_2dsphere"]; f.Status != IndexOK {
		t.Fatalf("got %+v, want OK", f)
	}
	// Si la versión se declara, sí se compara.
	want = []IndexSpec{spec("items", bson.D{{Key: "location", Value: "2dsphere"}}, options.Index().SetName("location_2dsphere").SetSphereVersion(2))}
	if f := mustCompare(t, want, have)["items.location_2dsphere"]; f.Status != IndexIncompatible {
		t.Fatalf("versión declarada distinta: got %+v", f)
	}
	have["items"][0] = listed("location_2dsphere", bson.D{{Key: "location", Value: int32(1)}})
	want = []IndexSpec{spec("items", bson.D{{Key: "location", Value: "2dsphere"}}, options.Index().SetName("location_2dsphere"))}
	if f := mustCompare(t, want, have)["items.location_2dsphere"]; f.Status != IndexIncompatible {
		t.Fatalf("2dsphere contra ascendente: got %+v", f)
	}
}

func TestCompareUnknownOptionIsUnverifiable(t *testing.T) {
	want := []IndexSpec{spec("u", bson.D{{Key: "a", Value: 1}}, options.Index().SetName("a"))}
	have := map[string][]bson.D{"u": {listed("a", bson.D{{Key: "a", Value: int32(1)}}, bson.E{Key: "clustered", Value: true})}}
	f := mustCompare(t, want, have)["u.a"]
	if f.Status != IndexUnverifiable || !strings.Contains(f.Detail, "clustered") || !f.Discrepancy() {
		t.Fatalf("got %+v, want NO VERIFICABLE por clustered", f)
	}
}

func TestCompareCollationCompletedByServerIsUnverifiable(t *testing.T) {
	want := []IndexSpec{spec("c", bson.D{{Key: "name", Value: 1}}, options.Index().SetName("name").
		SetCollation(&options.Collation{Locale: "es", Strength: 1}))}
	full := bson.D{{Key: "locale", Value: "es"}, {Key: "caseLevel", Value: false}, {Key: "strength", Value: int32(1)}}
	have := map[string][]bson.D{"c": {listed("name", bson.D{{Key: "name", Value: int32(1)}}, bson.E{Key: "collation", Value: full})}}
	if f := mustCompare(t, want, have)["c.name"]; f.Status != IndexUnverifiable {
		t.Fatalf("got %+v, want NO VERIFICABLE", f)
	}
	exact := bson.D{{Key: "strength", Value: int32(1)}, {Key: "locale", Value: "es"}}
	have["c"][0] = listed("name", bson.D{{Key: "name", Value: int32(1)}}, bson.E{Key: "collation", Value: exact})
	if f := mustCompare(t, want, have)["c.name"]; f.Status != IndexOK {
		t.Fatalf("collation igual: got %+v", f)
	}
}

func TestCompareRejectsInvalidDeclarations(t *testing.T) {
	cases := map[string][]IndexSpec{
		"claves sin orden": {{Collection: "c", Model: drv.IndexModel{Keys: bson.M{"a": 1}, Options: options.Index().SetName("a")}}},
		"sin claves":       {spec("c", bson.D{}, options.Index().SetName("a"))},
		"duplicado": {
			spec("c", bson.D{{Key: "a", Value: 1}}, options.Index().SetName("a")),
			spec("c", bson.D{{Key: "b", Value: 1}}, options.Index().SetName("a")),
		},
		"texto sin campos de texto": {spec("c", bson.D{{Key: "a", Value: 1}}, options.Index().SetName("a").SetDefaultLanguage("spanish"))},
	}
	for name, specs := range cases {
		if _, err := CompareIndexes(specs, nil); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestExpectedDocumentDefaultName(t *testing.T) {
	name, _, err := ExpectedDocument(spec("c", bson.D{{Key: "a", Value: 1}, {Key: "b", Value: -1}}, nil))
	if err != nil || name != "a_1_b_-1" {
		t.Fatalf("name = %q, err = %v", name, err)
	}
}
