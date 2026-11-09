package policy

import "testing"

// Fija los valores provisionales de ROADMAP §8: si una política cambia, esta
// prueba obliga a cambiarla a propósito.
func TestPolicyValues(t *testing.T) {
	ints := []struct {
		name      string
		got, want int64
	}{
		{"POL04SuggestedPercent", POL04SuggestedPercent, 3},
		{"POL04MinSuggestedCents", POL04MinSuggestedCents, 1000},
		{"POL04RoundStepCents", POL04RoundStepCents, 500},
		{"POL04MinDeclaredValueCents", POL04MinDeclaredValueCents, 5000},
		{"POL04MaxDeclaredValueCents", POL04MaxDeclaredValueCents, 10_000_000},
		{"POL01LoanPriceCents", POL01LoanPriceCents, 0},
		{"POL01MinRentPriceCents", POL01MinRentPriceCents, 1000},
		{"POL02MinDays", POL02MinDays, 1},
		{"POL02MaxDays", POL02MaxDays, 7},
		{"POL13MinItemPhotos", POL13MinItemPhotos, 1},
		{"POL13MaxItemPhotos", POL13MaxItemPhotos, 5},
	}
	for _, c := range ints {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestPOL14Categories(t *testing.T) {
	want := []string{
		"herramientas_manuales", "herramientas_electricas", "construccion_jardineria",
		"movilidad", "deportes", "hogar", "otros",
	}
	got := POL14Categories()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, k := range want {
		if got[i].Key != k || got[i].Label == "" {
			t.Errorf("categoría %d = %+v, want key %q con etiqueta", i, got[i], k)
		}
		if !POL14Valid(k) {
			t.Errorf("POL14Valid(%q) = false", k)
		}
	}
	// Una copia modificada no afecta a la siguiente llamada.
	got[0].Key = "x"
	if POL14Categories()[0].Key != want[0] {
		t.Error("POL14Categories devuelve estado compartido")
	}
	for _, k := range []string{"", "Hogar", "electronica"} {
		if POL14Valid(k) {
			t.Errorf("POL14Valid(%q) = true", k)
		}
	}
}
