package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

// fakeValidatorTool simula el estado de los validadores y registra lo aplicado.
type fakeValidatorTool struct {
	fakeNullSource
	state    adaptermongo.Strictness
	stateErr error
	skipped  []string
	applyErr error
	applied  []bool // un elemento por Apply: strict
}

func (f *fakeValidatorTool) State(context.Context, adaptermongo.ValidatorSpec) (adaptermongo.Strictness, error) {
	return f.state, f.stateErr
}

func (f *fakeValidatorTool) Apply(_ context.Context, specs []adaptermongo.ValidatorSpec, strict bool) (adaptermongo.ValidatorsResult, error) {
	f.applied = append(f.applied, strict)
	if f.applyErr != nil {
		return adaptermongo.ValidatorsResult{}, f.applyErr
	}
	res := adaptermongo.ValidatorsResult{Skipped: f.skipped}
	for _, sp := range specs {
		if !contains(f.skipped, sp.Collection) {
			res.Updated = append(res.Updated, sp.Collection)
		}
	}
	return res, nil
}

func contains(l []string, s string) bool {
	for _, e := range l {
		if e == s {
			return true
		}
	}
	return false
}

func runEnforce(t *testing.T, env string, tool *fakeValidatorTool, openErr error, args ...string) (int, string) {
	t.Helper()
	load := func() (config.Config, error) { return config.Config{AppEnv: env}, nil }
	open := func(context.Context, config.Config) (validatorTool, func(), error) {
		if openErr != nil {
			return nil, nil, openErr
		}
		return tool, func() {}, nil
	}
	var out, errOut bytes.Buffer
	code := runEnforceValidators(args, &out, &errOut, load, open)
	return code, out.String() + errOut.String()
}

func cleanTool() *fakeValidatorTool {
	return &fakeValidatorTool{
		fakeNullSource: fakeNullSource{docs: map[string][]bson.D{"users": {{{Key: "_id", Value: "u1"}}}}},
		state:          adaptermongo.StrictnessTolerant,
	}
}

// Por omisión es un dry-run: muestra el estado y no aplica nada.
func TestEnforceValidatorsDryRunByDefault(t *testing.T) {
	tool := cleanTool()
	code, out := runEnforce(t, config.EnvStaging, tool, nil)
	if code != enforceOK || len(tool.applied) != 0 {
		t.Fatalf("código %d, aplicado %v:\n%s", code, tool.applied, out)
	}
	for _, want := range []string{"users", "tolerante", "cero null", "dry-run", "--apply"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
}

// Con --apply endurece solo si la verificación da cero null.
func TestEnforceValidatorsApplyStrictAfterCleanVerification(t *testing.T) {
	tool := cleanTool()
	code, out := runEnforce(t, config.EnvStaging, tool, nil, "--apply")
	if code != enforceOK || len(tool.applied) != 1 || !tool.applied[0] {
		t.Fatalf("código %d, aplicado %v:\n%s", code, tool.applied, out)
	}
	if !strings.Contains(out, "validador estricto") {
		t.Errorf("salida:\n%s", out)
	}
}

// Seguridad del despliegue: con null no se endurece, ni siquiera con --apply.
func TestEnforceValidatorsRefusesWithNulls(t *testing.T) {
	tool := cleanTool()
	tool.docs = map[string][]bson.D{"users": {{{Key: "_id", Value: "u1"}, {Key: "phone", Value: nil}}}}
	for _, args := range [][]string{nil, {"--apply"}} {
		code, out := runEnforce(t, config.EnvStaging, tool, nil, args...)
		if code != enforceRefused || len(tool.applied) != 0 {
			t.Fatalf("args %v: código %d, aplicado %v:\n%s", args, code, tool.applied, out)
		}
		if !strings.Contains(out, "no se endurece") || !strings.Contains(out, "users.phone") {
			t.Errorf("debe explicar y mostrar la ruta:\n%s", out)
		}
	}
}

// El camino de vuelta no exige verificación y aplica el tolerante.
func TestEnforceValidatorsTolerantRollback(t *testing.T) {
	tool := cleanTool()
	tool.state = adaptermongo.StrictnessStrict
	tool.docs = map[string][]bson.D{"users": {{{Key: "phone", Value: nil}}}}
	code, out := runEnforce(t, config.EnvStaging, tool, nil, "--apply", "--tolerant")
	if code != enforceOK || len(tool.applied) != 1 || tool.applied[0] {
		t.Fatalf("código %d, aplicado %v:\n%s", code, tool.applied, out)
	}
}

func TestEnforceValidatorsProductionNeedsConfirmation(t *testing.T) {
	tool := cleanTool()
	if code, out := runEnforce(t, config.EnvProduction, tool, nil, "--apply"); code != enforceFailure || len(tool.applied) != 0 || !strings.Contains(out, "--yes-production") {
		t.Fatalf("código %d:\n%s", code, out)
	}
	if code, _ := runEnforce(t, config.EnvProduction, tool, nil, "--apply", "--yes-production"); code != enforceOK || len(tool.applied) != 1 {
		t.Fatalf("con confirmación debe aplicar: código %d", code)
	}
	// El dry-run no pide confirmación.
	if code, _ := runEnforce(t, config.EnvProduction, cleanTool(), nil); code != enforceOK {
		t.Fatalf("dry-run en producción: código %d", code)
	}
}

func TestEnforceValidatorsSkippedWithoutPermission(t *testing.T) {
	tool := cleanTool()
	tool.skipped = []string{"users"}
	code, out := runEnforce(t, config.EnvStaging, tool, nil, "--apply")
	if code != enforceRefused || !strings.Contains(out, "dbAdmin") {
		t.Fatalf("código %d:\n%s", code, out)
	}
}

func TestEnforceValidatorsErrors(t *testing.T) {
	if code, _ := runEnforce(t, config.EnvStaging, cleanTool(), errors.New("sin conexión")); code != enforceFailure {
		t.Errorf("fallo al abrir: %d", code)
	}
	st := cleanTool()
	st.stateErr = errors.New("listar")
	if code, _ := runEnforce(t, config.EnvStaging, st, nil); code != enforceFailure {
		t.Errorf("fallo al leer el estado: %d", code)
	}
	ap := cleanTool()
	ap.applyErr = errors.New("collMod")
	if code, _ := runEnforce(t, config.EnvStaging, ap, nil, "--apply"); code != enforceFailure {
		t.Errorf("fallo al aplicar: %d", code)
	}
	vf := cleanTool()
	vf.scanErr = errors.New("lectura")
	if code, _ := runEnforce(t, config.EnvStaging, vf, nil); code != enforceFailure {
		t.Errorf("fallo al verificar: %d", code)
	}
	for _, args := range [][]string{{"extra"}, {"--desconocida"}} {
		if code, _ := runEnforce(t, config.EnvStaging, cleanTool(), nil, args...); code != enforceFailure {
			t.Errorf("args %v: %d", args, code)
		}
	}
}

func TestAllValidatorSpecsCoverEveryCollection(t *testing.T) {
	specs := allValidatorSpecs()
	if len(specs) != 18 { // users, refresh_tokens, items + 15 de campos opcionales
		t.Errorf("colecciones = %d, want 18", len(specs))
	}
	seen := map[string]bool{}
	for _, sp := range specs {
		if seen[sp.Collection] {
			t.Errorf("colección repetida: %s", sp.Collection)
		}
		seen[sp.Collection] = true
	}
}
