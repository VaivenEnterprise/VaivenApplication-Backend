package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	recousecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/usecase"
)

var errReco = errors.New("falla de prueba")

// fakeReco cumple todos los puertos de recoDeps.
type fakeReco struct {
	res       recousecase.RecomputeResult
	err       map[string]bool
	inserted  []domain.Event
	purged    bool
	items     []domain.ItemFeatures
	views     []domain.Event
	closed    bool
	opened    bool
	failOpen  bool
	sinceArgs []time.Time
}

func (f *fakeReco) fail(op string) error {
	if f.err[op] {
		return errReco
	}
	return nil
}

func (f *fakeReco) Recompute(context.Context) (recousecase.RecomputeResult, error) {
	return f.res, f.fail("recompute")
}

func (f *fakeReco) InsertDemo(_ context.Context, events []domain.Event) (int, error) {
	f.inserted = events
	return len(events), f.fail("insert")
}

func (f *fakeReco) PurgeDemo(context.Context) (int64, error) {
	f.purged = true
	return 7, f.fail("purge")
}

func (f *fakeReco) ActiveFeatures(context.Context) ([]domain.ItemFeatures, error) {
	return f.items, f.fail("items")
}

func (f *fakeReco) Since(_ context.Context, since time.Time, _ int) ([]domain.Event, error) {
	f.sinceArgs = append(f.sinceArgs, since)
	return f.views, f.fail("since")
}

var recoNow = time.Date(2026, 11, 17, 18, 0, 0, 0, time.UTC)

func (f *fakeReco) factory(context.Context, config.Config) (recoDeps, error) {
	f.opened = true
	if f.failOpen {
		return recoDeps{}, errReco
	}
	return recoDeps{recompute: f, demo: f, items: f, views: f, requests: f, now: func() time.Time { return recoNow },
		close: func() { f.closed = true }}, nil
}

func loadEnv(env string) func() (config.Config, error) {
	return func() (config.Config, error) { return config.Config{AppEnv: env}, nil }
}

func loadFail() (config.Config, error) { return config.Config{}, errReco }

func activeItems(n int) []domain.ItemFeatures {
	out := make([]domain.ItemFeatures, n)
	for i := range out {
		out[i] = domain.ItemFeatures{ID: strings.Repeat("a", 23) + string(rune('0'+i%10)), Category: "hogar", Municipality: "Tula", Modality: "loan"}
	}
	return out
}

func TestRecoEvalSyntheticIsReproducible(t *testing.T) {
	var a, b bytes.Buffer
	f := &fakeReco{}
	if code := runRecoEval(nil, &a, &bytes.Buffer{}, loadFail, f.factory); code != 0 {
		t.Fatalf("código %d", code)
	}
	runRecoEval([]string{"--k", "10"}, &b, &bytes.Buffer{}, loadFail, f.factory)
	if a.String() != b.String() {
		t.Error("la salida no es reproducible")
	}
	for _, s := range []string{"| Híbrido (α = 0.6) | 0.263 |", "| Popularidad (línea base) | 0.087 |", "Personas evaluadas: 300", "semilla 15"} {
		if !strings.Contains(a.String(), s) {
			t.Errorf("falta %q en:\n%s", s, a.String())
		}
	}
	if f.opened {
		t.Error("la fuente sintética no debe abrir la base")
	}
}

