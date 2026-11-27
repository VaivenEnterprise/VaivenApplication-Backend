package main

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

func TestPlanNulls(t *testing.T) {
	doc := bson.D{
		{Key: "_id", Value: "u1"},
		{Key: "phone", Value: nil},
		{Key: "zone", Value: bson.D{{Key: "city", Value: "CDMX"}, {Key: "neighborhood", Value: nil}}},
		{Key: "payments", Value: bson.A{
			bson.D{{Key: "paidAt", Value: nil}},
			bson.D{{Key: "paidAt", Value: int32(1)}, {Key: "ref", Value: bson.D{{Key: "x", Value: nil}}}},
		}},
		{Key: "tags", Value: bson.A{"a", nil, "b", nil}},
	}
	raw, _ := bson.Marshal(doc)
	unset, pull, unsup, err := planNulls(raw)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(unset)
	wantUnset := []string{"payments.0.paidAt", "payments.1.ref.x", "phone", "zone.neighborhood"}
	if !slices.Equal(unset, wantUnset) {
		t.Errorf("unset = %v, want %v", unset, wantUnset)
	}
	if !slices.Equal(pull, []string{"tags"}) {
		t.Errorf("pull = %v, want [tags]", pull)
	}
	if len(unsup) != 0 {
		t.Errorf("no soportadas = %v, want ninguna", unsup)
	}
}

func TestPlanNullsUnsupportedDepth(t *testing.T) {
	doc := bson.D{
		{Key: "a", Value: bson.A{bson.D{{Key: "b", Value: bson.A{nil, bson.D{{Key: "c", Value: nil}}}}}}},
		{Key: "m", Value: bson.A{bson.A{nil}}},
		{Key: "mixed", Value: bson.A{nil, bson.D{{Key: "x", Value: nil}}}},
		{Key: "ok", Value: nil},
	}
	raw, _ := bson.Marshal(doc)
	unset, pull, unsup, err := planNulls(raw)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(unset)
	if !slices.Equal(unset, []string{"mixed.1.x", "ok"}) || len(pull) != 0 {
		t.Errorf("unset = %v pull = %v; solo ok y mixed.1.x son soportados", unset, pull)
	}
	for _, p := range []string{"a[].b[]", "a[].b[].c", "m[][]", "mixed[]"} {
		if unsup[p] == 0 {
			t.Errorf("falta la ruta no soportada %q en %v", p, unsup)
		}
	}
}

func TestPlanNullsClean(t *testing.T) {
	raw, _ := bson.Marshal(bson.D{{Key: "a", Value: 1}, {Key: "b", Value: bson.A{}}})
	u, p, s, err := planNulls(raw)
	if err != nil || len(u)+len(p)+len(s) != 0 {
		t.Fatalf("u=%v p=%v s=%v err=%v", u, p, s, err)
	}
}

// fakeMigrator sirve documentos y guarda los lotes que recibe.
type fakeMigrator struct {
	fakeNullSource
	batches  [][]nullOp
	applyErr error
}

func (f *fakeMigrator) Apply(_ context.Context, _ string, ops []nullOp) (int64, error) {
	if f.applyErr != nil {
		return 0, f.applyErr
	}
	f.batches = append(f.batches, slices.Clone(ops))
	return int64(len(ops)), nil
}

func nullDocs(n int) []bson.D {
	out := make([]bson.D, n)
	for i := range out {
		out[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "phone", Value: nil}}
	}
	return out
}

func TestMigrateNullsDryRunWritesNothing(t *testing.T) {
	m := &fakeMigrator{fakeNullSource: fakeNullSource{docs: map[string][]bson.D{"users": nullDocs(3), "items": {{{Key: "_id", Value: 1}}}}}}
	var out, errb bytes.Buffer
	if code := migrateNulls(context.Background(), &out, &errb, "", m, false, ""); code != migrateOK {
		t.Fatalf("código = %d\n%s%s", code, out.String(), errb.String())
	}
	if len(m.batches) != 0 {
		t.Error("el dry-run no debe escribir")
	}
	if !strings.Contains(out.String(), "DRY-RUN") || !strings.Contains(out.String(), "cambiaría=3") {
		t.Errorf("salida inesperada:\n%s", out.String())
	}
}

