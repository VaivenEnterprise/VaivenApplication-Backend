package domain

import (
	"errors"
	"math"
	"strings"
	"testing"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

func ptr[T any](v T) *T { return &v }

func validRent() Item {
	return Item{
		OwnerID:            "owner1",
		Title:              "Taladro percutor",
		Description:        "Con estuche",
		Category:           "herramientas_electricas",
		Condition:          ConditionGood,
		Modality:           ModalityRent,
		DeclaredValueCents: 100_000,
		PricePerDayCents:   3_000,
		PriceSource:        PriceSuggested,
		Zone:               Zone{Municipality: "Coyoacán"},
		Photos:             []images.Image{{PublicID: "p1"}},
		Status:             StatusActive,
	}
}

func TestSuggestPrice(t *testing.T) {
	cases := []struct{ declared, want int64 }{
		{100_000, 3_000},      // CA-02: $1,000 → $30
		{5_000, 1_000},        // $50 → 3 % = $1.50 → mínimo $10
		{33_334, 1_000},       // $10.00 exactos
		{40_000, 1_500},       // $12 → $15
		{50_000, 1_500},       // $15 exactos
		{50_001, 1_500},       // $15.0003 → round $15.00 → $15
		{51_700, 2_000},       // $15.51 → $20
		{10_000_000, 300_000}, // máximo: $3,000
		{0, 1_000},
		{-5, 1_000},
	}
	for _, c := range cases {
		if got := SuggestPrice(c.declared); got != c.want {
			t.Errorf("SuggestPrice(%d) = %d, want %d", c.declared, got, c.want)
		}
		if got := SuggestPrice(c.declared); got%500 != 0 {
			t.Errorf("SuggestPrice(%d) = %d no es múltiplo de $5", c.declared, got)
		}
	}
}

func TestValidDeclaredValue(t *testing.T) {
	for v, want := range map[int64]bool{4_999: false, 5_000: true, 10_000_000: true, 10_000_001: false} {
		if ValidDeclaredValue(v) != want {
			t.Errorf("ValidDeclaredValue(%d) != %v", v, want)
		}
	}
}

func TestRoundCoord(t *testing.T) {
	cases := map[float64]float64{
		19.123456:  19.12, // CA-03
		19.125:     19.13,
		-99.162301: -99.16,
		-0.001:     0,
		0:          0,
	}
	for in, want := range cases {
		got := RoundCoord(in)
		if got != want || math.Signbit(got) != math.Signbit(want) {
			t.Errorf("RoundCoord(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	it := validRent()
	it.Title = "  Taladro  "
	it.Description = " x "
	it.UsageRules = ptr("   ")
	it.Zone = Zone{Municipality: " Coyoacán ", Neighborhood: ptr(" ")}
	it.Location = &Location{Lat: 19.123456, Lng: -99.167}
	it.Normalize()
	if it.Title != "Taladro" || it.Description != "x" || it.Zone.Municipality != "Coyoacán" {
		t.Errorf("no recortó: %+v", it)
	}
	if it.UsageRules != nil || it.Zone.Neighborhood != nil {
		t.Error("opcionales vacíos deben quedar en nil")
	}
	if *it.Location != (Location{19.12, -99.17}) {
		t.Errorf("Location = %+v", *it.Location)
	}
	it.UsageRules = ptr(" usar con cuidado ")
	it.Normalize()
	if *it.UsageRules != "usar con cuidado" {
		t.Errorf("UsageRules = %q", *it.UsageRules)
	}
}

// fields devuelve los campos con detalle de un error de validación.
func fields(t *testing.T, err error) []string {
	t.Helper()
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeValidation {
		t.Fatalf("err = %v, want validation_failed", err)
	}
	var out []string
	for _, d := range de.Details {
		out = append(out, d.Field)
	}
	return out
}

func TestValidateOK(t *testing.T) {
	loan := validRent()
	loan.Modality, loan.PricePerDayCents, loan.PriceSource = ModalityLoan, 0, PriceManual
	manual := validRent()
	manual.PriceSource, manual.PricePerDayCents = PriceManual, 100_000 // igual al valor declarado
	full := validRent()
	full.UsageRules = ptr(strings.Repeat("a", 500))
	full.Zone.Neighborhood = ptr("Del Carmen")
	full.Location = &Location{Lat: -90, Lng: 180}
	full.Photos = make([]images.Image, 5)
	full.Title = strings.Repeat("ñ", 80)
	full.Description = strings.Repeat("é", 1000)
	for name, it := range map[string]Item{"renta": validRent(), "préstamo": loan, "manual": manual, "límites": full} {
		if err := it.Validate(); err != nil {
			t.Errorf("%s: Validate() = %v", name, err)
		}
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*Item)
		field string
	}{
		{"título corto", func(i *Item) { i.Title = "ab" }, "title"},
		{"título largo", func(i *Item) { i.Title = strings.Repeat("a", 81) }, "title"},
		{"descripción larga", func(i *Item) { i.Description = strings.Repeat("a", 1001) }, "description"},
		{"categoría", func(i *Item) { i.Category = "electronica" }, "category"},
		{"estado físico", func(i *Item) { i.Condition = "roto" }, "condition"},
		{"modalidad", func(i *Item) { i.Modality = "venta" }, "modality"},
		{"valor bajo", func(i *Item) { i.DeclaredValueCents = 4_999; i.PriceSource = PriceManual; i.PricePerDayCents = 1_000 }, "declaredValueCents"},
		{"valor alto", func(i *Item) { i.DeclaredValueCents = 10_000_001 }, "declaredValueCents"},
		{"origen", func(i *Item) { i.PriceSource = "otro" }, "priceSource"},
		// CA-01: préstamo con precio > 0.
		{"préstamo con precio", func(i *Item) { i.Modality = ModalityLoan; i.PriceSource = PriceManual }, "pricePerDayCents"},
		{"préstamo sugerido", func(i *Item) { i.Modality = ModalityLoan; i.PricePerDayCents = 0 }, "priceSource"},
		{"renta menor a $10", func(i *Item) { i.PricePerDayCents = 999; i.PriceSource = PriceManual }, "pricePerDayCents"},
		{"renta mayor al valor", func(i *Item) { i.PricePerDayCents = 100_001; i.PriceSource = PriceManual }, "pricePerDayCents"},
		{"sugerido distinto", func(i *Item) { i.PricePerDayCents = 3_500 }, "pricePerDayCents"},
		{"reglas largas", func(i *Item) { i.UsageRules = ptr(strings.Repeat("a", 501)) }, "usageRules"},
		{"municipio corto", func(i *Item) { i.Zone.Municipality = "a" }, "zone.municipality"},
		{"colonia larga", func(i *Item) { i.Zone.Neighborhood = ptr(strings.Repeat("a", 61)) }, "zone.neighborhood"},
		{"latitud", func(i *Item) { i.Location = &Location{Lat: 91} }, "approxLocation.lat"},
		{"longitud", func(i *Item) { i.Location = &Location{Lng: -181} }, "approxLocation.lng"},
		{"latitud NaN", func(i *Item) { i.Location = &Location{Lat: math.NaN()} }, "approxLocation.lat"},
		{"sin fotos", func(i *Item) { i.Photos = nil }, "photos"},
		{"6 fotos", func(i *Item) { i.Photos = make([]images.Image, 6) }, "photos"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := validRent()
			c.edit(&it)
			got := fields(t, it.Validate())
			if len(got) != 1 || got[0] != c.field {
				t.Errorf("campos = %v, want [%s]", got, c.field)
			}
		})
	}
}

func TestValidateManyDetails(t *testing.T) {
	got := fields(t, Item{}.Validate())
	if len(got) < 7 {
		t.Errorf("un objeto vacío debe dar varios detalles, dio %v", got)
	}
}

func TestVisibility(t *testing.T) {
	it := validRent()
	cases := []struct {
		status Status
		viewer string
		admin  bool
		want   bool
	}{
		{StatusActive, "otro", false, true},
		{StatusPaused, "otro", false, false}, // CA-06
		{StatusPaused, "owner1", false, true},
		{StatusPaused, "otro", true, true},
		{StatusBlocked, "otro", false, false},
		{StatusBlocked, "owner1", false, true},
		{StatusRemoved, "owner1", true, false},
	}
	for _, c := range cases {
		it.Status = c.status
		if got := it.VisibleTo(c.viewer, c.admin); got != c.want {
			t.Errorf("%s visto por %s (admin=%v) = %v, want %v", c.status, c.viewer, c.admin, got, c.want)
		}
	}
	if it.ShowsDeclaredValue("otro", false) || !it.ShowsDeclaredValue("owner1", false) || !it.ShowsDeclaredValue("otro", true) {
		t.Error("ShowsDeclaredValue incorrecto")
	}
	if it.IsOwner("") {
		t.Error("un id vacío no es dueño")
	}
}

func TestOwnerChecks(t *testing.T) {
	it := validRent()
	if err := it.CheckOwnerAction("owner1"); err != nil {
		t.Errorf("dueño: %v", err)
	}
	if err := it.CheckOwnerAction("otro"); !errors.Is(err, ErrNotOwner) { // CA-05
		t.Errorf("otro: %v", err)
	}
	it.Status = StatusBlocked
	if err := it.CheckOwnerAction("owner1"); !errors.Is(err, ErrBlocked) {
		t.Errorf("bloqueado: %v", err)
	}
	if err := it.CheckOwner("owner1"); err != nil {
		t.Errorf("un bloqueado se puede eliminar: %v", err)
	}
	it.Status = StatusRemoved
	if err := it.CheckOwnerAction("owner1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("eliminado: %v", err)
	}
}

func TestWithStatus(t *testing.T) {
	it := validRent()
	if s, changed := it.WithStatus(true); s != StatusPaused || !changed {
		t.Errorf("pausar activo = %s %v", s, changed)
	}
	if s, changed := it.WithStatus(false); s != StatusActive || changed {
		t.Errorf("reanudar activo = %s %v", s, changed)
	}
	it.Status = StatusPaused
	if s, changed := it.WithStatus(false); s != StatusActive || !changed {
		t.Errorf("reanudar pausado = %s %v", s, changed)
	}
}

func TestEnumsAndCategories(t *testing.T) {
	for _, s := range []Status{StatusActive, StatusPaused, StatusBlocked} {
		if !ListableStatus(s) {
			t.Errorf("ListableStatus(%s) = false", s)
		}
	}
	if ListableStatus(StatusRemoved) || ListableStatus("x") {
		t.Error("removed y desconocidos no se listan")
	}
	if len(Categories()) != 7 || !ValidCategory("hogar") || ValidCategory("x") {
		t.Error("categorías de POL-14 incorrectas")
	}
	if !PriceManual.Valid() || PriceSource("x").Valid() || !ModalityRent.Valid() || !ConditionWorn.Valid() {
		t.Error("enumeraciones incorrectas")
	}
}
