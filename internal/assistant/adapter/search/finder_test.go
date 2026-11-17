package search

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/usecase"
	searchdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

// fakeSearch devuelve, para cada texto, las tarjetas de results[texto].
type fakeSearch struct {
	mu       sync.Mutex
	results  map[string][]string // texto → ids
	fail     map[string]bool
	queries  []searchdomain.RawQuery
	viewers  []string
	inFlight atomic.Int32
	maxSeen  atomic.Int32
	delay    time.Duration
}

func (f *fakeSearch) Search(ctx context.Context, viewerID string, raw searchdomain.RawQuery) (searchusecase.Result, error) {
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		m := f.maxSeen.Load()
		if n <= m || f.maxSeen.CompareAndSwap(m, n) {
			break
		}
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return searchusecase.Result{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, raw)
	f.viewers = append(f.viewers, viewerID)
	if f.fail[raw.Q] {
		return searchusecase.Result{}, errors.New("búsqueda caída")
	}
	var cards []searchusecase.CardView
	for _, id := range f.results[raw.Q] {
		cards = append(cards, searchusecase.CardView{Card: searchdomain.Card{ID: id, Category: raw.Category}})
	}
	return searchusecase.Result{Cards: cards}, nil
}

func need(name, cat string, terms ...string) aidomain.Need {
	return aidomain.Need{Name: name, Category: cat, SearchTerms: terms}
}

func ids(cards []searchusecase.CardView) []string {
	out := []string{}
	for _, c := range cards {
		out = append(out, c.Card.ID)
	}
	return out
}

