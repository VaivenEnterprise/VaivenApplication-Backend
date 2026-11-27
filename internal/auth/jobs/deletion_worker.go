// Package jobs ejecuta las tareas periódicas del módulo de cuentas (spec 021,
// T021; plan §4). Sin estado propio (Art. XIV): cada pasada es idempotente, así
// que varias réplicas pueden correrla a la vez y reanudar una baja interrumpida
// da el mismo resultado.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
)

// Interval es cada cuánto se reanudan las bajas pendientes (mismo ritmo que el
// vencimiento de solicitudes de bookings/jobs).
const Interval = 60 * time.Second

// Resumer es el caso de uso que reanuda las bajas (usecase.Service o
// usecase.Resumer).
type Resumer interface {
	ResumeDeletions(ctx context.Context) (usecase.PassReport, error)
}

// RunDeletionWorker hace una pasada al arrancar y otra en cada tick hasta que
// ctx se cancela. tick nil usa un time.Ticker de Interval (se inyecta en
// pruebas). Un error se registra y la tarea sigue; los registros llevan solo
// contadores, nunca ids, correos ni textos (RNF-03).
func RunDeletionWorker(ctx context.Context, r Resumer, tick <-chan time.Time, log *slog.Logger) {
	if tick == nil {
		t := time.NewTicker(Interval)
		defer t.Stop()
		tick = t.C
	}
	run := func() {
		rep, err := r.ResumeDeletions(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			log.ErrorContext(ctx, "auth: error al reanudar bajas de cuenta", "error", err)
		case rep.JobsDone+rep.JobsRetried+rep.JobsFailed+rep.Accounts > 0:
			log.InfoContext(ctx, "auth: bajas de cuenta reanudadas",
				"jobsDone", rep.JobsDone, "jobsRetried", rep.JobsRetried, "jobsFailed", rep.JobsFailed,
				"accounts", rep.Accounts, "cleared", rep.Cleared)
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
