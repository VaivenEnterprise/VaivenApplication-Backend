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
//
// Memoria (plan P-11): un índice invertido objeto → personas y un acumulador
// denso que se reutiliza para cada objeto; no se guarda la matriz objeto ×
// objeto. Todo se recorre en orden fijo (personas y objetos ordenados): sumar
// flotantes en otro orden cambia los últimos decimales y podría cambiar un
// desempate (P-01). Por eso cf(i, j) y cf(j, i) son idénticos.
func CollaborativeNeighbors(m Matrix, k int) map[string][]Scored {
	type cell struct {
		idx int32
		w   float64
	}
	itemIdx := map[string]int32{}
	var names []string
	for _, u := range sortedKeys(m) {
		for it := range m[u] {
			if _, ok := itemIdx[it]; !ok {
				itemIdx[it] = 0
				names = append(names, it)
			}
		}
	}
	slices.Sort(names)
	for i, it := range names {
		itemIdx[it] = int32(i) //nolint:gosec // G115: hay muchos menos de 2³¹ objetos
	}
	users := sortedKeys(m)
	userItems := make([][]cell, len(users))
	itemUsers := make([][]cell, len(names)) // cell.idx es la persona
	norms := make([]float64, len(names))
	for u, user := range users {
		row := m[user]
		cells := make([]cell, 0, len(row))
		for _, it := range sortedKeys(row) {
			i := itemIdx[it]
			w := row[it]
			cells = append(cells, cell{i, w})
			itemUsers[i] = append(itemUsers[i], cell{int32(u), w}) //nolint:gosec // G115: hay muchas menos de 2³¹ personas
			norms[i] += w * w
		}
		userItems[u] = cells
	}

	out := make(map[string][]Scored, len(names))
	acc := make([]float64, len(names))
	touched := make([]int32, 0, 256)
	best := newCollector(k)
	for i := range names {
		for _, uc := range itemUsers[i] {
			for _, jc := range userItems[uc.idx] {
				if int(jc.idx) == i {
					continue
				}
				if acc[jc.idx] == 0 {
					touched = append(touched, jc.idx)
				}
				acc[jc.idx] += uc.w * jc.w
			}
		}
		best.reset()
		for _, j := range touched {
			best.add(names[j], acc[j]/math.Sqrt(norms[i]*norms[j]))
			acc[j] = 0
		}
		touched = touched[:0]
		if len(best.best) > 0 {
			out[names[i]] = best.result()
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
