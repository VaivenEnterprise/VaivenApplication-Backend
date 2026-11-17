// Package synth genera datos sintéticos para evaluar y demostrar el
// recomendador (spec 015, RF-07 y RF-08; plan P-07 y P-08). Con la misma
// configuración y la misma semilla produce exactamente los mismos datos.
//
// Los datos tienen patrones a propósito, para que la evaluación sea
// significativa: cada persona prefiere una o dos categorías y un municipio, y
// dentro de cada categoría unos objetos son mucho más populares que otros. Se
// declaran como sintéticos en el informe. Las personas son identificadores
// demo-NNN, no cuentas, y los objetos de Items no existen en ninguna base.
package synth

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// Config describe el conjunto sintético.
type Config struct {
	Users          int
	Items          int // solo para Items
	Categories     int // solo para Items
	Municipalities int // solo para Items
	EventsPerUser  int
	Seed           uint64
	Start          time.Time // fecha de la primera interacción
}

// Default es la configuración de la evaluación (RF-07).
func Default() Config {
	return Config{
		Users: 300, Items: 600, Categories: 8, Municipalities: 6, EventsPerUser: 12, Seed: 15,
		Start: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
	}
}

// Probabilidades del comportamiento sintético.
const (
	pFavorite    = 0.75 // interactuar con una categoría preferida
	pSecondTaste = 0.4  // tener una segunda categoría preferida
	pHome        = 0.7  // dentro de la categoría, preferir el municipio propio
	pRequest     = 0.15 // la interacción es una solicitud y no una vista
)

// newRand es un generador determinista: con la misma semilla da los mismos
// datos (RF-07 exige una evaluación reproducible). No se usa para nada de
// seguridad, por eso no es crypto/rand.
func newRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) //nolint:gosec // G404: datos sintéticos reproducibles, no secretos
}

// Items genera cfg.Items objetos sintéticos con ids de 24 hexadecimales.
func Items(cfg Config) []domain.ItemFeatures {
	r := newRand(cfg.Seed)
	out := make([]domain.ItemFeatures, cfg.Items)
	for i := range out {
		modality, price := "loan", int64(0)
		if r.Float64() < 0.6 {
			modality, price = "rent", int64(1+r.IntN(80))*1_000
		}
		out[i] = domain.ItemFeatures{
			ID:               fmt.Sprintf("%024x", i+1),
			OwnerID:          fmt.Sprintf("owner-%03d", r.IntN(max(1, cfg.Items/4))),
			Title:            fmt.Sprintf("Objeto sintético %d", i+1),
			Category:         fmt.Sprintf("cat-%02d", r.IntN(max(1, cfg.Categories))),
			Modality:         modality,
			PricePerDayCents: price,
			Municipality:     fmt.Sprintf("Municipio %02d", r.IntN(max(1, cfg.Municipalities))),
		}
	}
	return out
}

// Interactions genera cfg.EventsPerUser interacciones por persona sobre items
// (sintéticos o reales). Las personas son demo-001, demo-002, …; las fechas
// crecen de un minuto en un minuto desde cfg.Start, así que la última
// interacción de cada persona está bien definida (leave-last-out).
func Interactions(items []domain.ItemFeatures, cfg Config) []domain.Event {
	if len(items) == 0 || cfg.Users <= 0 || cfg.EventsPerUser <= 0 {
		return nil
	}
	r := newRand(cfg.Seed + 1)
	byCat := map[string][]domain.ItemFeatures{}
	byCatMun := map[string][]domain.ItemFeatures{}
	for _, it := range items {
		byCat[it.Category] = append(byCat[it.Category], it)
		byCatMun[it.Category+"|"+it.Municipality] = append(byCatMun[it.Category+"|"+it.Municipality], it)
	}
	cats := sortedKeys(byCat)
	muns := municipalities(items)

	out := make([]domain.Event, 0, cfg.Users*cfg.EventsPerUser)
	minute := 0
	for u := range cfg.Users {
		user := fmt.Sprintf("demo-%03d", u+1)
		tastes := []string{cats[r.IntN(len(cats))]}
		if r.Float64() < pSecondTaste {
			tastes = append(tastes, cats[r.IntN(len(cats))])
		}
		home := muns[r.IntN(len(muns))]
		for range cfg.EventsPerUser {
			var pool []domain.ItemFeatures
			if r.Float64() < pFavorite {
				cat := tastes[r.IntN(len(tastes))]
				pool = byCat[cat]
				if local := byCatMun[cat+"|"+home]; len(local) > 0 && r.Float64() < pHome {
					pool = local
				}
			} else {
				pool = items
			}
			it := pool[skewed(r, len(pool))]
			typ := domain.EventView
			if r.Float64() < pRequest {
				typ = domain.EventRequest
			}
			out = append(out, domain.Event{
				UserID: user, ItemID: it.ID, Type: typ, Weight: typ.Weight(),
				At: cfg.Start.Add(time.Duration(minute) * time.Minute).UTC(),
			})
			minute++
		}
	}
	return out
}

// skewed elige un índice en [0, n) con sesgo hacia los primeros: unos objetos
// son mucho más populares que otros, como en la realidad.
func skewed(r *rand.Rand, n int) int {
	x := r.Float64()
	return min(n-1, int(float64(n)*x*x))
}

func municipalities(items []domain.ItemFeatures) []string {
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.Municipality] = true
	}
	return sortedKeys(seen)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
