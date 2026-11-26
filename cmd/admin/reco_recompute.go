package main

import (
	"context"
	"fmt"
	"io"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

// runRecoRecompute es `recompute-recommendations` (spec 015, RF-06): recalcula
// el modelo ahora, con el mismo candado que el recálculo perezoso. Solo
// imprime conteos, nunca identificadores (RF-09). Se permite en todos los
// entornos: no toca datos de personas, solo el modelo calculado.
func runRecoRecompute(stdout, stderr io.Writer, load func() (config.Config, error), open recoFactory) int {
	cfg, err := load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
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
	res, err := d.recompute.Recompute(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if !res.Built {
		_, _ = fmt.Fprintln(stdout, "otro proceso está recalculando el modelo; intenta en unos minutos")
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "modelo recalculado: %d interacciones, %d objetos activos, %d cuentas dadas de baja purgadas\n",
		res.Interactions, res.Items, res.Purged)
	return 0
}
