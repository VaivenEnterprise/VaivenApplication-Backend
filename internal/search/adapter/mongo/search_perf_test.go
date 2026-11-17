//go:build integration && perf

package mongo

// Prueba de rendimiento (spec 006, T008; RF-09): p95 < 500 ms con 5,000
// objetos en los 3 modos. Se ejecuta aparte:
//
//	go test -tags "integration perf" -run TestPerf -v ./internal/search/adapter/mongo/

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
)

const (
	perfItems   = 5000
	perfOwners  = 200
	perfRuns    = 60
	perfP95Goal = 500 * time.Millisecond
)

var perfWords = []string{"taladro", "escalera", "bicicleta", "patines", "sierra", "martillo", "carpa", "podadora", "lijadora", "compresor"}

func TestPerfSearch(t *testing.T) {
	s := newSeed(t)
	ctx := context.Background()
	rnd := rand.New(rand.NewPCG(6, 6))
	owners := make([]string, perfOwners)
	for i := range owners {
		owners[i] = s.user(fmt.Sprintf("dueño%d", i), "active", nil)
	}
	docs := make([]any, 0, perfItems)
	cats := []string{"herramientas_manuales", "herramientas_electricas", "construccion_jardineria", "movilidad", "deportes", "hogar", "otros"}
	for i := range perfItems {
		w := perfWords[rnd.IntN(len(perfWords))]
		docs = append(docs, bson.M{
			"ownerId": owners[rnd.IntN(perfOwners)], "title": fmt.Sprintf("%s modelo %d", w, i),
			"description": "Descripción de prueba con " + perfWords[rnd.IntN(len(perfWords))],
			"category":    cats[rnd.IntN(len(cats))], "condition": "good", "modality": []string{"loan", "rent"}[rnd.IntN(2)],
			"pricePerDayCents": int64(1000 + rnd.IntN(5000)),
			"zone":             bson.M{"municipality": "Coyoacán"},
			"location": bson.M{"type": "Point", "coordinates": bson.A{
				-99.16 + (rnd.Float64()-0.5)*0.6, 19.35 + (rnd.Float64()-0.5)*0.6,
			}},
			"photos":    bson.A{bson.M{"publicId": "p", "format": "jpg", "delivery": "public"}},
			"status":    "active",
			"createdAt": baseTime.Add(time.Duration(i) * time.Second),
		})
	}
	if _, err := s.db.Collection(ItemsCollection).InsertMany(ctx, docs); err != nil {
		t.Fatal(err)
	}
	period := &domain.Period{From: "2026-11-10", To: "2026-11-12"}
	for i := range 500 { // reservas que el $lookup debe revisar
		_, _ = s.db.Collection(BookingsCollection).InsertOne(ctx, bson.M{
			"itemId": fmt.Sprintf("%024x", i), "status": "accepted", "startDate": "2026-11-11", "endDate": "2026-11-11",
		})
	}

	modes := map[string]domain.Query{
		"reciente": {ViewerID: owners[0], Limit: 20, Period: period},
		"texto":    {ViewerID: owners[0], Limit: 20, Text: "taladro"},
		"cercanía": {ViewerID: owners[0], Limit: 20, Near: &domain.Point{Lat: 19.35, Lng: -99.16}, RadiusKm: 25, Text: "bicicleta"},
	}
	for name, q := range modes {
		var times []time.Duration
		for range perfRuns {
			start := time.Now()
			p, err := s.repo.Search(ctx, q)
			times = append(times, time.Since(start))
			if err != nil || len(p.Cards) == 0 {
				t.Fatalf("%s: %d resultados, %v", name, len(p.Cards), err)
			}
		}
		slices.Sort(times)
		p50, p95 := times[len(times)/2], times[len(times)*95/100]
		t.Logf("%-9s p50=%v p95=%v (n=%d, %d objetos)", name, p50.Round(time.Millisecond), p95.Round(time.Millisecond), perfRuns, perfItems)
		if p95 > perfP95Goal {
			t.Errorf("%s: p95 %v > %v (RF-09)", name, p95, perfP95Goal)
		}
	}
}
