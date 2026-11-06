package mongo

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeEnsurer cuenta las llamadas y devuelve err; simula un repositorio.
type fakeEnsurer struct {
	err   error
	calls int
}

func (f *fakeEnsurer) EnsureIndexes(context.Context) error {
	f.calls++
	return f.err
}

func TestEnsureAllRunsEveryEnsurer(t *testing.T) {
	a, b := &fakeEnsurer{}, &fakeEnsurer{}

	if err := EnsureAll(context.Background(), a, b); err != nil {
		t.Fatalf("EnsureAll() = %v, want nil", err)
	}
	if a.calls != 1 || b.calls != 1 {
		t.Errorf("llamadas = %d y %d, want 1 y 1", a.calls, b.calls)
	}
}

func TestEnsureAllEmptyList(t *testing.T) {
	if err := EnsureAll(context.Background()); err != nil {
		t.Errorf("EnsureAll() sin ensurers = %v, want nil", err)
	}
}

func TestEnsureAllAccumulatesErrors(t *testing.T) {
	errA := errors.New("fallo A")
	errC := errors.New("fallo C")
	a := &fakeEnsurer{err: errA}
	b := &fakeEnsurer{}
	c := &fakeEnsurer{err: errC}

	err := EnsureAll(context.Background(), a, b, c)

	if err == nil {
		t.Fatal("EnsureAll() = nil, want error")
	}
	if a.calls != 1 || b.calls != 1 || c.calls != 1 {
		t.Errorf("debe ejecutar todos aunque falle uno: %d, %d, %d", a.calls, b.calls, c.calls)
	}
	if !errors.Is(err, errA) || !errors.Is(err, errC) {
		t.Errorf("errors.Is debe encontrar ambas causas: %v", err)
	}
	for _, want := range []string{"ensurer 0", "ensurer 2", "fallo A", "fallo C"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("el error %q no contiene %q", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "ensurer 1") {
		t.Errorf("el ensurer 1 no falló: %q", err.Error())
	}
}

func TestEnsureAllIsIdempotentWhenEnsurersAre(t *testing.T) {
	e := &fakeEnsurer{}
	ctx := context.Background()

	for range 2 {
		if err := EnsureAll(ctx, e); err != nil {
			t.Fatalf("EnsureAll() = %v, want nil", err)
		}
	}
	if e.calls != 2 {
		t.Errorf("llamadas = %d, want 2 (una por ejecución)", e.calls)
	}
}

func TestEnsureAllNilEnsurer(t *testing.T) {
	ok := &fakeEnsurer{}

	err := EnsureAll(context.Background(), nil, ok)

	if err == nil || !strings.Contains(err.Error(), "ensurer 0 es nil") {
		t.Errorf("err = %v, want error por ensurer nil", err)
	}
	if ok.calls != 1 {
		t.Errorf("el resto debe ejecutarse: calls = %d", ok.calls)
	}
}
