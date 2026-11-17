package model

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

const eps = 1e-12

var t0 = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func ev(user, item string, typ domain.EventType, minute int) domain.Event {
	return domain.Event{UserID: user, ItemID: item, Type: typ, Weight: typ.Weight(), At: t0.Add(time.Duration(minute) * time.Minute)}
}

func near(a, b float64) bool { return math.Abs(a-b) < eps }

func TestCosine(t *testing.T) {
	if got := Cosine(Vector{"x": 1, "y": 2}, Vector{"x": 2, "y": 1}); !near(got, 0.8) {
		t.Errorf("coseno = %v, want 0.8", got)
	}
	if Cosine(Vector{"x": 1}, Vector{"y": 1}) != 0 || Cosine(Vector{}, Vector{"x": 1}) != 0 {
		t.Error("vectores sin nada en común deben dar 0")
	}
	if got := Cosine(Vector{"x": 3}, Vector{"x": 1, "y": 0, "z": 0}); !near(got, 1) {
		t.Errorf("mismo sentido = %v", got)
	}
	if !near(Norm(Vector{"a": 3, "b": 4}), 5) {
		t.Error("norma")
	}
}

// Matriz del ejemplo a mano:
//
//	      A  B  C
//	u1    1  1  .
//	u2    3  1  1   (A es una solicitud: peso 3)
//	u3    .  1  1
//
// ‖A‖² = 10, ‖B‖² = 3, ‖C‖² = 2; A·B = 4, A·C = 3, B·C = 2.
func exampleEvents() []domain.Event {
	return []domain.Event{
		ev("u1", "A", domain.EventView, 1), ev("u1", "A", domain.EventView, 2), ev("u1", "B", domain.EventView, 3),
		ev("u2", "A", domain.EventView, 4), ev("u2", "A", domain.EventRequest, 5), ev("u2", "B", domain.EventView, 6), ev("u2", "C", domain.EventView, 7),
		ev("u3", "B", domain.EventView, 8), ev("u3", "C", domain.EventView, 9),
	}
}

func TestCollaborativeExact(t *testing.T) {
	m := BuildMatrix(exampleEvents())
	if m["u1"]["A"] != 1 || m["u2"]["A"] != 3 || len(m) != 3 {
		t.Fatalf("matriz = %v", m)
	}
	cf := CollaborativeNeighbors(m, 20)
	ab, ac, bc := 4/math.Sqrt(30), 3/math.Sqrt(20), 2/math.Sqrt(6)
	want := map[string][]Scored{
		"A": {{"B", ab}, {"C", ac}},
		"B": {{"C", bc}, {"A", ab}},
		"C": {{"B", bc}, {"A", ac}},
	}
	for item, ws := range want {
		got := cf[item]
		if len(got) != len(ws) {
			t.Fatalf("%s: %v", item, got)
		}
		for i := range ws {
			if got[i].ItemID != ws[i].ItemID || !near(got[i].Score, ws[i].Score) {
				t.Errorf("%s[%d] = %+v, want %+v", item, i, got[i], ws[i])
			}
		}
	}
	// Coincide con el coseno genérico de las columnas.
	colA, colB := Vector{"u1": 1, "u2": 3}, Vector{"u1": 1, "u2": 1, "u3": 1}
	if !near(Cosine(colA, colB), ab) {
		t.Error("el colaborativo no es el coseno de las columnas")
	}
	if top1 := CollaborativeNeighbors(m, 1); len(top1["A"]) != 1 || top1["A"][0].ItemID != "B" {
		t.Errorf("k = 1: %v", top1["A"])
	}
}

