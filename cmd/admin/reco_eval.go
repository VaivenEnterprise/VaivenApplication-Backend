package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain/model"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain/synth"
	recousecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/usecase"
)

// runRecoEval es `eval-recommendations` (spec 015, RF-07): evaluación
// leave-last-out de los cuatro modelos. Por defecto usa los datos sintéticos
// con semilla fija, en memoria y sin base (reproducible: la misma salida en
// cada ejecución). Con --source=db usa las interacciones del entorno, nunca en
// producción. Escribe una tabla en Markdown para docs/ml/evaluacion.md.
func runRecoEval(args []string, stdout, stderr io.Writer, load func() (config.Config, error), open recoFactory) int {
	fs := flag.NewFlagSet("eval-recommendations", flag.ContinueOnError)
	fs.SetOutput(stderr)
	source := fs.String("source", "synthetic", "synthetic (datos sintéticos, por defecto) o db (interacciones del entorno)")
	k := fs.Int("k", 10, "tamaño de la lista evaluada")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 || *k < 1 || *k > domain.PopularSize || (*source != "synthetic" && *source != "db") {
		_, _ = fmt.Fprintln(stderr, "error: uso: admin eval-recommendations [--source synthetic|db] [--k N], con 1 ≤ N ≤ 50")
		return 2
	}
	if *source == "synthetic" {
		cfg := synth.Default()
		items := synth.Items(cfg)
		events := synth.Interactions(items, cfg)
		desc := fmt.Sprintf("sintética (semilla %d; %d personas, %d objetos, %d categorías, %d municipios, %d interacciones por persona)",
			cfg.Seed, cfg.Users, cfg.Items, cfg.Categories, cfg.Municipalities, cfg.EventsPerUser)
		printEval(stdout, desc, len(events), recousecase.Evaluate(events, items, *k))
		return 0
	}

	cfg, err := load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if cfg.IsProduction() {
		_, _ = fmt.Fprintln(stderr, "error: eval-recommendations --source db no se ejecuta con APP_ENV=production")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), recoTimeout)
	defer cancel()
	d, err := open(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer d.close()
	since := d.now().Add(-domain.Window)
	views, err := d.views.Since(ctx, since, domain.MaxInteractionsRead)
	if err == nil {
		var reqs []domain.Event
		reqs, err = d.requests.Since(ctx, since, domain.MaxInteractionsRead)
		views = slices.Concat(views, reqs)
	}
	var items []domain.ItemFeatures
	if err == nil {
		items, err = d.items.ActiveFeatures(ctx)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	printEval(stdout, "interacciones de "+cfg.AppEnv+" (últimos 90 días)", len(views), recousecase.Evaluate(views, items, *k))
	return 0
}

func printEval(w io.Writer, source string, events int, r recousecase.EvalReport) {
	_, _ = fmt.Fprintf(w, "Evaluación de recomendaciones (leave-last-out, K=%d)\n\n", r.K)
	_, _ = fmt.Fprintf(w, "- Fuente: %s\n- Interacciones: %d\n- Personas evaluadas: %d\n\n", source, events, r.Users)
	_, _ = fmt.Fprintf(w, "| Modelo | Hit@%d | Recall@%d | MRR@%d |\n|---|---|---|---|\n", r.K, r.K, r.K)
	for _, row := range []struct {
		name string
		m    model.Metrics
	}{{fmt.Sprintf("Híbrido (α = %.1f)", domain.Alpha), r.Hybrid}, {"Colaborativo", r.CF}, {"Contenido", r.Content}, {"Popularidad (línea base)", r.Popularity}} {
		_, _ = fmt.Fprintf(w, "| %s | %.3f | %.3f | %.3f |\n", row.name, row.m.HitAt, row.m.Recall, row.m.MRR)
	}
}
