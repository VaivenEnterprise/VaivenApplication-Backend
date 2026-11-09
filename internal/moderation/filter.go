package moderation

import (
	"context"
	"strings"
	"unicode"
)

// Filter es el Checker real (spec 011, RF-07, POL-16): normaliza el texto
// (minúsculas y sin acentos) y busca términos y frases prohibidos por palabra
// completa. No usa red ni dependencias externas; es seguro para uso
// concurrente (solo lee sus tablas).
type Filter struct {
	single     map[string]string  // palabra → categoría
	phrases    map[string][]entry // primera palabra → frases que empiezan con ella
	exceptions map[string][][]string
}

type entry struct {
	category string
	words    []string
}

// NewFilter crea el filtro con la lista de blocklist.go.
func NewFilter() *Filter { return newFilter(blocklist, exceptions) }

func newFilter(lists map[string][]string, allowed []string) *Filter {
	f := &Filter{single: map[string]string{}, phrases: map[string][]entry{}, exceptions: map[string][][]string{}}
	for cat, terms := range lists {
		for _, t := range terms {
			words := tokenize(t)
			switch len(words) {
			case 0:
			case 1:
				f.single[words[0]] = cat
			default:
				f.phrases[words[0]] = append(f.phrases[words[0]], entry{cat, words})
			}
		}
	}
	for _, e := range allowed {
		if words := tokenize(e); len(words) > 0 {
			f.exceptions[words[0]] = append(f.exceptions[words[0]], words)
		}
	}
	return f
}

var _ Checker = (*Filter)(nil)

// Check devuelve Allowed=false con la categoría y el motivo en es-MX si el
// texto contiene algo de la lista. Un texto vacío se permite (otras reglas
// exigen que no lo esté).
func (f *Filter) Check(_ context.Context, text string) (Result, error) {
	words := tokenize(text)
	skip := f.maskExceptions(words)
	for i, w := range words {
		if skip[i] {
			continue
		}
		if cat, ok := f.single[w]; ok {
			return blocked(cat), nil
		}
		for _, p := range f.phrases[w] {
			if matchAt(words, skip, i, p.words) {
				return blocked(p.category), nil
			}
		}
	}
	return Result{Allowed: true}, nil
}

func blocked(category string) Result {
	return Result{
		Allowed:  false,
		Category: category,
		Reason:   "Contiene contenido no permitido: " + categoryLabel[category] + ".",
	}
}

// maskExceptions marca las palabras que forman parte de una frase permitida.
func (f *Filter) maskExceptions(words []string) []bool {
	if len(f.exceptions) == 0 {
		return make([]bool, len(words))
	}
	skip := make([]bool, len(words))
	for i, w := range words {
		for _, e := range f.exceptions[w] {
			if matchAt(words, nil, i, e) {
				for j := range e {
					skip[i+j] = true
				}
			}
		}
	}
	return skip
}

// matchAt indica si phrase aparece en words desde i sin tocar palabras
// marcadas en skip (que puede ser nil).
func matchAt(words []string, skip []bool, i int, phrase []string) bool {
	if i+len(phrase) > len(words) {
		return false
	}
	for j, p := range phrase {
		if words[i+j] != p || (skip != nil && skip[i+j]) {
			return false
		}
	}
	return true
}

// tokenize pasa el texto a minúsculas, le quita los acentos del español (la ñ
// se conserva: "año" no es "ano") y lo separa en palabras de letras y dígitos.
func tokenize(text string) []string {
	var words []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			words = append(words, b.String())
			b.Reset()
		}
	}
	for _, r := range text {
		r = unicode.ToLower(r)
		if plain, ok := accents[r]; ok {
			r = plain
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return words
}

var accents = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
}