func TestMaxItemsPerUser(t *testing.T) {
	var events []domain.Event
	for i := range MaxItemsPerUser + 5 {
		events = append(events, ev("u", fmt.Sprintf("i%03d", i), domain.EventView, i))
	}
	events = append(events, ev("u", "i000", domain.EventRequest, 0)) // peso 3, el más viejo
	row := BuildMatrix(events)["u"]
	if len(row) != MaxItemsPerUser {
		t.Fatalf("objetos = %d", len(row))
	}
	if row["i000"] != 3 {
		t.Error("debe quedarse el de mayor peso aunque sea viejo")
	}
	for _, gone := range []string{"i001", "i002", "i003", "i004", "i005"} {
		if _, ok := row[gone]; ok {
			t.Errorf("%s es de los más viejos y debía salir", gone)
		}
	}
}

func item(id, cat, mod string, cents int64, mun string) domain.ItemFeatures {
	return domain.ItemFeatures{ID: id, Category: cat, Modality: mod, PricePerDayCents: cents, Municipality: mun}
}

func TestContentExact(t *testing.T) {
	f1 := item("1", "hogar", "rent", 8000, "Guadalajara")
	f2 := item("2", "hogar", "rent", 15000, "Zapopan")
	f3 := item("3", "herramientas", "loan", 0, "  guadalajara ")
	f4 := item("4", "hogar", "loan", 0, "")
	f6 := item("6", "otro", "rent", 9000, "Monterrey")
	for _, c := range []struct {
		a, b domain.ItemFeatures
		want float64
	}{
		{f1, f2, 0.75},                    // categoría, modalidad y banda
		{f1, f3, 0.25},                    // solo municipio (sin importar mayúsculas)
		{f1, f4, 1 / math.Sqrt(12)},       // solo categoría; f4 tiene 3 rasgos
		{f3, f4, 2 / math.Sqrt(12)},       // modalidad y banda de préstamo
		{f1, f6, 0.5},                     // modalidad y banda
		{f1, item("7", "", "", 0, ""), 0}, // nada en común
	} {
		got := ContentSimilarity(c.a, c.b)
		if !near(got, c.want) {
			t.Errorf("sim(%s,%s) = %v, want %v", c.a.ID, c.b.ID, got, c.want)
		}
		if !near(got, Cosine(ContentVector(c.a), ContentVector(c.b))) {
			t.Errorf("sim(%s,%s) no es el coseno one-hot", c.a.ID, c.b.ID)
		}
	}
	nb := ContentNeighbors([]domain.ItemFeatures{f1, f2, f3, f4, f6}, 20)
	got := nb["1"]
	if len(got) != 3 || got[0].ItemID != "2" || got[1].ItemID != "4" || got[2].ItemID != "3" {
		t.Fatalf("vecinos de 1 = %+v", got)
	}
	// f6 comparte modalidad y banda pero no categoría ni municipio: no se compara (RF-03).
	if _, ok := nb["6"]; ok {
		t.Errorf("6 no comparte bloque con nadie: %v", nb["6"])
	}
	if len(ContentNeighbors([]domain.ItemFeatures{f1, f2}, 1)["1"]) != 1 {
		t.Error("k = 1")
	}
}

func TestMergeNeighbors(t *testing.T) {
	feats := map[string]domain.ItemFeatures{
		"A": item("A", "hogar", "rent", 8000, "Tula"),
		"B": item("B", "hogar", "rent", 8000, "Tula"), // contenido 1 con A
		"D": item("D", "otro", "loan", 0, "Tula"),     // contenido 1/4 con A
	}
	cf := map[string][]Scored{"A": {{"B", 0.5}, {"C", 0.9}}, "Z": {{"B", 1}}}
	content := map[string][]Scored{"A": {{"D", 0.25}, {"B", 1}, {"A", 1}}}
	got := MergeNeighbors(cf, content, feats, 20)
	a := got["A"]
	// C no está activo y A no es vecino de sí mismo. B: 0.7·0.5 + 0.3·1 = 0.65; D: 0.3·0.25 = 0.075.
	if len(a) != 2 || a[0] != (domain.Neighbor{ItemID: "B", CF: 0.5, Content: 1}) || a[1].ItemID != "D" || a[1].CF != 0 || !near(a[1].Content, 0.25) {
		t.Fatalf("A = %+v", a)
	}
	if !near(Hybrid(a[0].CF, a[0].Content), 0.65) || !near(Hybrid(1, 0), 0.7) || !near(Hybrid(0, 1), 0.3) {
		t.Error("híbrido")
	}
	// Z ya no está activo: conserva vecinos colaborativos, sin contenido.
	if z := got["Z"]; len(z) != 1 || z[0].Content != 0 || z[0].CF != 1 {
		t.Errorf("Z = %+v", z)
	}
	if one := MergeNeighbors(cf, content, feats, 1); len(one["A"]) != 1 {
		t.Error("k = 1")
	}
	if empty := MergeNeighbors(map[string][]Scored{"Q": {{"C", 1}}}, nil, feats, 5); len(empty) != 0 {
		t.Errorf("sin vecinos activos no hay entrada: %v", empty)
	}
}

