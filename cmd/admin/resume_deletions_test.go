package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	authusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

type fakeDeletionResumer struct {
	rep authusecase.PassReport
	err error
}

func (f fakeDeletionResumer) ResumeDeletions(context.Context) (authusecase.PassReport, error) {
	return f.rep, f.err
}

func resumeFactoryOf(r deletionResumer, closed *bool, warnings ...string) resumeFactory {
	return func(context.Context, config.Config) (deletionResumer, func(), []string, error) {
		return r, func() { *closed = true }, warnings, nil
	}
}

// Spec 021, T021: `admin resume-deletions` (N-1), con códigos de salida 0/1/2 y
// solo contadores en la salida.
func TestResumeDeletionsCommand(t *testing.T) {
	var closed bool
	var out, errb bytes.Buffer
	f := resumeFactoryOf(fakeDeletionResumer{rep: authusecase.PassReport{JobsDone: 3, JobsRetried: 1, Accounts: 2, Cleared: 2}}, &closed, "Redis no está configurado")
	code := run([]string{"resume-deletions"}, &out, &errb, loadOK, nil, nil, nil, nil, extraDeps{resume: f})
	if code != resumeOK || !closed {
		t.Fatalf("código = %d, cerrada = %v; stderr = %s", code, closed, errb.String())
	}
	want := "jobs hechos: 3, reintentos programados: 1, jobs agotados: 0, cuentas revisadas: 2, marcas quitadas: 2\n"
	if out.String() != want {
		t.Errorf("salida = %q, want %q", out.String(), want)
	}
	if !strings.Contains(errb.String(), "aviso: Redis no está configurado") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestResumeDeletionsExitCodes(t *testing.T) {
	cases := map[string]struct {
		load func() (config.Config, error)
		open resumeFactory
		want int
	}{
		"agotados":          {loadOK, resumeFactoryOf(fakeDeletionResumer{rep: authusecase.PassReport{JobsFailed: 1}}, new(bool)), resumeFailed},
		"pasada con error":  {loadOK, resumeFactoryOf(fakeDeletionResumer{err: errors.New("mongo caído")}, new(bool)), resumeFailure},
		"sin configuración": {func() (config.Config, error) { return config.Config{}, errors.New("falta MONGODB_URI") }, resumeFactoryOf(fakeDeletionResumer{}, new(bool)), resumeFailure},
		"sin conexión": {loadOK, func(context.Context, config.Config) (deletionResumer, func(), []string, error) {
			return nil, nil, nil, errors.New("no conecta")
		}, resumeFailure},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := runResumeDeletions(&out, &errb, c.load, c.open); code != c.want {
				t.Errorf("código = %d, want %d (stdout %q stderr %q)", code, c.want, out.String(), errb.String())
			}
		})
	}
}

func TestResumeDeletionsViaRunLoadError(t *testing.T) {
	var errb bytes.Buffer
	bad := func() (config.Config, error) { return config.Config{}, errors.New("falta MONGODB_URI") }
	if code := run([]string{"resume-deletions"}, &bytes.Buffer{}, &errb, bad, nil, nil, nil, nil); code != resumeFailure {
		t.Errorf("código = %d", code)
	}
	if code := run([]string{"resume-deletions", "--extra"}, &bytes.Buffer{}, &errb, loadOK, nil, nil, nil, nil); code != 2 {
		t.Errorf("con argumentos extra = %d, want 2 (uso)", code)
	}
	if !strings.Contains(usage, "resume-deletions") {
		t.Error("la ayuda debe listar resume-deletions")
	}
}

func TestResolveExtrasDefaultsIncludeResume(t *testing.T) {
	if d := resolveExtras(nil); d.resume == nil || d.nulls == nil || d.migrate == nil {
		t.Errorf("valores por omisión incompletos: %+v", d)
	}
}
