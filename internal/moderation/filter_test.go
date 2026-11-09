package moderation

import (
	"context"
	"strings"
	"testing"
	"time"
)

func check(t *testing.T, f *Filter, text string) Result {
	t.Helper()
	res, err := f.Check(context.Background(), text)
	if err != nil {
		t.Fatalf("Check(%q): %v", text, err)
	}
	return res
}

func TestFilterBlocksEachCategory(t *testing.T) { // RF-07, CA-03
	f := NewFilter()
	cases := []struct {
		text string
		cat  string
	}{
		{"Vendo pistola calibre 9", CategoryWeapons},
		{"Rifle de caza", CategoryWeapons},
		{"Compro ARMAS DE FUEGO", CategoryWeapons},
		{"AK-47 en buen estado", CategoryWeapons},
		{"Tengo munición", CategoryWeapons},
		{"contenido pornográfico", CategorySexual},
		{"Servicios sexuales a domicilio", CategorySexual},
		{"fotos XXX", CategorySexual},
		{"vendo marihuana", CategoryDrugs},
		{"Cocaína de calidad", CategoryDrugs},
		{"se renta droga", CategoryDrugs},
		{"MDMA y LSD", CategoryDrugs},
		{"mercancía robada barata", CategoryIllegal},
		{"hago documentos falsos", CategoryIllegal},
		{"lavado de dinero", CategoryIllegal},
	}
	for _, c := range cases {
		res := check(t, f, c.text)
		if res.Allowed || res.Category != c.cat {
			t.Errorf("Check(%q) = %+v, quiero bloqueado como %q", c.text, res, c.cat)
			continue
		}
		if !strings.HasPrefix(res.Reason, "Contiene contenido no permitido: ") || !strings.HasSuffix(res.Reason, ".") {
			t.Errorf("Check(%q).Reason = %q", c.text, res.Reason)
		}
	}
}

func TestFilterReasonNamesTheCategoryInSpanish(t *testing.T) {
	f := NewFilter()
	want := map[string]string{
		"pistola":     "Contiene contenido no permitido: armas.",
		"porno":       "Contiene contenido no permitido: contenido sexual.",
		"cocaina":     "Contiene contenido no permitido: drogas.",
		"contrabando": "Contiene contenido no permitido: actividades ilegales.",
	}
	for text, reason := range want {
		if got := check(t, f, text).Reason; got != reason {
			t.Errorf("Check(%q).Reason = %q, quiero %q", text, got, reason)
		}
	}
}

func TestFilterNormalizesCaseAndAccents(t *testing.T) {
	f := NewFilter()
	for _, text := range []string{"COCAINA", "cocaína", "CoCaÍnA", "revolver", "REVÓLVER", "Pornografía", "pornografia", "heroína", "HEROINA"} {
		if check(t, f, text).Allowed {
			t.Errorf("Check(%q) debe bloquear sin importar mayúsculas ni acentos", text)
		}
	}
}

func TestFilterMatchesWholeWordsOnly(t *testing.T) {
	f := NewFilter()
	// Contienen un término prohibido como parte de otra palabra.
	for _, text := range []string{"camiseta xxxl", "drogaría Guadalajara", "riflero", "pistolero de juguete", "opiniones", "lsdx", "escorting"} {
		if !check(t, f, text).Allowed {
			t.Errorf("Check(%q) no debe bloquear una palabra distinta", text)
		}
	}
}

func TestFilterMatchesPhrasesAcrossPunctuation(t *testing.T) {
	f := NewFilter()
	for _, text := range []string{"lavado,   de dinero", "Armas-de-fuego", "ak\t47", "tráfico de\npersonas"} {
		if check(t, f, text).Allowed {
			t.Errorf("Check(%q) debe bloquear la frase", text)
		}
	}
	// Las palabras sueltas de una frase no bastan.
	for _, text := range []string{"lavado de ropa", "dinero en efectivo", "armas de juguete", "tráfico pesado en la ciudad"} {
		if !check(t, f, text).Allowed {
			t.Errorf("Check(%q) no debe bloquear: la frase completa no aparece", text)
		}
	}
}

