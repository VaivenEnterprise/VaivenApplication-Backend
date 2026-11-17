package model

import (
	"cmp"
	"math"
	"slices"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// MaxItemsPerUser acota los objetos de una persona que entran al colaborativo:
// el costo crece con el cuadrado de esa cantidad (plan P-11). Se quedan los de
// mayor peso y, a igual peso, los más recientes.
const MaxItemsPerUser = 200

// Matrix es la matriz persona × objeto dispersa: Matrix[persona][objeto] es el
// peso máximo de sus interacciones con ese objeto (RF-02).
type Matrix map[string]map[string]float64

// BuildMatrix arma la matriz con el peso máximo por par persona-objeto.
func BuildMatrix(events []domain.Event) Matrix {
	m := Matrix{}
	type key struct{ user, item string }
	last := map[key]int64{}
	for _, e := range events {
		row := m[e.UserID]
		if row == nil {
			row = map[string]float64{}
			m[e.UserID] = row
		}
		if e.Weight > row[e.ItemID] {
			row[e.ItemID] = e.Weight
		}
		k := key{e.UserID, e.ItemID}
		if t := e.At.UnixNano(); t > last[k] {
			last[k] = t
		}
	}
	for user, row := range m {
		if len(row) <= MaxItemsPerUser {
			continue
		}
		items := make([]string, 0, len(row))
		for it := range row {
			items = append(items, it)
		}
		slices.SortFunc(items, func(a, b string) int {
			if c := cmp.Compare(row[b], row[a]); c != 0 {
				return c
			}
			if c := cmp.Compare(last[key{user, b}], last[key{user, a}]); c != 0 {
				return c
			}
			return cmp.Compare(a, b)
		})
		for _, it := range items[MaxItemsPerUser:] {
			delete(row, it)
		}
	}
	return m
}

// CollaborativeNeighbors calcula, para cada objeto con interacciones, sus k
// vecinos más parecidos por coseno objeto-objeto (RF-02):
//
//	cf(i, j) = Σ_u w(u,i)·w(u,j) / (‖i‖·‖j‖)
//
// donde ‖i‖ es la norma de la columna del objeto i. Solo se suman personas que
// tocaron los dos objetos, así que el costo es Σ_u n_u² y no objetos².
func CollaborativeNeighbors(m Matrix, k int) map[string][]Scored {
	// Todo se recorre en orden fijo: sumar flotantes en otro orden cambia los
	// últimos decimales y podría cambiar un desempate (P-01).
	norms := map[string]float64{}
	dots := map[string]map[string]float64{}
	for _, u := range sortedKeys(m) {
		items := sortedKeys(m[u])
		for _, it := range items {
			norms[it] += m[u][it] * m[u][it]
		}
		for a := 0; a < len(items); a++ {
			wa := m[u][items[a]]
			for b := a + 1; b < len(items); b++ {
				p := wa * m[u][items[b]]
				addDot(dots, items[a], items[b], p)
				addDot(dots, items[b], items[a], p)
			}
		}
	}
	out := make(map[string][]Scored, len(dots))
	best := newCollector(k)
	for i, row := range dots {
		ni := norms[i]
		best.reset()
		for j, d := range row {
			best.add(j, d/math.Sqrt(ni*norms[j]))
		}
		if len(best.best) > 0 {
			out[i] = best.result()
		}
	}
	return out
}

func addDot(dots map[string]map[string]float64, i, j string, p float64) {
	row := dots[i]
	if row == nil {
		row = map[string]float64{}
		dots[i] = row
	}
	row[j] += p
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
