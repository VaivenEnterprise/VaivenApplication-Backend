package model

import (
	"cmp"
	"slices"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// Popular arma las listas de populares (RF-04) sobre los objetos activos de
// items: una general (domain.PopularAll), una por municipio y una por
// categoría, de hasta size objetos cada una.
//
// La popularidad de un objeto es la suma de los pesos de sus interacciones. Los
// objetos sin interacciones también entran, después, del más nuevo al más
// viejo (el ObjectID crece con el tiempo): así, si hay objetos visibles, el
// arranque en frío nunca queda vacío (CA-02).
func Popular(events []domain.Event, items []domain.ItemFeatures, size int) map[string][]string {
	score := map[string]float64{}
	for _, e := range events {
		score[e.ItemID] += e.Weight
	}
	sorted := slices.Clone(items)
	slices.SortFunc(sorted, func(a, b domain.ItemFeatures) int {
		if c := cmp.Compare(score[b.ID], score[a.ID]); c != 0 {
			return c
		}
		return cmp.Compare(b.ID, a.ID)
	})
	out := map[string][]string{}
	add := func(key, id string) {
		if len(out[key]) < size {
			out[key] = append(out[key], id)
		}
	}
	for _, it := range sorted {
		add(domain.PopularAll, it.ID)
		if it.Municipality != "" {
			add(domain.PopularMunicipality(it.Municipality), it.ID)
		}
		if it.Category != "" {
			add(domain.PopularCategory(it.Category), it.ID)
		}
	}
	return out
}