func TestMigrateNullsApplyBatchesAndReportsBackupRef(t *testing.T) {
	m := &fakeMigrator{fakeNullSource: fakeNullSource{docs: map[string][]bson.D{"users": nullDocs(2500)}}}
	var out, errb bytes.Buffer
	if code := migrateNulls(context.Background(), &out, &errb, "", m, true, "resp-2026-11-26"); code != migrateOK {
		t.Fatalf("código = %d\n%s%s", code, out.String(), errb.String())
	}
	if len(m.batches) != 3 || len(m.batches[0]) != 1000 || len(m.batches[1]) != 1000 || len(m.batches[2]) != 500 {
		sizes := []int{}
		for _, b := range m.batches {
			sizes = append(sizes, len(b))
		}
		t.Errorf("lotes = %v, want [1000 1000 500]", sizes)
	}
	if !strings.Contains(out.String(), `backup-ref="resp-2026-11-26"`) || !strings.Contains(out.String(), "modificó=2500") {
		t.Errorf("salida sin backup-ref o conteos:\n%s", out.String())
	}
	if op := m.batches[0][0]; len(op.Unset) != 1 || op.Unset[0] != "phone" || op.ID.Type != bson.TypeInt32 {
		t.Errorf("op = %+v", op)
	}
}

func TestMigrateNullsUnsupportedExitsOneAndErrors(t *testing.T) {
	m := &fakeMigrator{fakeNullSource: fakeNullSource{docs: map[string][]bson.D{"x": {{{Key: "m", Value: bson.A{bson.A{nil}}}}}}}}
	var out bytes.Buffer
	if code := migrateNulls(context.Background(), &out, &bytes.Buffer{}, "", m, true, "r"); code != migrateUnsupported {
		t.Errorf("código = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "NO SOPORTADA") || len(m.batches) != 0 {
		t.Errorf("debe reportar y no tocar:\n%s", out.String())
	}

	const uri = "mongodb://u:clave-secreta@h:27017"
	fail := &fakeMigrator{fakeNullSource: fakeNullSource{docs: map[string][]bson.D{"users": nullDocs(1)}}, applyErr: errors.New("boom " + uri)}
	var errb bytes.Buffer
	if code := migrateNulls(context.Background(), &bytes.Buffer{}, &errb, uri, fail, true, "r"); code != migrateFailure || strings.Contains(errb.String(), "clave-secreta") {
		t.Errorf("código = %d, stderr=%s", code, errb.String())
	}
}

func TestRunMigrateNullsGuards(t *testing.T) {
	opened := 0
	factory := func(context.Context, config.Config) (nullMigrator, func(), error) {
		opened++
		return &fakeMigrator{}, func() {}, nil
	}
	prod := func() (config.Config, error) { return config.Config{AppEnv: config.EnvProduction}, nil }
	cases := []struct {
		name string
		args []string
		load func() (config.Config, error)
		want int
		open int
	}{
		{"por omisión es dry-run", nil, loadOK, 0, 1},
		{"--dry-run explícito", []string{"--dry-run"}, loadOK, 0, 1},
		{"apply sin backup-ref", []string{"--apply"}, loadOK, 2, 0},
		{"apply con backup-ref vacío", []string{"--apply", "--backup-ref", "  "}, loadOK, 2, 0},
		{"apply y dry-run", []string{"--apply", "--dry-run", "--backup-ref", "r"}, loadOK, 2, 0},
		{"apply con backup-ref", []string{"--apply", "--backup-ref", "r"}, loadOK, 0, 1},
		{"producción sin yes", []string{"--apply", "--backup-ref", "r"}, prod, 2, 0},
		{"producción con yes", []string{"--apply", "--backup-ref", "r", "--yes-production"}, prod, 0, 1},
		{"producción dry-run libre", nil, prod, 0, 1},
		{"opción desconocida", []string{"--nope"}, loadOK, 2, 0},
		{"argumento suelto", []string{"x"}, loadOK, 2, 0},
		{"backup-ref larguísimo", []string{"--apply", "--backup-ref", strings.Repeat("a", 201)}, loadOK, 2, 0},
	}
	for _, c := range cases {
		opened = 0
		if code := runMigrateNulls(c.args, &bytes.Buffer{}, &bytes.Buffer{}, c.load, factory); code != c.want || opened != c.open {
			t.Errorf("%s: código = %d (abierta %d), want %d (%d)", c.name, code, opened, c.want, c.open)
		}
	}
	badLoad := func() (config.Config, error) { return config.Config{}, errors.New("config") }
	if code := runMigrateNulls(nil, &bytes.Buffer{}, &bytes.Buffer{}, badLoad, factory); code != migrateFailure {
		t.Errorf("config inválida: código = %d", code)
	}
}

func TestRunMigrateViaRun(t *testing.T) {
	factory := func(context.Context, config.Config) (nullMigrator, func(), error) {
		return &fakeMigrator{fakeNullSource: fakeNullSource{docs: map[string][]bson.D{"u": nullDocs(1)}}}, func() {}, nil
	}
	var out bytes.Buffer
	code := run([]string{"migrate-remove-nulls", "--apply", "--backup-ref", "r"}, &out, &bytes.Buffer{}, loadOK, nil, nil, nil, nil, extraDeps{migrate: factory})
	if code != 0 || !strings.Contains(out.String(), "modificó=1") {
		t.Errorf("código = %d\n%s", code, out.String())
	}
}