func TestNearSearchesTermByTermAndMergesUnique(t *testing.T) {
	s := &fakeSearch{results: map[string][]string{
		"escalera": {"a", "b", "c"},
		"tijera":   {"b", "d", "e", "f", "g"},
	}}
	res, err := New(s, 10).Find(context.Background(), usecase.FindRequest{
		ViewerID: "u1", Near: &usecase.GeoPoint{Lat: 25.67, Lng: -100.31},
		Needs: []aidomain.Need{need("Escalera de tijera", "hogar", "escalera", "tijera", "aluminio")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(ids(res[0].Matches)); got != "[a b c d e]" {
		t.Errorf("coincidencias = %s, want únicas y como máximo 5", got)
	}
	if len(s.queries) != 2 {
		t.Errorf("con 5 resultados no debe buscar el tercer término: %d búsquedas", len(s.queries))
	}
	q := s.queries[0]
	if q.Lat != "25.67" || q.Lng != "-100.31" || q.RadiusKm != "10" || q.Category != "hogar" || q.Limit != "5" || q.Municipality != "" {
		t.Errorf("consulta = %+v", q)
	}
	if s.viewers[0] != "u1" {
		t.Error("la búsqueda debe saber quién busca para excluir sus objetos")
	}
}

func TestWithoutLocationOneTextSearchOrMunicipality(t *testing.T) {
	s := &fakeSearch{results: map[string][]string{"rodillo brocha": {"x"}}}
	res, err := New(s, 10).Find(context.Background(), usecase.FindRequest{
		ViewerID: "u1", Municipality: "Monterrey",
		Needs: []aidomain.Need{need("Rodillo", "hogar", "rodillo", "brocha")},
	})
	if err != nil || fmt.Sprint(ids(res[0].Matches)) != "[x]" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(s.queries) != 1 || s.queries[0].Municipality != "Monterrey" || s.queries[0].Lat != "" {
		t.Errorf("consultas = %+v", s.queries)
	}
}

func TestKeepsOrderAndEmptyMatchesAreNotNil(t *testing.T) {
	s := &fakeSearch{results: map[string][]string{"escalera": {"a"}}}
	res, err := New(s, 10).Find(context.Background(), usecase.FindRequest{
		Needs: []aidomain.Need{need("Lona", "hogar", "lona"), need("Escalera", "hogar", "escalera"), need("Nada", "otros")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Need.Name != "Lona" || res[1].Need.Name != "Escalera" || len(res[1].Matches) != 1 {
		t.Errorf("orden o coincidencias: %+v", res)
	}
	if res[0].Matches == nil || len(res[0].Matches) != 0 || res[2].Matches == nil {
		t.Error("sin coincidencias debe ser [] (CA-08), no nil")
	}
}

func TestConcurrencyIsBounded(t *testing.T) {
	s := &fakeSearch{results: map[string][]string{}, delay: 20 * time.Millisecond}
	var needs []aidomain.Need
	for i := 0; i < 8; i++ {
		needs = append(needs, need(fmt.Sprint("n", i), "otros", "a", "b", "c"))
	}
	if _, err := New(s, 10).Find(context.Background(), usecase.FindRequest{Near: &usecase.GeoPoint{}, Needs: needs}); err != nil {
		t.Fatal(err)
	}
	if m := s.maxSeen.Load(); m > Parallel || m < 2 {
		t.Errorf("búsquedas simultáneas = %d, want entre 2 y %d", m, Parallel)
	}
	if len(s.queries) != 24 {
		t.Errorf("búsquedas = %d, want 24 (8 necesidades × 3 términos sin resultados)", len(s.queries))
	}
}

func TestFailuresArePerNeed(t *testing.T) {
	s := &fakeSearch{results: map[string][]string{"rodillo": {"r"}}, fail: map[string]bool{"lona": true}}
	res, err := New(s, 10).Find(context.Background(), usecase.FindRequest{
		Needs: []aidomain.Need{need("Lona", "hogar", "lona"), need("Rodillo", "hogar", "rodillo")},
	})
	if err != nil || len(res[0].Matches) != 0 || len(res[1].Matches) != 1 {
		t.Errorf("una falla no tumba las demás: res=%+v err=%v", res, err)
	}
	// Con ubicación: un término caído no impide usar el siguiente.
	res, err = New(s, 10).Find(context.Background(), usecase.FindRequest{Near: &usecase.GeoPoint{},
		Needs: []aidomain.Need{need("Rodillo", "hogar", "lona", "rodillo")}})
	if err != nil || len(res[0].Matches) != 1 {
		t.Errorf("término caído: res=%+v err=%v", res, err)
	}
	// Si fallan todas, se devuelve el error.
	s.fail = map[string]bool{"lona": true, "rodillo": true}
	for _, near := range []*usecase.GeoPoint{nil, {}} {
		if _, err := New(s, 10).Find(context.Background(), usecase.FindRequest{Near: near,
			Needs: []aidomain.Need{need("Lona", "hogar", "lona"), need("Rodillo", "hogar", "rodillo")}}); err == nil {
			t.Error("si todas fallan debe haber error")
		}
	}
}

func TestCanceledContextAndTimeout(t *testing.T) {
	s := &fakeSearch{delay: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(s, 10).Find(ctx, usecase.FindRequest{Needs: []aidomain.Need{need("x", "otros", "x")}}); err == nil {
		t.Error("contexto cancelado debe fallar")
	}
	f := New(s, 10)
	f.timeout = 10 * time.Millisecond
	start := time.Now()
	if _, err := f.Find(context.Background(), usecase.FindRequest{Needs: []aidomain.Need{need("x", "otros", "x")}}); err == nil || time.Since(start) > 500*time.Millisecond {
		t.Errorf("tiempo de espera por búsqueda: err=%v en %v", err, time.Since(start))
	}
}

func TestNoNeedsAndCapOnResults(t *testing.T) {
	res, err := New(&fakeSearch{}, 10).Find(context.Background(), usecase.FindRequest{})
	if err != nil || len(res) != 0 {
		t.Errorf("sin necesidades: %+v %v", res, err)
	}
	s := &fakeSearch{results: map[string][]string{"a": {"1", "2", "3", "4", "5", "6", "7"}}}
	res, _ = New(s, 10).Find(context.Background(), usecase.FindRequest{Needs: []aidomain.Need{need("A", "otros", "a")}})
	if len(res[0].Matches) != MaxMatches {
		t.Errorf("coincidencias = %d, want %d", len(res[0].Matches), MaxMatches)
	}
}
