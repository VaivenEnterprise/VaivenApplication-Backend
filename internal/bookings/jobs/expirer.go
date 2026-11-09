// Package jobs ejecuta las tareas periódicas del módulo de reservas (spec
// 007, T008; plan P-04). Sin estado propio (Art. XIV): cada pasada es
// idempotente, así que varias réplicas pueden correrla a la vez.
package jobs

import (
	"context"
	"log/slog"
	"time"
)

// Interval es cada cuánto se vencen las solicitudes (plan P-04).
const Interval = 60 * time.Second

// Expirer es el caso de uso que vence solicitudes (usecase.Service).
type Expirer interface {
	ExpireDue(ctx context.Context) (int, error)
}

// RunExpirer llama a ExpireDue al arrancar y en cada tick hasta que ctx se
// cancela. tick nil usa un time.Ticker de Interval (se inyecta en pruebas).
// Un error se registra y la tarea sigue.
func RunExpirer(ctx context.Context, e Expirer, tick <-chan time.Time, log *slog.Logger) {
	if tick == nil {
		t := time.NewTicker(Interval)
		defer t.Stop()
		tick = t.C
	}
	run := func() {
		n, err := e.ExpireDue(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			log.ErrorContext(ctx, "bookings: error al vencer solicitudes", "error", err)
		case n > 0:
			log.InfoContext(ctx, "bookings: solicitudes vencidas", "total", n)
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
