package model

import (
	"math"
	"strings"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// Rasgos del modelo de contenido (RF-03): categoría, modalidad, banda de
// tarifa y municipio, en codificación one-hot (cada rasgo vale 1).
type features struct {
	category, modality, band, municipality string
}

func featuresOf(f domain.ItemFeatures) features {
	return features{
		category:     f.Category,
		modality:     f.Modality,
		band:         domain.PriceBand(f.Modality, f.PricePerDayCents),
		municipality: normMunicipality(f.Municipality),
	}
}

func normMunicipality(m string) string { return strings.ToLower(strings.Join(strings.Fields(m), " ")) }

func (f features) count() int {
	n := 1 // la banda siempre existe
	for _, s := range []string{f.category, f.modality, f.municipality} {
		if s != "" {
			n++
		}
	}
	return n
}

// ContentVector es el vector one-hot de un objeto. Documenta el modelo; el
// cálculo usa ContentSimilarity, que da lo mismo sin crear mapas.
func ContentVector(f domain.ItemFeatures) Vector {
	ft := featuresOf(f)
	v := Vector{"band:" + ft.band: 1}
	if ft.category != "" {
		v["cat:"+ft.category] = 1
	}
	if ft.modality != "" {
		v["mod:"+ft.modality] = 1
	}
	if ft.municipality != "" {
		v["mun:"+ft.municipality] = 1
	}
	return v
}

// ContentSimilarity es el coseno de contenido de a y b. Con vectores one-hot
// es rasgos_compartidos / √(rasgos_a · rasgos_b).
func ContentSimilarity(a, b domain.ItemFeatures) float64 {
	return similarity(featuresOf(a), featuresOf(b))
}

func similarity(a, b features) float64 {
	shared := 0
	if a.category != "" && a.category == b.category {
		shared++
	}
	if a.modality != "" && a.modality == b.modality {
		shared++
	}
	if a.band == b.band {
		shared++
	}
	if a.municipality != "" && a.municipality == b.municipality {
		shared++
	}
	if shared == 0 {
		return 0
	}
	return float64(shared) / math.Sqrt(float64(a.count()*b.count()))
}

// ContentNeighbors calcula los k vecinos de contenido de cada objeto. Para que
// el costo sea acotado, solo compara objetos que comparten categoría o
// municipio (RF-03); los demás comparten a lo sumo modalidad y banda.
func ContentNeighbors(items []domain.ItemFeatures, k int) map[string][]Scored {
	feats := make(map[string]features, len(items))
	byCategory := map[string][]string{}
	byMunicipality := map[string][]string{}
	for _, it := range items {
		f := featuresOf(it)
		feats[it.ID] = f
		if f.category != "" {
			byCategory[f.category] = append(byCategory[f.category], it.ID)
		}
		if f.municipality != "" {
			byMunicipality[f.municipality] = append(byMunicipality[f.municipality], it.ID)
		}
	}
	out := make(map[string][]Scored, len(items))
	best := newCollector(k)
	for _, it := range items {
		f := feats[it.ID]
		best.reset()
		for _, j := range byCategory[f.category] {
			if j != it.ID {
				best.add(j, similarity(f, feats[j]))
			}
		}
		for _, j := range byMunicipality[f.municipality] {
			// Los de la misma categoría ya se compararon arriba.
			if g := feats[j]; j != it.ID && (f.category == "" || g.category != f.category) {
				best.add(j, similarity(f, g))
			}
		}
		if len(best.best) > 0 {
			out[it.ID] = best.result()
		}
	}
	return out
}