func TestRecoEvalDB(t *testing.T) {
	f := &fakeReco{items: activeItems(3), views: []domain.Event{{UserID: "u", ItemID: "x", Weight: 1}}}
	var out, errb bytes.Buffer
	if code := runRecoEval([]string{"--source", "db"}, &out, &errb, loadEnv(config.EnvStaging), f.factory); code != 0 {
		t.Fatalf("código %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "interacciones de staging") || !f.closed || len(f.sinceArgs) != 2 || !f.sinceArgs[0].Equal(recoNow.Add(-domain.Window)) {
		t.Errorf("salida = %s, since = %v", out.String(), f.sinceArgs)
	}
	if code := runRecoEval([]string{"--source", "db"}, &out, &errb, loadEnv(config.EnvProduction), (&fakeReco{}).factory); code != 1 {
		t.Error("--source db debe negarse en producción")
	}
	for _, op := range []string{"since", "items"} {
		g := &fakeReco{err: map[string]bool{op: true}}
		if code := runRecoEval([]string{"--source=db"}, &out, &errb, loadEnv(config.EnvTest), g.factory); code != 1 {
			t.Errorf("%s: código %d", op, code)
		}
	}
	if code := runRecoEval([]string{"--source=db"}, &out, &errb, loadFail, f.factory); code != 1 {
		t.Error("config inválida")
	}
	if code := runRecoEval([]string{"--source=db"}, &out, &errb, loadEnv(config.EnvTest), (&fakeReco{failOpen: true}).factory); code != 1 {
		t.Error("sin conexión")
	}
	for _, args := range [][]string{{"--source", "otra"}, {"--k", "0"}, {"--k", "51"}, {"extra"}, {"--nada"}} {
		if code := runRecoEval(args, &out, &errb, loadFail, f.factory); code != 2 {
			t.Errorf("%v: código %d", args, code)
		}
	}
}

func TestRecoRecompute(t *testing.T) {
	f := &fakeReco{res: recousecase.RecomputeResult{Built: true, Interactions: 12, Items: 4, Purged: 1}}
	var out, errb bytes.Buffer
	if code := runRecoRecompute(&out, &errb, loadEnv(config.EnvProduction), f.factory); code != 0 ||
		!strings.Contains(out.String(), "12 interacciones, 4 objetos activos, 1 cuentas") || !f.closed {
		t.Fatalf("código %d: %s", code, out.String())
	}
	busy := &fakeReco{}
	if code := runRecoRecompute(&out, &errb, loadEnv(config.EnvStaging), busy.factory); code != 1 {
		t.Error("con el candado ocupado debe salir con 1")
	}
	for name, factory := range map[string]recoFactory{
		"recálculo": (&fakeReco{err: map[string]bool{"recompute": true}}).factory,
		"conexión":  (&fakeReco{failOpen: true}).factory,
	} {
		if code := runRecoRecompute(&out, &errb, loadEnv(config.EnvStaging), factory); code != 1 {
			t.Errorf("%s: código %d", name, code)
		}
	}
	if code := runRecoRecompute(&out, &errb, loadFail, f.factory); code != 1 {
		t.Error("config inválida")
	}
}

func TestRecoSeed(t *testing.T) {
	f := &fakeReco{items: activeItems(5)}
	var out, errb bytes.Buffer
	if code := runRecoSeed([]string{"--users", "10"}, &out, &errb, loadEnv(config.EnvStaging), f.factory); code != 0 {
		t.Fatalf("código %d: %s", code, errb.String())
	}
	if len(f.inserted) != 10*12 || !f.closed {
		t.Fatalf("insertados = %d", len(f.inserted))
	}
	for _, e := range f.inserted {
		if !strings.HasPrefix(e.UserID, "demo-") || !e.At.Before(recoNow) || e.At.Before(recoNow.Add(-domain.Window)) {
			t.Fatalf("evento = %+v", e)
		}
	}
	// --purge borra sin generar.
	g := &fakeReco{}
	if code := runRecoSeed([]string{"--purge"}, &out, &errb, loadEnv(config.EnvDevelopment), g.factory); code != 0 || !g.purged || g.inserted != nil ||
		!strings.Contains(out.String(), "borradas: 7") {
		t.Errorf("purge: %d %s", code, out.String())
	}
	// Nunca en producción, y sin conectar (RF-08).
	p := &fakeReco{items: activeItems(1)}
	if code := runRecoSeed(nil, &out, &errb, loadEnv(config.EnvProduction), p.factory); code != 1 || p.opened {
		t.Error("debe negarse en producción sin conectar")
	}
	if code := runRecoSeed(nil, &out, &errb, loadEnv(config.EnvStaging), (&fakeReco{}).factory); code != 1 || !strings.Contains(errb.String(), "no hay objetos activos") {
		t.Error("sin objetos activos")
	}
	for name, ff := range map[string]*fakeReco{
		"objetos":  {err: map[string]bool{"items": true}},
		"insertar": {items: activeItems(2), err: map[string]bool{"insert": true}},
		"conexión": {failOpen: true},
	} {
		if code := runRecoSeed(nil, &out, &errb, loadEnv(config.EnvStaging), ff.factory); code != 1 {
			t.Errorf("%s: código %d", name, code)
		}
	}
	if code := runRecoSeed([]string{"--purge"}, &out, &errb, loadEnv(config.EnvStaging), (&fakeReco{err: map[string]bool{"purge": true}}).factory); code != 1 {
		t.Error("falla al purgar")
	}
	if code := runRecoSeed(nil, &out, &errb, loadFail, f.factory); code != 1 {
		t.Error("config inválida")
	}
	for _, args := range [][]string{{"--users", "0"}, {"--users", "2001"}, {"extra"}, {"--nada"}} {
		if code := runRecoSeed(args, &out, &errb, loadEnv(config.EnvStaging), f.factory); code != 2 {
			t.Errorf("%v: código %d", args, code)
		}
	}
}

func TestRunRoutesRecoCommands(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"eval-recommendations", "--k", "5"}, &out, &errb, loadFail, nil, nil, nil, nil); code != 0 || !strings.Contains(out.String(), "K=5") {
		t.Errorf("eval por run: %d %s", code, errb.String())
	}
	if code := run([]string{"recompute-recommendations", "extra"}, &out, &errb, loadFail, nil, nil, nil, nil); code != 2 {
		t.Errorf("recompute no acepta argumentos: %d", code)
	}
	for _, cmd := range []string{"recompute-recommendations", "seed-interactions"} {
		if code := run([]string{cmd}, &out, &errb, loadFail, nil, nil, nil, nil); code != 1 {
			t.Errorf("%s sin config: %d", cmd, code)
		}
	}
}