func TestFilterKeepsEnyeDistinct(t *testing.T) {
	f := newFilter(map[string][]string{CategorySexual: {"ano"}}, nil)
	if check(t, f, "el año pasado").Allowed != true {
		t.Error("«año» no debe confundirse con «ano»")
	}
	if check(t, f, "el ano").Allowed {
		t.Error("«ano» sí es el término")
	}
}

// normalPhrases son textos comunes de publicaciones y mensajes que nunca se
// deben bloquear (sin falsos positivos).
var normalPhrases = []string{
	"Taladro inalámbrico con dos baterías",
	"Bicicleta de montaña rodada 29, casi nueva",
	"Pistola de calor para manualidades y estaño",
	"Pistola de silicón con 10 barras incluidas",
	"Pistola de pintura eléctrica para muebles",
	"Tienda de campaña para cuatro personas",
	"Cámara réflex con lente 50 mm",
	"Hola, ¿sigue disponible para el sábado?",
	"Te espero en el metro Chapultepec a las 5",
	"Gracias, todo salió muy bien con el préstamo",
	"Proyector con control remoto y cable HDMI",
	"Escalera de aluminio de tres metros",
	"Granada y mango de temporada", // la fruta
	"Disfraz de pirata para fiesta infantil",
	"Cristal templado para ventana",
	"Máquina de coser en excelente estado",
	"Playera talla XXL color negro",
	"Pistola de impacto para llantas",
	"El año pasado la usé en una mudanza",
	"Cortadora de césped, la devuelvo limpia",
}

func TestFilterHasNoFalsePositivesOnNormalPhrases(t *testing.T) {
	if len(normalPhrases) < 20 {
		t.Fatalf("la prueba pide 20 frases normales, hay %d", len(normalPhrases))
	}
	f := NewFilter()
	for _, text := range normalPhrases {
		if res := check(t, f, text); !res.Allowed {
			t.Errorf("falso positivo en %q: %+v", text, res)
		}
	}
}

func TestFilterExceptionsOnlyCoverThePhrase(t *testing.T) {
	f := NewFilter()
	// La excepción no debe ocultar otra pistola en el mismo texto.
	if check(t, f, "pistola de calor y también pistola calibre 38").Allowed {
		t.Error("una pistola real junto a una herramienta debe bloquearse")
	}
	if check(t, f, "pistola de juguete y cocaína").Allowed {
		t.Error("la excepción no debe ocultar otros términos")
	}
}

func TestFilterEmptyAndNoWords(t *testing.T) {
	f := NewFilter()
	for _, text := range []string{"", "   ", "¡¡¡???", "12345"} {
		if !check(t, f, text).Allowed {
			t.Errorf("Check(%q) debe permitirse", text)
		}
	}
}

func TestFilterIsFast(t *testing.T) { // RNF-02: < 5 ms por 1,000 caracteres
	f := NewFilter()
	text := strings.Repeat("Taladro inalámbrico con dos baterías y cargador. ", 21)[:1000]
	check(t, f, text) // calienta
	const runs = 50
	start := time.Now()
	for i := 0; i < runs; i++ {
		check(t, f, text)
	}
	if avg := time.Since(start) / runs; avg > 5*time.Millisecond {
		t.Errorf("promedio %v por 1,000 caracteres, el máximo es 5 ms", avg)
	}
}

func TestBlocklistIsWellFormed(t *testing.T) {
	for cat, terms := range blocklist {
		if _, ok := categoryLabel[cat]; !ok {
			t.Errorf("categoría %q sin etiqueta en español", cat)
		}
		if len(terms) == 0 {
			t.Errorf("categoría %q vacía", cat)
		}
		for _, term := range terms {
			if len(tokenize(term)) == 0 {
				t.Errorf("término %q no produce palabras", term)
			}
		}
	}
	for _, cat := range []string{CategorySexual, CategoryWeapons, CategoryDrugs, CategoryIllegal} {
		if len(blocklist[cat]) == 0 {
			t.Errorf("falta la categoría %q de POL-16", cat)
		}
	}
}
