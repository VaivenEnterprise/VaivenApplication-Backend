package synth

import (
	"reflect"
	"strings"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

func TestDeterministic(t *testing.T) {
	cfg := Default()
	a, b := Items(cfg), Items(cfg)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Items no es determinista")
	}
	if !reflect.DeepEqual(Interactions(a, cfg), Interactions(b, cfg)) {
		t.Fatal("Interactions no es determinista")
	}
	other := cfg
	other.Seed++
	if reflect.DeepEqual(Interactions(a, cfg), Interactions(a, other)) {
		t.Error("otra semilla debe dar otros datos")
	}
}

func TestShape(t *testing.T) {
	cfg := Default()
	items := Items(cfg)
	if len(items) != cfg.Items {
		t.Fatalf("objetos = %d", len(items))
	}
	for _, it := range items {
		if !domain.ValidObjectID(it.ID) || it.Category == "" || it.Municipality == "" {
			t.Fatalf("objeto = %+v", it)
		}
		if (it.Modality == "loan") != (it.PricePerDayCents == 0) {
			t.Errorf("préstamo sin tarifa: %+v", it)
		}
	}
	events := Interactions(items, cfg)
	if len(events) != cfg.Users*cfg.EventsPerUser {
		t.Fatalf("eventos = %d", len(events))
	}
	requests := 0
	for i, e := range events {
		if !strings.HasPrefix(e.UserID, "demo-") || e.Weight != e.Type.Weight() {
			t.Fatalf("evento = %+v", e)
		}
		if i > 0 && !e.At.After(events[i-1].At) {
			t.Fatal("las fechas deben crecer")
		}
		if e.Type == domain.EventRequest {
			requests++
		}
	}
	if share := float64(requests) / float64(len(events)); share < 0.1 || share > 0.2 {
		t.Errorf("solicitudes = %.2f, se esperaba ~0.15", share)
	}
}

// Cada persona interactúa sobre todo con sus categorías preferidas: es el
// patrón que el modelo debe aprender.
func TestUsersHaveTastes(t *testing.T) {
	cfg := Default()
	items := Items(cfg)
	cat := map[string]string{}
	for _, it := range items {
		cat[it.ID] = it.Category
	}
	byUser := map[string]map[string]int{}
	for _, e := range Interactions(items, cfg) {
		if byUser[e.UserID] == nil {
			byUser[e.UserID] = map[string]int{}
		}
		byUser[e.UserID][cat[e.ItemID]]++
	}
	sum := 0.0
	for _, cats := range byUser {
		top := 0
		for _, n := range cats {
			top = max(top, n)
		}
		sum += float64(top) / float64(cfg.EventsPerUser)
	}
	if avg := sum / float64(len(byUser)); avg < 0.4 {
		t.Errorf("concentración media en la categoría principal = %.2f", avg)
	}
}

func TestEmptyInputs(t *testing.T) {
	cfg := Default()
	if Interactions(nil, cfg) != nil {
		t.Error("sin objetos")
	}
	cfg.Users = 0
	if Interactions(Items(Default()), cfg) != nil {
		t.Error("sin personas")
	}
	tiny := Config{Items: 3, Users: 2, EventsPerUser: 2, Seed: 1}
	if len(Interactions(Items(tiny), tiny)) != 4 {
		t.Error("configuración mínima (sin categorías ni municipios)")
	}
}
