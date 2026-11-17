package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain/synth"
)

// maxDemoUsers acota seed-interactions.
const maxDemoUsers = 2000

// runRecoSeed es `seed-interactions` (spec 015, RF-08): genera interacciones
// sintéticas con patrones por categoría y municipio sobre los objetos activos
// del entorno, con personas demo-NNN (no son cuentas) y marcadas con demo:
// true. --purge las borra. Nunca se ejecuta en producción.
func runRecoSeed(args []string, stdout, stderr io.Writer, load func() (config.Config, error), open recoFactory) int {
	fs := flag.NewFlagSet("seed-interactions", flag.ContinueOnError)
	fs.SetOutput(stderr)
	purge := fs.Bool("purge", false, "borra las interacciones de demostración en lugar de crearlas")
	users := fs.Int("users", 300, "personas sintéticas")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 || *users < 1 || *users > maxDemoUsers {
		_, _ = fmt.Fprintf(stderr, "error: uso: admin seed-interactions [--purge] [--users N], con 1 ≤ N ≤ %d\n", maxDemoUsers)
		return 2
	}
	cfg, err := load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	// Guarda antes de conectar: nada se escribe en producción.
	if cfg.IsProduction() {
		_, _ = fmt.Fprintln(stderr, "error: seed-interactions no se ejecuta con APP_ENV=production (spec 015, RF-08)")
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

	if *purge {
		n, err := d.demo.PurgeDemo(ctx)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "interacciones de demostración borradas: %d\n", n)
		return 0
	}
	items, err := d.items.ActiveFeatures(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if len(items) == 0 {
		_, _ = fmt.Fprintln(stderr, "error: no hay objetos activos; corre antes `admin seed`")
		return 1
	}
	sc := synth.Default()
	sc.Users = *users
	// Las fechas terminan antes de ahora y caben de sobra en la ventana de 90 días.
	sc.Start = d.now().UTC().Add(-time.Duration(sc.Users*sc.EventsPerUser+60) * time.Minute)
	n, err := d.demo.InsertDemo(ctx, synth.Interactions(items, sc))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "interacciones de demostración creadas: %d (%d personas sintéticas sobre %d objetos activos); corre `admin recompute-recommendations` para usarlas\n",
		n, sc.Users, len(items))
	return 0
}
