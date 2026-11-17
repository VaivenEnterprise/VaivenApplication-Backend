// Package search implementa assistant.Finder sobre el caso de uso de búsqueda
// de la spec 006 (spec 014, RF-11; plan P-07 y P-10). Hereda sus reglas de
// visibilidad: solo objetos activos de otras personas con cuenta activa y
// ubicación aproximada.
//
// Estrategia por necesidad:
//   - Con ubicación, la búsqueda por cercanía exige que todas las palabras
//     aparezcan; por eso se busca término por término y se juntan resultados
//     únicos hasta MaxMatches.
//   - Sin ubicación, el modo de texto ya combina las palabras con «o»: una sola
//     búsqueda con todos los términos.
//
// Como máximo Parallel búsquedas a la vez (todas las necesidades comparten el
// límite) y cada búsqueda tiene su propio tiempo de espera.
package search

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/usecase"
	searchdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

// Límites (RF-11, RNF-01).
const (
	MaxMatches    = 5
	Parallel      = 4
	searchTimeout = 3 * time.Second
)

// Searcher es la búsqueda de la spec 006 (searchusecase.Service la cumple).
type Searcher interface {
	Search(ctx context.Context, viewerID string, raw searchdomain.RawQuery) (searchusecase.Result, error)
}

// Finder busca quién tiene cada necesidad.
type Finder struct {
	search   Searcher
	radiusKm int
	timeout  time.Duration
}

// New crea el buscador. radiusKm es ASSISTANT_RADIUS_KM.
func New(s Searcher, radiusKm int) *Finder {
	return &Finder{search: s, radiusKm: radiusKm, timeout: searchTimeout}
}

// Find devuelve las coincidencias de cada necesidad en el mismo orden. Una
// búsqueda que falla deja esa necesidad sin coincidencias; solo si fallan
// todas se devuelve el error.
func (f *Finder) Find(ctx context.Context, req usecase.FindRequest) ([]usecase.NeedResult, error) {
	out := make([]usecase.NeedResult, len(req.Needs))
	sem := make(chan struct{}, Parallel)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		failures int
		firstErr error
	)
	for i, need := range req.Needs {
		out[i] = usecase.NeedResult{Need: need, Matches: []searchusecase.CardView{}}
		wg.Add(1)
		go func() {
			defer wg.Done()
			matches, err := f.findOne(ctx, sem, req, need.Category, need.SearchTerms)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			out[i].Matches = matches
		}()
	}
	wg.Wait()
	if len(req.Needs) > 0 && failures == len(req.Needs) {
		return nil, firstErr
	}
	return out, nil
}

func (f *Finder) findOne(ctx context.Context, sem chan struct{}, req usecase.FindRequest, category string, terms []string) ([]searchusecase.CardView, error) {
	if len(terms) == 0 {
		return []searchusecase.CardView{}, nil
	}
	if req.Near == nil {
		return f.query(ctx, sem, req, category, strings.Join(terms, " "))
	}
	seen := map[string]bool{}
	out := []searchusecase.CardView{}
	var lastErr error
	ok := false
	for _, term := range terms {
		cards, err := f.query(ctx, sem, req, category, term)
		if err != nil {
			lastErr = err
			continue
		}
		ok = true
		for _, c := range cards {
			if len(out) == MaxMatches {
				break
			}
			if !seen[c.Card.ID] {
				seen[c.Card.ID] = true
				out = append(out, c)
			}
		}
		if len(out) == MaxMatches {
			break
		}
	}
	if !ok {
		return nil, lastErr
	}
	return out, nil
}

// query hace una búsqueda respetando el límite de concurrencia.
func (f *Finder) query(ctx context.Context, sem chan struct{}, req usecase.FindRequest, category, text string) ([]searchusecase.CardView, error) {
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-sem }()
	raw := searchdomain.RawQuery{Q: text, Category: category, Limit: strconv.Itoa(MaxMatches)}
	switch {
	case req.Near != nil:
		raw.Lat = strconv.FormatFloat(req.Near.Lat, 'f', -1, 64)
		raw.Lng = strconv.FormatFloat(req.Near.Lng, 'f', -1, 64)
		raw.RadiusKm = strconv.Itoa(f.radiusKm)
	case req.Municipality != "":
		raw.Municipality = req.Municipality
	}
	cctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	res, err := f.search.Search(cctx, req.ViewerID, raw)
	if err != nil {
		return nil, err
	}
	if len(res.Cards) > MaxMatches {
		res.Cards = res.Cards[:MaxMatches]
	}
	if res.Cards == nil {
		// Sin resultados el contrato pide [] y no null (CA-08).
		return []searchusecase.CardView{}, nil
	}
	return res.Cards, nil
}
