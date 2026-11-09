package domain

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

const today = "2026-11-08"
const oid = "6731a2b3c4d5e6f7a8b9c0d1"

func fieldsOf(t *testing.T, err error) []string {
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

func TestParseDefaults(t *testing.T) {
	q, err := Parse(RawQuery{}, "u1", today)
	if err != nil {
		t.Fatal(err)
	}
	if q.ViewerID != "u1" || q.Limit != DefaultLimit || q.Mode() != ModeRecent || q.Period != nil || q.Near != nil {
		t.Errorf("q = %+v", q)
	}
}

func TestParseFull(t *testing.T) {
	c := Cursor{Mode: ModeNear, Key: 1234.5, ID: oid}
	q, err := Parse(RawQuery{
		Q: "  taladro ", Category: "hogar", Modality: "rent", Municipality: " Coyoacán ",
		From: "2026-11-08", To: "2026-11-14", Lat: "19.35", Lng: "-99.16", RadiusKm: "10",
		Limit: "50", Cursor: c.Encode(),
	}, "u1", today)
	if err != nil {
		t.Fatal(err)
	}
	if q.Text != "taladro" || q.Municipality != "Coyoacán" || q.Period.From != "2026-11-08" || q.Period.To != "2026-11-14" ||
		q.Near.Lat != 19.35 || q.RadiusKm != 10 || q.Limit != 50 || *q.After != c || q.Mode() != ModeNear {
		t.Errorf("q = %+v", q)
	}
	q, _ = Parse(RawQuery{Q: "taladro", Lat: "1", Lng: "2"}, "u", today)
	if q.RadiusKm != DefaultRadiusKm {
		t.Errorf("radio por defecto = %d", q.RadiusKm)
	}
	if q, _ = Parse(RawQuery{Q: "taladro"}, "u", today); q.Mode() != ModeText {
		t.Errorf("modo texto = %v", q.Mode())
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  RawQuery
		want string
	}{
		{"texto corto", RawQuery{Q: "a"}, "q"},
		{"texto largo", RawQuery{Q: strings.Repeat("a", 81)}, "q"},
		{"categoría", RawQuery{Category: "electronica"}, "category"},
		{"modalidad", RawQuery{Modality: "venta"}, "modality"},
		{"municipio", RawQuery{Municipality: "a"}, "municipality"},
		{"solo from", RawQuery{From: "2026-11-09"}, "to"},
		{"solo to", RawQuery{To: "2026-11-09"}, "from"},
		{"from formato", RawQuery{From: "09/11/2026", To: "2026-11-10"}, "from"},
		{"to formato", RawQuery{From: "2026-11-09", To: "mañana"}, "to"},
		{"from pasado", RawQuery{From: "2026-11-07", To: "2026-11-09"}, "from"},
		{"to antes de from", RawQuery{From: "2026-11-10", To: "2026-11-09"}, "to"},
		{"más de 7 días", RawQuery{From: "2026-11-08", To: "2026-11-15"}, "to"},
		{"lat sin lng", RawQuery{Lat: "19.3"}, "lng"}, // CA-06
		{"lng sin lat", RawQuery{Lng: "-99"}, "lat"},
		{"lat fuera", RawQuery{Lat: "91", Lng: "0"}, "lat"},
		{"lng no número", RawQuery{Lat: "1", Lng: "x"}, "lng"},
		{"lat NaN", RawQuery{Lat: "NaN", Lng: "0"}, "lat"},
		{"radio fuera", RawQuery{Lat: "1", Lng: "1", RadiusKm: "51"}, "radiusKm"},
		{"radio sin ubicación", RawQuery{RadiusKm: "5"}, "radiusKm"},
		{"limit 0", RawQuery{Limit: "0"}, "limit"},
		{"limit texto", RawQuery{Limit: "diez"}, "limit"},
		{"cursor basura", RawQuery{Cursor: "%%%"}, "cursor"},
		{"cursor de otro modo", RawQuery{Cursor: Cursor{Mode: ModeText, Key: 1, ID: oid}.Encode()}, "cursor"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.raw, "u", today)
			got := fieldsOf(t, err)
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("campos = %v, want [%s]", got, c.want)
			}
		})
	}
	// Varios errores a la vez.
	_, err := Parse(RawQuery{Q: "a", Lat: "1", Limit: "x"}, "u", today)
	if got := fieldsOf(t, err); len(got) != 3 {
		t.Errorf("campos = %v", got)
	}
	// Exactamente 7 días y empezar hoy está permitido.
	if _, err := Parse(RawQuery{From: today, To: "2026-11-14"}, "u", today); err != nil {
		t.Errorf("7 días desde hoy: %v", err)
	}
}

func TestCursor(t *testing.T) {
	c := Cursor{Mode: ModeText, Key: 7.25, ID: oid}
	got, err := DecodeCursor(c.Encode())
	if err != nil || got != c {
		t.Fatalf("ida y vuelta = %+v %v", got, err)
	}
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for name, s := range map[string]string{
		"vacío":        "",
		"largo":        strings.Repeat("a", MaxCursorLen+1),
		"no base64":    "***",
		"no JSON":      enc("hola"),
		"modo":         enc(`{"m":"x","k":1,"id":"` + oid + `"}`),
		"id":           enc(`{"m":"text","k":1,"id":"zz"}`),
		"clave neg":    enc(`{"m":"text","k":-1,"id":"` + oid + `"}`),
		"id mayúscula": enc(`{"m":"text","k":1,"id":"` + strings.ToUpper(oid) + `"}`),
	} {
		if _, err := DecodeCursor(s); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	for _, m := range []Mode{ModeRecent, ModeText, ModeNear} {
		if back, ok := parseMode(m.String()); !ok || back != m {
			t.Errorf("modo %v", m)
		}
	}
}

func TestCardHelpers(t *testing.T) {
	var c Card
	if c.DistanceKm() != nil {
		t.Error("sin distancia")
	}
	m := 1449.0
	c.DistanceMeters = &m
	if got := *c.DistanceKm(); got != 1.4 {
		t.Errorf("DistanceKm = %v", got)
	}
	m = 1450
	if got := *c.DistanceKm(); got != 1.5 {
		t.Errorf("DistanceKm = %v", got)
	}
	if RatingAverage(10, 0) != nil || *RatingAverage(9, 2) != 4.5 {
		t.Error("RatingAverage")
	}
}
