// Package domain valida lo que devuelve el modelo de IA antes de que llegue a
// la persona (spec 014, RF-04, RF-10 y RF-17; plan P-03). El modelo nunca
// decide: lo que no cumple el esquema se descarta o se recorta. No conoce al
// proveedor ni usa infraestructura (Art. III).
package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// Límites del esquema (spec 014, RF-04 y RF-10).
const (
	MaxTitleRunes       = 80
	MaxDescriptionRunes = 1000
	MaxSummaryRunes     = 600
	MaxNeeds            = 8
	MaxNeedNameRunes    = 60
	MaxSearchTerms      = 3
	MaxSearchTermRunes  = 40
	MaxSteps            = 10
	MaxStepRunes        = 200
	MaxSafetyNotes      = 3
)

// ErrNothingValid indica que la respuesta del modelo no dejó nada utilizable;
// el caso de uso responde service_unavailable.
var ErrNothingValid = errors.New("ai: la respuesta del modelo no tiene contenido válido")

// ErrMalformed indica que la respuesta del modelo no es JSON válido.
var ErrMalformed = errors.New("ai: la respuesta del modelo no es JSON válido")

// Suggestion es la propuesta de la parte A (título y categoría a partir de una
// foto). Category vacía significa que no hubo una categoría válida.
type Suggestion struct {
	Title       string `json:"title"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// Need es algo que el proyecto necesita (parte B). Solo lo propone el modelo;
// quién lo tiene lo decide la búsqueda, nunca el modelo (RF-11).
type Need struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	SearchTerms []string `json:"searchTerms"`
	Optional    bool     `json:"optional"`
}

// Plan es la respuesta del modelo para la parte B.
type Plan struct {
	Summary     string   `json:"summary"`
	Needs       []Need   `json:"needs"`
	Steps       []string `json:"steps"`
	SafetyNotes []string `json:"safetyNotes"`
}

// ParseSuggestion decodifica y valida la salida de la parte A. Los campos que
// no estén en el esquema se ignoran.
func ParseSuggestion(data []byte) (Suggestion, error) {
	var s Suggestion
	if err := json.Unmarshal(data, &s); err != nil {
		return Suggestion{}, ErrMalformed
	}
	return ValidateSuggestion(s)
}

// ValidateSuggestion recorta y depura una sugerencia: el título es obligatorio
// (≤ 80); una categoría que no sea de POL-14 se omite; la descripción, ≤ 1000.
func ValidateSuggestion(s Suggestion) (Suggestion, error) {
	out := Suggestion{
		Title:       clip(s.Title, MaxTitleRunes),
		Description: clip(s.Description, MaxDescriptionRunes),
	}
	if c := strings.TrimSpace(s.Category); policy.POL14Valid(c) {
		out.Category = c
	}
	if out.Title == "" {
		return Suggestion{}, ErrNothingValid
	}
	return out, nil
}

// ParsePlan decodifica y valida la salida de la parte B.
func ParsePlan(data []byte) (Plan, error) {
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return Plan{}, ErrMalformed
	}
	return ValidatePlan(p)
}

// ValidatePlan descarta las necesidades inválidas (sin nombre, con categoría
// fuera de POL-14 o sin términos de búsqueda), recorta textos y cantidades, y
// falla solo si no queda resumen, ni necesidades, ni pasos (CA-12). Una
// negativa válida (RF-14) trae resumen y listas vacías.
func ValidatePlan(p Plan) (Plan, error) {
	out := Plan{
		Summary:     clip(p.Summary, MaxSummaryRunes),
		Needs:       []Need{},
		Steps:       clipList(p.Steps, MaxSteps, MaxStepRunes),
		SafetyNotes: clipList(p.SafetyNotes, MaxSafetyNotes, MaxStepRunes),
	}
	for _, n := range p.Needs {
		if len(out.Needs) == MaxNeeds {
			break
		}
		if v, ok := validNeed(n); ok {
			out.Needs = append(out.Needs, v)
		}
	}
	if out.Summary == "" && len(out.Needs) == 0 && len(out.Steps) == 0 {
		return Plan{}, ErrNothingValid
	}
	return out, nil
}

func validNeed(n Need) (Need, bool) {
	name := clip(n.Name, MaxNeedNameRunes)
	cat := strings.TrimSpace(n.Category)
	if name == "" || !policy.POL14Valid(cat) {
		return Need{}, false
	}
	terms := clipList(n.SearchTerms, MaxSearchTerms, MaxSearchTermRunes)
	if len(terms) == 0 {
		return Need{}, false
	}
	return Need{Name: name, Category: cat, SearchTerms: terms, Optional: n.Optional}, true
}

// clipList quita espacios y vacíos, recorta cada texto a maxRunes y la lista a
// maxItems. Nunca devuelve nil, para que el JSON sea [] y no null.
func clipList(in []string, maxItems, maxRunes int) []string {
	out := []string{}
	for _, s := range in {
		if len(out) == maxItems {
			break
		}
		if c := clip(s, maxRunes); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// clip quita espacios en los extremos y recorta a maxRunes runas sin partir
// caracteres (los textos son en español, con acentos).
func clip(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:maxRunes]))
}
