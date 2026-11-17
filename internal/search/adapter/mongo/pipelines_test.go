package mongo

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
)

// -update regenera los archivos golden de testdata/.
var update = flag.Bool("update", false, "regenera los archivos golden")

var fixedNow = time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC)

const cursorID = "6731a2b3c4d5e6f7a8b9c0d1"

func pipelineJSON(t *testing.T, p []bson.D) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("[\n")
	for i, st := range p {
		b, err := bson.MarshalExtJSON(st, true, false)
		if err != nil {
			t.Fatal(err)
		}
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, b, "  ", "  "); err != nil {
			t.Fatal(err)
		}
		buf.WriteString("  ")
		buf.Write(pretty.Bytes())
		if i < len(p)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("]\n")
	return buf.Bytes()
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden.json")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("falta %s (ejecuta go test -run TestPipelineGolden -update): %v", path, err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		t.Errorf("%s cambió; si es intencional ejecuta con -update.\n--- obtenido ---\n%s", path, got)
	}
}

func TestPipelineGolden(t *testing.T) {
	period := &domain.Period{From: "2026-11-10", To: "2026-11-12"}
	cases := map[string]domain.Query{
		"recent": {ViewerID: "u1", Limit: 20},
		"recent_filters_cursor": {ViewerID: "u1", Category: "hogar", Modality: "rent", Municipality: "Coyoacán (Centro)",
			Period: period, Limit: 10, After: &domain.Cursor{Mode: domain.ModeRecent, Key: 1794240000000, ID: cursorID}},
		"text": {ViewerID: "u1", Text: "taladro eléctrico", Limit: 20},
		"text_cursor": {ViewerID: "u1", Text: "taladro", Period: period, Limit: 5,
			After: &domain.Cursor{Mode: domain.ModeText, Key: 1.75, ID: cursorID}},
		"near": {ViewerID: "u1", Near: &domain.Point{Lat: 19.35, Lng: -99.16}, RadiusKm: 25, Limit: 20},
		"near_text_cursor": {ViewerID: "u1", Text: "bici montaña", Near: &domain.Point{Lat: 19.35, Lng: -99.16}, RadiusKm: 5,
			Limit: 20, After: &domain.Cursor{Mode: domain.ModeNear, Key: 1234.5, ID: cursorID}},
	}
	for name, q := range cases {
		t.Run(name, func(t *testing.T) {
			golden(t, name, pipelineJSON(t, BuildPipeline(q, fixedNow)))
		})
	}
}

func stageOps(p []bson.D) []string {
	ops := make([]string, 0, len(p))
	for _, st := range p {
		ops = append(ops, st[0].Key)
	}
	return ops
}

func TestPipelineShape(t *testing.T) {
	near := BuildPipeline(domain.Query{ViewerID: "u", Near: &domain.Point{Lat: 1, Lng: 2}, RadiusKm: 3, Limit: 4}, fixedNow)
	if near[0][0].Key != "$geoNear" {
		t.Errorf("$geoNear debe ser la primera etapa: %v", stageOps(near))
	}
	text := BuildPipeline(domain.Query{ViewerID: "u", Text: "x y", Limit: 4}, fixedNow)
	first := text[0][0].Value.(bson.D)
	if first[0].Key != "$text" {
		t.Errorf("$text debe ir en la primera etapa: %v", first)
	}
	recent := BuildPipeline(domain.Query{ViewerID: "u", Limit: 4}, fixedNow)
	want := "$match,$sort,$addFields,$lookup,$unwind,$match,$limit,$project"
	if got := strings.Join(stageOps(recent), ","); got != want {
		t.Errorf("etapas = %s, want %s", got, want)
	}
	// Pide limit+1 para saber si hay otra página.
	if lim := recent[len(recent)-2][0].Value; lim != int64(5) {
		t.Errorf("$limit = %v", lim)
	}
	withPeriod := BuildPipeline(domain.Query{ViewerID: "u", Limit: 4, Period: &domain.Period{From: "a", To: "b"}}, fixedNow)
	if len(withPeriod) != len(recent)+3 {
		t.Errorf("la disponibilidad agrega 3 etapas: %v", stageOps(withPeriod))
	}
}

func TestAccentInsensitive(t *testing.T) {
	rx := regexp.MustCompile("(?i)" + AccentInsensitive("Eléctrico"))
	for _, s := range []string{"electrico", "ELÉCTRICO", "taladro eléctrico", "Electrico"} {
		if !rx.MatchString(s) {
			t.Errorf("%q no coincide con %s", s, rx)
		}
	}
	if rx.MatchString("electrica") {
		t.Error("no debe coincidir con otra palabra")
	}
	// Los metacaracteres se escapan.
	rx = regexp.MustCompile(AccentInsensitive("a.b*(c)"))
	if !rx.MatchString("a.b*(c)") || rx.MatchString("axbbc") {
		t.Errorf("escape incorrecto: %s", rx)
	}
	if AccentInsensitive("ñu") != AccentInsensitive("nu") || AccentInsensitive("10") != "10" {
		t.Error("ñ y dígitos")
	}
}

func TestWordFiltersLimit(t *testing.T) {
	if n := len(wordFilters(strings.Repeat("palabra ", 20))); n != maxTextWords {
		t.Errorf("palabras = %d", n)
	}
	if mustOID("zz") != bson.NilObjectID {
		t.Error("mustOID con id inválido")
	}
}
