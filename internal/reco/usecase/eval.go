package usecase

import (
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain/model"
)

// EvalReport compara los cuatro modelos con las mismas personas (RF-07).
type EvalReport struct {
	K          int
	Users      int
	Hybrid     model.Metrics
	CF         model.Metrics // solo colaborativo
	Content    model.Metrics // solo contenido
	Popularity model.Metrics // línea base
}

// Evaluate hace la evaluación leave-last-out de los cuatro modelos sobre
// events e items (RF-07). Todos completan su lista con populares, así que la
// comparación mide lo que cada modelo agrega sobre la popularidad. Es pura y
// determinista: mismos datos, mismo informe.
func Evaluate(events []domain.Event, items []domain.ItemFeatures, k int) EvalReport {
	split := model.LeaveLastOut(events)
	feats := make(map[string]domain.ItemFeatures, len(items))
	for _, it := range items {
		feats[it.ID] = it
	}
	cf := model.CollaborativeNeighbors(model.BuildMatrix(split.Train), domain.Neighbors)
	content := model.ContentNeighbors(items, domain.Neighbors)
	hybrid := model.MergeNeighbors(cf, content, feats, domain.Neighbors)
	popular := model.Popular(split.Train, items, domain.PopularSize)[domain.PopularAll]

	r := EvalReport{
		K:          k,
		Hybrid:     model.Evaluate(split, ranked(hybrid, popular), k),
		CF:         model.Evaluate(split, ranked(asNeighbors(cf, true), popular), k),
		Content:    model.Evaluate(split, ranked(asNeighbors(content, false), popular), k),
		Popularity: model.Evaluate(split, ranked(nil, popular), k),
	}
	r.Users = r.Hybrid.Users
	return r
}

// ranked recomienda con neighbors (si hay) y completa con popular, sin
// repetir ni incluir objetos del historial.
func ranked(neighbors map[string][]domain.Neighbor, popular []string) model.Recommender {
	return func(_ string, history []domain.Event) []string {
		seen := map[string]bool{}
		for _, e := range history {
			seen[e.ItemID] = true
		}
		var out []string
		for _, c := range model.Rank(history, neighbors, nil) {
			out = append(out, c.ItemID)
			seen[c.ItemID] = true
		}
		for _, id := range popular {
			if !seen[id] {
				out = append(out, id)
			}
		}
		return out
	}
}

// asNeighbors convierte listas de un solo modelo: cf en el campo CF y
// contenido en el campo Content. Como Hybrid escala cada campo por una
// constante, el orden es el del modelo solo.
func asNeighbors(lists map[string][]model.Scored, isCF bool) map[string][]domain.Neighbor {
	out := make(map[string][]domain.Neighbor, len(lists))
	for i, list := range lists {
		ns := make([]domain.Neighbor, len(list))
		for j, s := range list {
			if isCF {
				ns[j] = domain.Neighbor{ItemID: s.ItemID, CF: s.Score}
			} else {
				ns[j] = domain.Neighbor{ItemID: s.ItemID, Content: s.Score}
			}
		}
		out[i] = ns
	}
	return out
}
