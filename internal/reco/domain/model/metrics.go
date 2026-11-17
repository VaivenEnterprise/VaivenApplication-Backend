package model

import (
	"cmp"
	"slices"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// Split es una partición leave-last-out (RF-07): de cada persona con al menos
// dos objetos distintos se oculta su última interacción (Test) y el resto queda
// para entrenar (Train).
type Split struct {
	Train []domain.Event
	Test  map[string]string // persona → objeto oculto
}

// LeaveLastOut parte los eventos. La última interacción es la de fecha mayor
// (a igual fecha, el mayor ItemID). Se quitan de Train todos los eventos de esa
// persona con el objeto oculto, para no filtrar la respuesta.
func LeaveLastOut(events []domain.Event) Split {
	type last struct {
		item string
		at   int64
	}
	lasts := map[string]last{}
	distinct := map[string]map[string]bool{}
	for _, e := range events {
		if distinct[e.UserID] == nil {
			distinct[e.UserID] = map[string]bool{}
		}
		distinct[e.UserID][e.ItemID] = true
		t := e.At.UnixNano()
		if l, ok := lasts[e.UserID]; !ok || t > l.at || (t == l.at && e.ItemID > l.item) {
			lasts[e.UserID] = last{e.ItemID, t}
		}
	}
	s := Split{Test: map[string]string{}}
	for u, items := range distinct {
		if len(items) >= 2 {
			s.Test[u] = lasts[u].item
		}
	}
	for _, e := range events {
		if hidden, ok := s.Test[e.UserID]; ok && hidden == e.ItemID {
			continue
		}
		s.Train = append(s.Train, e)
	}
	return s
}

// Metrics son las métricas de una evaluación top-K.
type Metrics struct {
	Users  int     // personas evaluadas
	HitAt  float64 // fracción con el objeto oculto en el top-K
	Recall float64 // objetos ocultos recuperados / objetos ocultos
	MRR    float64 // media de 1/rango del objeto oculto (0 si no está en el top-K)
}

// Recommender devuelve la lista ordenada de objetos para una persona a partir
// de su historial de entrenamiento.
type Recommender func(user string, history []domain.Event) []string

// Evaluate calcula Hit@K, Recall@K y MRR@K de rec sobre s (RF-07). Con
// leave-last-out hay un solo objeto relevante por persona, así que Recall@K
// coincide con Hit@K; se reportan los dos porque la spec los pide.
func Evaluate(s Split, rec Recommender, k int) Metrics {
	history := map[string][]domain.Event{}
	for _, e := range s.Train {
		history[e.UserID] = append(history[e.UserID], e)
	}
	var m Metrics
	hits, found, relevant, rr := 0, 0, 0, 0.0
	for _, u := range sortedKeys(s.Test) {
		h := history[u]
		slices.SortFunc(h, func(a, b domain.Event) int {
			if c := b.At.Compare(a.At); c != 0 {
				return c
			}
			return cmp.Compare(a.ItemID, b.ItemID)
		})
		list := rec(u, h)
		if len(list) > k {
			list = list[:k]
		}
		m.Users++
		relevant++
		if i := slices.Index(list, s.Test[u]); i >= 0 {
			hits++
			found++
			rr += 1 / float64(i+1)
		}
	}
	if m.Users > 0 {
		m.HitAt = float64(hits) / float64(m.Users)
		m.Recall = float64(found) / float64(relevant)
		m.MRR = rr / float64(m.Users)
	}
	return m
}
