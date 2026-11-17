package model

import (
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain/synth"
)

// built es un modelo calculado con datos de entrenamiento.
type built struct {
	neighbors map[string][]domain.Neighbor
	popular   map[string][]string
}

// build calcula el modelo completo, como lo hará el recálculo (RF-02 a RF-04).
func build(train []domain.Event, items []domain.ItemFeatures) built {
	feats := make(map[string]domain.ItemFeatures, len(items))
	for _, it := range items {
		feats[it.ID] = it
	}
	cf := CollaborativeNeighbors(BuildMatrix(train), domain.Neighbors)
	content := ContentNeighbors(items, domain.Neighbors)
	return built{
		neighbors: MergeNeighbors(cf, content, feats, domain.Neighbors),
		popular:   Popular(train, items, domain.PopularSize),
	}
}

// recommender usa el modelo híbrido y completa con populares, como hará el caso
// de uso. Con hybridOnly en false es la línea base de popularidad.
func recommender(b built, hybridOnly bool) Recommender {
	return func(_ string, history []domain.Event) []string {
		seen := map[string]bool{}
		for _, e := range history {
			seen[e.ItemID] = true
		}
		var out []string
		if hybridOnly {
			for _, c := range Rank(history, b.neighbors, nil) {
				out = append(out, c.ItemID)
				seen[c.ItemID] = true
			}
		}
		for _, id := range b.popular[domain.PopularAll] {
			if !seen[id] {
				out = append(out, id)
			}
		}
		return out
	}
}

// TestEvaluationIsReproducibleAndBeatsPopularity es una prueba de control: con
// los datos sintéticos por defecto, la evaluación da lo mismo dos veces y el
// híbrido supera a la popularidad en Hit@10 (CA-04). El informe con todos los
// modelos y los números finales es docs/ml/evaluacion.md (T008 y T010).
func TestEvaluationIsReproducibleAndBeatsPopularity(t *testing.T) {
	cfg := synth.Default()
	items := synth.Items(cfg)
	split := LeaveLastOut(synth.Interactions(items, cfg))
	run := func() (Metrics, Metrics) {
		b := build(split.Train, items)
		return Evaluate(split, recommender(b, true), 10), Evaluate(split, recommender(b, false), 10)
	}
	hybrid, pop := run()
	hybrid2, pop2 := run()
	if !reflect.DeepEqual(hybrid, hybrid2) || !reflect.DeepEqual(pop, pop2) {
		t.Fatal("la evaluación no es reproducible")
	}
	t.Logf("personas=%d híbrido Hit@10=%.3f MRR=%.3f | popularidad Hit@10=%.3f MRR=%.3f",
		hybrid.Users, hybrid.HitAt, hybrid.MRR, pop.HitAt, pop.MRR)
	if hybrid.Users < 250 {
		t.Errorf("personas evaluadas = %d", hybrid.Users)
	}
	if hybrid.HitAt <= pop.HitAt {
		t.Errorf("el híbrido (%.3f) no supera a la popularidad (%.3f) en Hit@10", hybrid.HitAt, pop.HitAt)
	}
}

// BenchmarkRecompute mide el recálculo completo con 5 000 objetos y 50 000
// interacciones (RF-06: < 5 s y < 100 MB; plan P-11). Reporta la memoria
// asignada por recálculo, que es una cota superior del pico.
func BenchmarkRecompute(b *testing.B) {
	cfg := synth.Config{Users: 2500, Items: 5000, Categories: 12, Municipalities: 20, EventsPerUser: 20, Seed: 7,
		Start: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	items := synth.Items(cfg)
	events := synth.Interactions(items, cfg)
	if len(events) != 50_000 {
		b.Fatalf("eventos = %d", len(events))
	}
	b.ReportAllocs()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	b.ResetTimer()
	for b.Loop() {
		_ = build(events, items)
	}
	b.StopTimer()
	runtime.ReadMemStats(&after)
	b.ReportMetric(float64(after.TotalAlloc-before.TotalAlloc)/float64(b.N)/(1<<20), "MB/recálculo")
}
