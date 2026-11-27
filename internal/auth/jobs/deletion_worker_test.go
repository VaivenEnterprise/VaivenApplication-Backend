package jobs

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
)

type fakeResumer struct {
	mu    sync.Mutex
	calls int
	steps []func() (usecase.PassReport, error)
	done  chan struct{}
}

func (f *fakeResumer) ResumeDeletions(context.Context) (usecase.PassReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == 3 {
		close(f.done)
	}
	if len(f.steps) >= f.calls {
		return f.steps[f.calls-1]()
	}
	return usecase.PassReport{}, nil
}

func TestRunDeletionWorker(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	f := &fakeResumer{done: make(chan struct{}), steps: []func() (usecase.PassReport, error){
		func() (usecase.PassReport, error) {
			return usecase.PassReport{JobsDone: 2, Accounts: 1, Cleared: 1}, nil
		},
		func() (usecase.PassReport, error) { return usecase.PassReport{}, errors.New("mongo caído") },
		func() (usecase.PassReport, error) { return usecase.PassReport{}, nil },
	}}
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { RunDeletionWorker(ctx, f, tick, log); close(finished) }()
	tick <- time.Now() // segunda pasada: error registrado, sigue
	tick <- time.Now() // tercera pasada: sin trabajo, no registra
	<-f.done
	cancel()
	<-finished
	out := buf.String()
	if !strings.Contains(out, "error al reanudar") || !strings.Contains(out, "bajas de cuenta reanudadas") || !strings.Contains(out, "jobsDone=2") {
		t.Errorf("logs = %s", out)
	}
	if strings.Count(out, "bajas de cuenta reanudadas") != 1 {
		t.Errorf("una pasada sin trabajo no debe registrar: %s", out)
	}
	if f.calls != 3 {
		t.Errorf("pasadas = %d", f.calls)
	}
}

func TestRunDeletionWorkerDefaultTickerAndCancel(t *testing.T) {
	f := &fakeResumer{done: make(chan struct{}), steps: []func() (usecase.PassReport, error){
		func() (usecase.PassReport, error) { return usecase.PassReport{}, context.Canceled },
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // corre la primera pasada y termina; un error por cancelación no se registra
	var buf bytes.Buffer
	RunDeletionWorker(ctx, f, nil, slog.New(slog.NewTextHandler(&buf, nil)))
	if f.calls != 1 || buf.Len() != 0 {
		t.Errorf("pasadas = %d, logs = %q", f.calls, buf.String())
	}
}