func TestRankExact(t *testing.T) {
	history := []domain.Event{ev("u", "A", domain.EventView, 1), ev("u", "A", domain.EventRequest, 2), ev("u", "B", domain.EventView, 3)}
	neighbors := map[string][]domain.Neighbor{
		"A": {{ItemID: "C", CF: 0.5, Content: 0.5}, {ItemID: "D", CF: 1}, {ItemID: "B", CF: 1, Content: 1}, {ItemID: "Z"}},
		"B": {{ItemID: "C", CF: 1, Content: 1}, {ItemID: "A", CF: 1, Content: 1}},
	}
	// C: 3·(0.35+0.15) + 1·(0.7+0.3) = 2.5 (más por A); D: 3·0.7 = 2.1.
	got := Rank(history, neighbors, nil)
	if len(got) != 2 || got[0].ItemID != "C" || !near(got[0].Score, 2.5) || got[0].Reason != (domain.Reason{Code: domain.ReasonSimilarTo, ItemID: "A"}) ||
		got[1].ItemID != "D" || !near(got[1].Score, 2.1) {
		t.Fatalf("Rank = %+v", got)
	}
	if ex := Rank(history, neighbors, map[string]bool{"D": true}); len(ex) != 1 || ex[0].ItemID != "C" {
		t.Errorf("exclude = %+v", ex)
	}
	// Empate de aporte: gana el objeto de menor id como motivo.
	tie := Rank([]domain.Event{ev("u", "B", domain.EventView, 1), ev("u", "A", domain.EventView, 2)},
		map[string][]domain.Neighbor{"A": {{ItemID: "X", CF: 1}}, "B": {{ItemID: "X", CF: 1}}}, nil)
	if tie[0].Reason.ItemID != "A" {
		t.Errorf("motivo con empate = %+v", tie[0])
	}
	if len(Rank(nil, neighbors, nil)) != 0 {
		t.Error("sin historial no hay candidatos")
	}
}

