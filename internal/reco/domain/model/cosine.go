// Package model es la matemática del recomendador (spec 015, T004; plan P-01 y
// P-02): similitud coseno colaborativa y de contenido, el híbrido, populares y
// métricas de evaluación. Son funciones puras y deterministas: no leen la hora,
// no hacen E/S y desempatan siempre por identificador, así que los mismos datos
// dan siempre el mismo resultado. Solo usa la biblioteca estándar (Art. VIII).
package model

import (
	"cmp"
	"math"
	"slices"
)

// Vector es un vector disperso: clave → valor. Las claves ausentes valen 0.
type Vector map[string]float64

// Norm es la norma euclidiana de v. Suma en orden de clave para que el
// resultado sea idéntico en cada ejecución.
func Norm(v Vector) float64 {
	s := 0.0
	for _, k := range sortedKeys(v) {
		s += v[k] * v[k]
	}
	return math.Sqrt(s)
}

// Cosine es la similitud coseno de a y b: dot(a, b) / (|a|·|b|). Es 0 si alguno
// es el vector cero.
func Cosine(a, b Vector) float64 {
	if len(b) < len(a) {
		a, b = b, a
	}
	dot := 0.0
	for _, k := range sortedKeys(a) {
		dot += a[k] * b[k]
	}
	if dot == 0 {
		return 0
	}
	return dot / (Norm(a) * Norm(b))
}

// Scored es un objeto con su puntaje.
type Scored struct {
	ItemID string
	Score  float64
}

// sortScored ordena por puntaje descendente y desempata por ItemID ascendente.
func sortScored(s []Scored) {
	slices.SortFunc(s, func(a, b Scored) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		return cmp.Compare(a.ItemID, b.ItemID)
	})
}

// better indica si a va antes que b: mayor puntaje y, a igual puntaje, menor id.
func better(a, b Scored) bool {
	return a.Score > b.Score || (a.Score == b.Score && a.ItemID < b.ItemID)
}

// collector guarda solo los k mejores mientras se recorren candidatos, sin
// juntarlos todos (plan P-11). El resultado no depende del orden de llegada.
type collector struct {
	k    int
	best []Scored
}

func newCollector(k int) *collector { return &collector{k: k, best: make([]Scored, 0, k)} }

func (c *collector) reset() { c.best = c.best[:0] }

func (c *collector) add(id string, score float64) {
	s := Scored{ItemID: id, Score: score}
	if score <= 0 || c.k <= 0 || (len(c.best) == c.k && !better(s, c.best[len(c.best)-1])) {
		return
	}
	if len(c.best) < c.k {
		c.best = append(c.best, s)
	} else {
		c.best[len(c.best)-1] = s
	}
	for i := len(c.best) - 1; i > 0 && better(c.best[i], c.best[i-1]); i-- {
		c.best[i], c.best[i-1] = c.best[i-1], c.best[i]
	}
}

// result devuelve una copia de los mejores, ya ordenados.
func (c *collector) result() []Scored { return slices.Clone(c.best) }

// topK ordena s y se queda con los k primeros con puntaje > 0.
func topK(s []Scored, k int) []Scored {
	sortScored(s)
	n := 0
	for n < len(s) && n < k && s[n].Score > 0 {
		n++
	}
	return s[:n:n]
}
