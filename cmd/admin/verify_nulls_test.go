package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

// fakeNullSource sirve documentos sintéticos por colección.
type fakeNullSource struct {
	docs    map[string][]bson.D
	listErr error
	scanErr error
}

func (f fakeNullSource) Collections(context.Context) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	names := make([]string, 0, len(f.docs))
	for n := range f.docs {
		names = append(names, n)
	}
	// orden estable para la salida
	for i := range names {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names, nil
}

func (f fakeNullSource) Scan(_ context.Context, col string, visit func(bson.Raw) error) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	for _, d := range f.docs[col] {
		raw, err := bson.Marshal(d)
		if err != nil {
			return err
		}
		if err := visit(raw); err != nil {
			return err
		}
	}
	return nil
}

func rawOf(t *testing.T, d bson.D) bson.Raw {
	t.Helper()
	raw, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestNullPaths(t *testing.T) {
	doc := bson.D{
		{Key: "_id", Value: "u1"},
		{Key: "phone", Value: nil},
		{Key: "zone", Value: bson.D{{Key: "city", Value: "CDMX"}, {Key: "neighborhood", Value: nil}}},
		{Key: "payments", Value: bson.A{
			bson.D{{Key: "amount", Value: int64(5)}, {Key: "paidAt", Value: nil}},
			bson.D{{Key: "paidAt", Value: nil}, {Key: "ref", Value: bson.D{{Key: "x", Value: nil}}}},
		}},
		{Key: "tags", Value: bson.A{"a", nil, "b", nil}},
		{Key: "matrix", Value: bson.A{bson.A{nil, int32(1)}}},
		{Key: "ok", Value: bson.D{{Key: "a", Value: bson.A{}}, {Key: "b", Value: bson.D{}}}},
	}
	got, err := nullPaths(rawOf(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{
		"phone":             1,
		"zone.neighborhood": 1,
		"payments[].paidAt": 2,
		"payments[].ref.x":  1,
		"tags[]":            2,
		"matrix[][]":        1,
	}
	if len(got) != len(want) {
		t.Fatalf("rutas = %v, want %v", got, want)
	}
	for p, n := range want {
		if got[p] != n {
			t.Errorf("ruta %q = %d, want %d (todo: %v)", p, got[p], n, got)
		}
	}
}

func TestNullPathsClean(t *testing.T) {
	got, err := nullPaths(rawOf(t, bson.D{{Key: "a", Value: 1}, {Key: "b", Value: bson.A{}}}))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, err %v; want vacío", got, err)
	}
}

func TestNullPathsInvalidBSON(t *testing.T) {
	if _, err := nullPaths(bson.Raw{0x09, 0x00, 0x00, 0x00, 0x02, 0x61, 0x00, 0x7f, 0x00}); err == nil {
		t.Fatal("esperaba error con BSON inválido")
	}
}

func TestVerifyNullsClean(t *testing.T) {
	src := fakeNullSource{docs: map[string][]bson.D{
		"users": {{{Key: "_id", Value: "1"}}},
		"items": {},
	}}
	var out, errb bytes.Buffer
	if code := verifyNulls(context.Background(), &out, &errb, "", src); code != verifyNullsClean {
		t.Fatalf("código = %d, want 0\n%s%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "0 valores null en total") {
		t.Fatalf("salida inesperada:\n%s", out.String())
	}
}

func TestVerifyNullsFoundAndNoValuesPrinted(t *testing.T) {
	src := fakeNullSource{docs: map[string][]bson.D{
		"users": {
			{{Key: "_id", Value: "id-secreto-1"}, {Key: "email", Value: "persona@correo.example"}, {Key: "phone", Value: nil}},
			{{Key: "_id", Value: "id-secreto-2"}, {Key: "phone", Value: nil}},
		},
		"bookings": {
			{{Key: "_id", Value: "b1"}, {Key: "payments", Value: bson.A{bson.D{{Key: "paidAt", Value: nil}}}}},
		},
		"items": {{{Key: "_id", Value: "i1"}}},
	}}
	var out, errb bytes.Buffer
	code := verifyNulls(context.Background(), &out, &errb, "", src)
	if code != verifyNullsFound {
		t.Fatalf("código = %d, want 1\n%s", code, out.String())
	}
	s := out.String()
	for _, want := range []string{"users.phone: 2", "bookings.payments[].paidAt: 1", "OK     items", "3 valores null en total", "2 con null"} {
		if !strings.Contains(s, want) {
			t.Errorf("falta %q en:\n%s", want, s)
		}
	}
	for _, leak := range []string{"id-secreto", "persona@correo.example"} {
		if strings.Contains(s, leak) {
			t.Errorf("la salida filtra %q", leak)
		}
	}
}

func TestVerifyNullsErrors(t *testing.T) {
	const uri = "mongodb://usuario:clave-super-secreta@db.example:27017"
	cases := map[string]fakeNullSource{
		"listar": {listErr: errors.New("falló " + uri)},
		"leer":   {docs: map[string][]bson.D{"users": {{{Key: "_id", Value: 1}}}}, scanErr: errors.New("falló " + uri)},
	}
	for name, src := range cases {
		var out, errb bytes.Buffer
		if code := verifyNulls(context.Background(), &out, &errb, uri, src); code != verifyNullsFailure {
			t.Errorf("%s: código = %d, want 2", name, code)
		}
		if strings.Contains(errb.String(), "clave-super-secreta") {
			t.Errorf("%s: el error filtra la contraseña: %s", name, errb.String())
		}
	}
}

func TestRunVerifyNullsViaRun(t *testing.T) {
	var opened bool
	factory := func(context.Context, config.Config) (nullSource, func(), error) {
		opened = true
		return fakeNullSource{docs: map[string][]bson.D{"users": {{{Key: "phone", Value: nil}}}}}, func() {}, nil
	}
	var out bytes.Buffer
	code := run([]string{"verify-no-nulls"}, &out, &bytes.Buffer{}, loadOK, nil, nil, nil, nil, extraDeps{nulls: factory})
	if code != verifyNullsFound || !opened {
		t.Fatalf("código = %d (abierta=%v), want 1", code, opened)
	}

	failing := func(context.Context, config.Config) (nullSource, func(), error) {
		return nil, nil, errors.New("sin conexión")
	}
	if code := run([]string{"verify-no-nulls"}, &bytes.Buffer{}, &bytes.Buffer{}, loadOK, nil, nil, nil, nil, extraDeps{nulls: failing}); code != verifyNullsFailure {
		t.Fatalf("conexión fallida: código = %d, want 2", code)
	}
	badLoad := func() (config.Config, error) { return config.Config{}, errors.New("config") }
	if code := run([]string{"verify-no-nulls"}, &bytes.Buffer{}, &bytes.Buffer{}, badLoad, nil, nil, nil, nil); code != verifyNullsFailure {
		t.Fatalf("config inválida: código = %d, want 2", code)
	}
	if code := run([]string{"verify-no-nulls", "extra"}, &bytes.Buffer{}, &bytes.Buffer{}, loadOK, nil, nil, nil, nil); code != 2 {
		t.Fatalf("argumento extra: código = %d, want 2", code)
	}
}
