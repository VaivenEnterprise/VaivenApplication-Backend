package jobs

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDetector struct {
	calls atomic.Int32
	n     int
	err   error
}

func (f *fakeDetector) DetectLate(context.Context) (int, error) {
	f.calls.Add(1)
	return f.n, f.err
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("tiempo agotado esperando la condición")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRunLateDetectorRunsAtStartAndOnEveryTick(t *testing.T) {
	d := &fakeDetector{n: 2}
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		RunLateDetector(ctx, d, tick, slog.New(slog.NewTextHandler(&logs, nil)))
		close(done)
	}()
	waitFor(t, func() bool { return d.calls.Load() == 1 }) // al arrancar
	tick <- time.Now()
	tick <- time.Now()
	waitFor(t, func() bool { return d.calls.Load() == 3 })
	cancel()
	<-done
	if !strings.Contains(logs.String(), "devoluciones tardías marcadas") {
		t.Errorf("debe registrar lo que marcó: %s", logs.String())
	}
}

func TestRunLateDetectorLogsErrorsAndKeepsGoing(t *testing.T) {
	d := &fakeDetector{err: errors.New("mongo caído")}
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		RunLateDetector(ctx, d, tick, slog.New(slog.NewTextHandler(&logs, nil)))
		close(done)
	}()
	waitFor(t, func() bool { return d.calls.Load() == 1 })
	tick <- time.Now()
	waitFor(t, func() bool { return d.calls.Load() == 2 })
	cancel()
	<-done
	if !strings.Contains(logs.String(), "error al detectar retrasos") {
		t.Errorf("debe registrar el error: %s", logs.String())
	}
}

func TestRunLateDetectorDefaultTickerStopsWithContext(t *testing.T) {
	if Interval != 5*time.Minute {
		t.Errorf("Interval = %v, quiero 5 min (plan P-05)", Interval)
	}
	d := &fakeDetector{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunLateDetector(ctx, d, nil, slog.Default())
		close(done)
	}()
	waitFor(t, func() bool { return d.calls.Load() == 1 })
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("no terminó al cancelar el contexto")
	}
}
