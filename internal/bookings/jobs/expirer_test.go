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
)

type fakeExpirer struct {
	mu    sync.Mutex
	calls int
	errs  []error
	done  chan struct{}
}

func (f *fakeExpirer) ExpireDue(context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	var err error
	if len(f.errs) > 0 {
		err, f.errs = f.errs[0], f.errs[1:]
	}
	if f.calls == 3 {
		close(f.done)
	}
	if err != nil {
		return 0, err
	}
	return f.calls, nil
}

func TestRunExpirer(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	f := &fakeExpirer{errs: []error{nil, errors.New("mongo caído")}, done: make(chan struct{})}
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { RunExpirer(ctx, f, tick, log); close(finished) }()

	tick <- time.Now() // segunda pasada: error registrado, sigue
	tick <- time.Now() // tercera pasada
	<-f.done
	cancel()
	<-finished
	out := buf.String()
	if !strings.Contains(out, "error al vencer") || !strings.Contains(out, "solicitudes vencidas") {
		t.Errorf("logs = %s", out)
	}
	if f.calls != 3 {
		t.Errorf("pasadas = %d", f.calls)
	}
}

func TestRunExpirerDefaultTicker(t *testing.T) {
	f := &fakeExpirer{done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // corre la primera pasada y termina
	RunExpirer(ctx, f, nil, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if f.calls != 1 {
		t.Errorf("pasadas = %d", f.calls)
	}
}
