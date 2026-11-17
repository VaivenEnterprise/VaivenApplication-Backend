package model

import (
	"cmp"
	"slices"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// Hybrid mezcla las dos similitudes (RF-04): α·cf + (1−α)·contenido.
func Hybrid(cf, content float64) float64 {
	return domain.Alpha*cf + (1-domain.Alpha)*content
}

// MergeNeighbors combina los vecinos colaborativos y de contenido de cada
// objeto en una sola lista de hasta k, ordenada por el puntaje híbrido. Solo
// quedan como vecinos los objetos de feats (los activos); el contenido se
// recalcula exacto para cada par, y el colaborativo vale 0 si el par no está
// entre los vecinos colaborativos.
func MergeNeighbors(cf, content map[string][]Scored, feats map[string]domain.ItemFeatures, k int) map[string][]domain.Neighbor {
	keys := map[string]bool{}
	for i := range cf {
		keys[i] = true
	}
	for i := range content {
		keys[i] = true
	}
	ft := make(map[string]features, len(feats))
	for id, f := range feats {
		ft[id] = featuresOf(f)
	}
	out := make(map[string][]domain.Neighbor, len(keys))
	for _, i := range sortedKeys(keys) {
		cfScore := map[string]float64{}
		for _, s := range cf[i] {
			cfScore[s.ItemID] = s.Score
		}
		seen := map[string]bool{}
		var ns []domain.Neighbor
		fi, keyActive := ft[i]
		add := func(j string) {
			fj, active := ft[j]
			if seen[j] || !active || j == i {
				return
			}
			seen[j] = true
			n := domain.Neighbor{ItemID: j, CF: cfScore[j]}
			if keyActive {
				n.Content = similarity(fi, fj)
			}
			ns = append(ns, n)
		}
		for _, s := range cf[i] {
			add(s.ItemID)
		}
		for _, s := range content[i] {
			add(s.ItemID)
		}
		slices.SortFunc(ns, func(a, b domain.Neighbor) int {
			if c := cmp.Compare(Hybrid(b.CF, b.Content), Hybrid(a.CF, a.Content)); c != 0 {
				return c
			}
			return cmp.Compare(a.ItemID, b.ItemID)
		})
		if len(ns) > k {
			ns = ns[:k]
		}
		if len(ns) > 0 {
			out[i] = ns
		}
	}
	return out
}

// Candidate es un objeto recomendado con su puntaje y su motivo.
type Candidate struct {
	ItemID string
	Score  float64
	Reason domain.Reason // en similar_to, ItemID es el objeto que más contribuyó (sin título)
}

// Rank puntúa los vecinos de los objetos del historial (RF-04):
//
//	score(c) = Σ_h w(h) · [α·cf(h,c) + (1−α)·contenido(h,c)]
//
// w(h) es el peso máximo de las interacciones con h. No recomienda objetos del
// historial ni de exclude. El motivo es el objeto h que más contribuyó.
func Rank(history []domain.Event, neighbors map[string][]domain.Neighbor, exclude map[string]bool) []Candidate {
	weight := map[string]float64{}
	for _, e := range history {
		weight[e.ItemID] = max(weight[e.ItemID], e.Weight)
	}
	type acc struct {
		score, best float64
		because     string
	}
	scores := map[string]*acc{}
	for _, h := range sortedKeys(weight) {
		for _, n := range neighbors[h] {
			if exclude[n.ItemID] || weight[n.ItemID] > 0 {
				continue
			}
			c := weight[h] * Hybrid(n.CF, n.Content)
			if c <= 0 {
				continue
			}
			a := scores[n.ItemID]
			if a == nil {
				a = &acc{}
				scores[n.ItemID] = a
			}
			a.score += c
			if c > a.best { // h va en orden: a igual aporte gana el menor id
				a.best, a.because = c, h
			}
		}
	}
	out := make([]Candidate, 0, len(scores))
	for id, a := range scores {
		out = append(out, Candidate{ItemID: id, Score: a.score, Reason: domain.Reason{Code: domain.ReasonSimilarTo, ItemID: a.because}})
	}
	sortCandidates(out)
	return out
}

func sortCandidates(c []Candidate) {
	slices.SortFunc(c, func(a, b Candidate) int {
		if x := cmp.Compare(b.Score, a.Score); x != 0 {
			return x
		}
		return cmp.Compare(a.ItemID, b.ItemID)
	})
}

// ColdStart arma la lista del arranque en frío (RF-04), en este orden y sin
// repetir: populares del municipio (popular_nearby), de cada categoría vista
// (popular_in_category) y generales (popular). Respeta exclude.
func ColdStart(popular map[string][]string, municipality string, categories []string, exclude map[string]bool) []Candidate {
	seen := map[string]bool{}
	var out []Candidate
	add := func(key string, code domain.ReasonCode) {
		for _, id := range popular[key] {
			if seen[id] || exclude[id] {
				continue
			}
			seen[id] = true
			out = append(out, Candidate{ItemID: id, Reason: domain.Reason{Code: code}})
		}
	}
	if municipality != "" {
		add(domain.PopularMunicipality(municipality), domain.ReasonPopularNearby)
	}
	for _, c := range categories {
		add(domain.PopularCategory(c), domain.ReasonPopularInCategory)
	}
	add(domain.PopularAll, domain.ReasonPopular)
	return out
}

// Diversify recorre cands en orden y se queda con hasta limit, con un máximo de
// maxPer por categoría (RF-05). category da la categoría de cada objeto; un
// objeto sin categoría conocida no se recomienda (ya no está activo).
func Diversify(cands []Candidate, category map[string]string, maxPer, limit int) []Candidate {
	perCat := map[string]int{}
	out := make([]Candidate, 0, min(limit, len(cands)))
	for _, c := range cands {
		if len(out) == limit {
			break
		}
		cat, ok := category[c.ItemID]
		if !ok || perCat[cat] == maxPer {
			continue
		}
		perCat[cat]++
		out = append(out, c)
	}
	return out
}
