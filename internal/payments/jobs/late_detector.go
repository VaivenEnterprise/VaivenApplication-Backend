// Package jobs ejecuta las tareas periódicas del módulo de pagos y retrasos
// (spec 009, T011; plan P-05). Sin estado propio (Art. XIV): cada pasada es
// idempotente, así que varias réplicas pueden correrla a la vez.
package jobs

import (
	"context"
	"log/slog"
	"time"
)

// Interval es cada cuánto se buscan devoluciones tardías (plan P-05).
const Interval = 5 * time.Minute

// Detector es el caso de uso que marca retrasos (usecase.Service).
type Detector interface {
	DetectLate(ctx context.Context) (int, error)
}

// RunLateDetector llama a DetectLate al arrancar y en cada tick hasta que ctx se
// cancela. tick nil usa un time.Ticker de Interval (se inyecta en pruebas). Un
// error se registra y la tarea sigue.
func RunLateDetector(ctx context.Context, d Detector, tick <-chan time.Time, log *slog.Logger) {
	if tick == nil {
		t := time.NewTicker(Interval)
		defer t.Stop()
		tick = t.C
	}
	run := func() {
		n, err := d.DetectLate(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			log.ErrorContext(ctx, "payments: error al detectar retrasos", "error", err)
		case n > 0:
			log.InfoContext(ctx, "payments: devoluciones tardías marcadas", "total", n)
		}
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			run()
		}
	}
}
