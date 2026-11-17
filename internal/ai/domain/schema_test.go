package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseSuggestionValid(t *testing.T) {
	s, err := ParseSuggestion([]byte(`{"title":"  Taladro inalámbrico  ","category":"herramientas_electricas","description":"Con dos baterías","extra":"ignorado"}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Taladro inalámbrico" || s.Category != "herramientas_electricas" || s.Description != "Con dos baterías" {
		t.Errorf("sugerencia = %+v", s)
	}
}

func TestSuggestionDropsInvalidCategoryAndClipsTitle(t *testing.T) {
	long := strings.Repeat("ñ", 100)
	s, err := ValidateSuggestion(Suggestion{Title: long, Category: "armas"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Category != "" {
		t.Errorf("categoría inválida no se omitió: %q", s.Category)
	}
	if utf8.RuneCountInString(s.Title) != MaxTitleRunes || !utf8.ValidString(s.Title) {
		t.Errorf("título recortado mal: %d runas", utf8.RuneCountInString(s.Title))
	}
}

func TestSuggestionWithoutTitleIsNothingValid(t *testing.T) {
	if _, err := ValidateSuggestion(Suggestion{Title: "   ", Category: "hogar"}); !errors.Is(err, ErrNothingValid) {
		t.Errorf("err = %v", err)
	}
	if _, err := ParseSuggestion([]byte(`no es json`)); !errors.Is(err, ErrMalformed) {
		t.Errorf("err = %v", err)
	}
}

func TestParsePlanValid(t *testing.T) {
	raw := `{"summary":"Para pintar necesitas escalera y rodillo.",
	 "needs":[{"name":"Escalera de tijera","category":"hogar","searchTerms":["escalera"],"optional":false},
	          {"name":"Lona","category":"hogar","searchTerms":["lona","plástico"],"optional":true}],
	 "steps":["Cubre el piso.","Pinta dos manos."],"safetyNotes":["Abre la escalera por completo."],
	 "itemIds":["inventado"]}`
	p, err := ParsePlan([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Needs) != 2 || p.Needs[1].Optional != true || len(p.Steps) != 2 || len(p.SafetyNotes) != 1 {
		t.Errorf("plan = %+v", p)
	}
	// Un campo fuera del esquema (itemIds) nunca sale: el modelo no decide quién tiene qué.
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "inventado") {
		t.Errorf("se filtró un campo fuera del esquema: %s", b)
	}
}

func TestPlanDropsInvalidNeedsAndClipsToEight(t *testing.T) {
	needs := []Need{
		{Name: "", Category: "hogar", SearchTerms: []string{"x"}},            // sin nombre
		{Name: "Pistola", Category: "armas", SearchTerms: []string{"x"}},     // categoría fuera de POL-14
		{Name: "Rodillo", Category: "hogar", SearchTerms: []string{" ", ""}}, // sin términos
	}
	for i := 0; i < 12; i++ {
		needs = append(needs, Need{Name: "Cosa", Category: "otros", SearchTerms: []string{"a", "b", "c", "d"}})
	}
	p, err := ValidatePlan(Plan{Summary: "ok", Needs: needs})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Needs) != MaxNeeds {
		t.Fatalf("necesidades = %d, se esperaban %d", len(p.Needs), MaxNeeds)
	}
	for _, n := range p.Needs {
		if n.Name != "Cosa" || len(n.SearchTerms) != MaxSearchTerms {
			t.Errorf("necesidad inválida o sin recortar: %+v", n)
		}
	}
}

func TestPlanClipsStepsAndNotes(t *testing.T) {
	steps := []string{" ", strings.Repeat("a", 300)}
	for i := 0; i < 15; i++ {
		steps = append(steps, "paso")
	}
	p, err := ValidatePlan(Plan{Steps: steps, SafetyNotes: []string{"1", "2", "3", "4"}, Summary: strings.Repeat("é", 700)})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != MaxSteps || utf8.RuneCountInString(p.Steps[0]) != MaxStepRunes {
		t.Errorf("pasos = %d, primero %d runas", len(p.Steps), utf8.RuneCountInString(p.Steps[0]))
	}
	if len(p.SafetyNotes) != MaxSafetyNotes || utf8.RuneCountInString(p.Summary) != MaxSummaryRunes {
		t.Errorf("avisos = %d, resumen %d runas", len(p.SafetyNotes), utf8.RuneCountInString(p.Summary))
	}
}

func TestRefusalIsValidWithEmptyLists(t *testing.T) {
	p, err := ValidatePlan(Plan{Summary: "Solo puedo ayudarte con proyectos que usen objetos prestables."})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	if !strings.Contains(string(b), `"needs":[]`) || !strings.Contains(string(b), `"steps":[]`) || !strings.Contains(string(b), `"safetyNotes":[]`) {
		t.Errorf("las listas vacías deben ser [] y no null: %s", b)
	}
}

func TestPlanNothingValid(t *testing.T) {
	cases := []Plan{
		{},
		{Summary: "  ", Needs: []Need{{Name: "x", Category: "nope", SearchTerms: []string{"x"}}}, Steps: []string{" "}},
	}
	for i, c := range cases {
		if _, err := ValidatePlan(c); !errors.Is(err, ErrNothingValid) {
			t.Errorf("caso %d: err = %v", i, err)
		}
	}
	if _, err := ParsePlan([]byte(`{"summary":`)); !errors.Is(err, ErrMalformed) {
		t.Errorf("JSON roto: err = %v", err)
	}
}