func TestColdStartAndDiversify(t *testing.T) {
	popular := map[string][]string{"m:tula": {"x1", "x2"}, "c:hogar": {"x2", "x3"}, "all": {"x4", "x1"}}
	got := ColdStart(popular, " TULA ", []string{"hogar", "nada"}, map[string]bool{"x3": true})
	want := []Candidate{
		{ItemID: "x1", Reason: domain.Reason{Code: domain.ReasonPopularNearby}},
		{ItemID: "x2", Reason: domain.Reason{Code: domain.ReasonPopularNearby}},
		{ItemID: "x4", Reason: domain.Reason{Code: domain.ReasonPopular}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ColdStart = %+v", got)
	}
	if c := ColdStart(popular, "", []string{"hogar"}, nil); c[0].ItemID != "x2" || c[0].Reason.Code != domain.ReasonPopularInCategory {
		t.Errorf("sin municipio = %+v", c)
	}

	cands := []Candidate{{ItemID: "c1"}, {ItemID: "c2"}, {ItemID: "c3"}, {ItemID: "c5"}, {ItemID: "c4"}, {ItemID: "c6"}}
	cat := map[string]string{"c1": "a", "c2": "a", "c3": "a", "c4": "b", "c6": "b"}
	div := Diversify(cands, cat, 2, 3)
	if len(div) != 3 || div[0].ItemID != "c1" || div[1].ItemID != "c2" || div[2].ItemID != "c4" {
		t.Errorf("Diversify = %+v", div)
	}
	if all := Diversify(cands, cat, 3, 10); len(all) != 5 {
		t.Errorf("sin categoría conocida no se recomienda: %+v", all)
	}
}

func TestPopular(t *testing.T) {
	items := []domain.ItemFeatures{
		item("0a", "cat1", "rent", 1, "Mun 1"),
		item("0b", "cat2", "rent", 1, "Mun 1"),
		item("0c", "cat1", "rent", 1, "Mun 2"),
		item("09", "cat2", "rent", 1, ""),
	}
	events := []domain.Event{ev("u1", "0a", domain.EventRequest, 1), ev("u1", "0b", domain.EventView, 2), ev("u2", "0b", domain.EventView, 3), ev("u3", "zz", domain.EventRequest, 4)}
	p := Popular(events, items, 50)
	// 0a = 3, 0b = 2; sin interacciones, del id mayor al menor: 0c, 09. zz no está activo.
	if !reflect.DeepEqual(p["all"], []string{"0a", "0b", "0c", "09"}) {
		t.Errorf("all = %v", p["all"])
	}
	if !reflect.DeepEqual(p["m:mun 1"], []string{"0a", "0b"}) || !reflect.DeepEqual(p["c:cat2"], []string{"0b", "09"}) {
		t.Errorf("listas = %v", p)
	}
	if len(Popular(events, items, 2)["all"]) != 2 {
		t.Error("tamaño")
	}
}

func TestLeaveLastOutAndEvaluate(t *testing.T) {
	events := []domain.Event{
		ev("u1", "A", domain.EventView, 1), ev("u1", "B", domain.EventView, 2), ev("u1", "A", domain.EventView, 3),
		ev("u2", "C", domain.EventView, 1), ev("u2", "C", domain.EventView, 2), // un solo objeto: no se evalúa
		ev("u3", "X", domain.EventView, 5), ev("u3", "Y", domain.EventView, 5), // empate de fecha: Y
		ev("u4", "P", domain.EventView, 1), ev("u4", "Q", domain.EventView, 2),
	}
	s := LeaveLastOut(events)
	if !reflect.DeepEqual(s.Test, map[string]string{"u1": "A", "u3": "Y", "u4": "Q"}) {
		t.Fatalf("Test = %v", s.Test)
	}
	for _, e := range s.Train {
		if s.Test[e.UserID] == e.ItemID {
			t.Errorf("el objeto oculto quedó en Train: %+v", e)
		}
	}
	if len(s.Train) != 5 {
		t.Errorf("Train = %d", len(s.Train))
	}
	rec := func(user string, h []domain.Event) []string {
		switch user {
		case "u1":
			if len(h) != 1 || h[0].ItemID != "B" {
				t.Errorf("historial de u1 = %+v", h)
			}
			return []string{"Z", "A"} // rango 2
		case "u3":
			return []string{"Y"} // rango 1
		default:
			return []string{"1", "2", "3", "Q"} // fuera del top-3
		}
	}
	m := Evaluate(s, rec, 3)
	if m.Users != 3 || !near(m.HitAt, 2.0/3) || !near(m.Recall, 2.0/3) || !near(m.MRR, (0.5+1)/3) {
		t.Errorf("métricas = %+v", m)
	}
	if empty := Evaluate(Split{}, rec, 10); empty != (Metrics{}) {
		t.Errorf("sin personas = %+v", empty)
	}
}

func TestTopKDropsZeroAndSortsTies(t *testing.T) {
	got := topK([]Scored{{"b", 1}, {"a", 1}, {"c", 0}, {"d", 2}}, 10)
	if len(got) != 3 || got[0].ItemID != "d" || got[1].ItemID != "a" || got[2].ItemID != "b" {
		t.Errorf("topK = %+v", got)
	}
}
