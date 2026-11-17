package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

const secretURI = "mongodb://usuario:clave-super-secreta@db.example:27017/?replicaSet=rs0"

type fakeSource struct {
	idx map[string][]bson.D
	err error
}

func (f fakeSource) ListIndexes(context.Context) (map[string][]bson.D, error) { return f.idx, f.err }

var userSpec = []adaptermongo.IndexSpec{{Collection: "users", Model: drv.IndexModel{
	Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetName("email_unique").SetUnique(true),
}}}

func usersIndex(name string) bson.D {
	return bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: bson.D{{Key: "email", Value: int32(1)}}},
		{Key: "name", Value: name}, {Key: "unique", Value: true}}
}

var idIdx = bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "name", Value: "_id_"}}

func TestCheckIndexesAllOK(t *testing.T) {
	var out bytes.Buffer
	src := fakeSource{idx: map[string][]bson.D{"users": {idIdx, usersIndex("email_unique")}}}
	if code := checkIndexes(context.Background(), &out, &bytes.Buffer{}, secretURI, userSpec, src); code != indexesOK {
		t.Fatalf("código = %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "OK") || !strings.Contains(out.String(), "1 correctos") {
		t.Fatalf("salida inesperada:\n%s", out.String())
	}
}

func TestCheckIndexesDiscrepancies(t *testing.T) {
	cases := map[string]map[string][]bson.D{
		"faltante":     {"users": {idIdx}},
		"sobrante":     {"users": {idIdx, usersIndex("email_unique"), usersIndex("otro")}},
		"incompatible": {"users": {idIdx, {{Key: "key", Value: bson.D{{Key: "email", Value: int32(-1)}}}, {Key: "name", Value: "email_unique"}, {Key: "unique", Value: true}}}},
	}
	for name, idx := range cases {
		var out bytes.Buffer
		if code := checkIndexes(context.Background(), &out, &bytes.Buffer{}, "", userSpec, fakeSource{idx: idx}); code != indexesDiscrepancy {
			t.Errorf("%s: código = %d, want 1\n%s", name, code, out.String())
		}
	}
}

func TestCheckIndexesRenamedIsStillADiscrepancy(t *testing.T) {
	var out bytes.Buffer
	src := fakeSource{idx: map[string][]bson.D{"users": {idIdx, usersIndex("email_1")}}}
	if code := checkIndexes(context.Background(), &out, &bytes.Buffer{}, "", userSpec, src); code != indexesDiscrepancy {
		t.Fatalf("código = %d, want 1", code)
	}
	s := out.String()
	for _, want := range []string{"FALTANTE", "SOBRANTE", "NOMBRE DISTINTO", "sigue siendo una discrepancia"} {
		if !strings.Contains(s, want) {
			t.Errorf("falta %q en:\n%s", want, s)
		}
	}
}

func TestCheckIndexesErrorsExitTwoWithoutURI(t *testing.T) {
	var errb bytes.Buffer
	src := fakeSource{err: errors.New("server selection error: " + secretURI)}
	if code := checkIndexes(context.Background(), &bytes.Buffer{}, &errb, secretURI, userSpec, src); code != indexesError {
		t.Fatalf("código = %d, want 2", code)
	}
	if strings.Contains(errb.String(), "clave-super-secreta") || strings.Contains(errb.String(), secretURI) {
		t.Fatalf("el error muestra la URI o la contraseña: %s", errb.String())
	}
	// Declaración inválida → 2.
	bad := []adaptermongo.IndexSpec{{Collection: "c", Model: drv.IndexModel{Keys: bson.M{"a": 1}}}}
	if code := checkIndexes(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "", bad, fakeSource{}); code != indexesError {
		t.Fatalf("declaración inválida: código = %d, want 2", code)
	}
}

func TestRedactRemovesCredentials(t *testing.T) {
	cases := []string{
		"falló " + secretURI,
		"falló mongodb://otro:pw@h:1/x",
		"falló mongodb+srv://u:p@cluster.example/db",
	}
	for _, c := range cases {
		got := redact(c, secretURI)
		if strings.Contains(got, "clave-super-secreta") || strings.Contains(got, ":pw@") || strings.Contains(got, "u:p@") {
			t.Errorf("redact(%q) = %q", c, got)
		}
	}
	if got := redact("sin uri", ""); got != "sin uri" {
		t.Errorf("redact sin URI = %q", got)
	}
}

// La conexión se cierra antes de que runIndexes devuelva el código (main hace
// os.Exit después, cuando ya corrieron los defer).
func TestRunIndexesClosesConnectionBeforeReturning(t *testing.T) {
	closed := false
	open := func(context.Context, config.Config) (indexSource, func(), error) {
		return fakeSource{idx: map[string][]bson.D{"users": {idIdx}}}, func() { closed = true }, nil
	}
	code := runIndexes(&bytes.Buffer{}, &bytes.Buffer{}, config.Config{MongoURI: secretURI}, open, userSpec)
	if code != indexesDiscrepancy {
		t.Fatalf("código = %d, want 1", code)
	}
	if !closed {
		t.Fatal("la conexión no se cerró antes de devolver el código")
	}
}

func TestRunIndexesConnectionError(t *testing.T) {
	var errb bytes.Buffer
	open := func(context.Context, config.Config) (indexSource, func(), error) {
		return nil, nil, errors.New("conectar a MongoDB: " + secretURI + ": context deadline exceeded")
	}
	if code := runIndexes(&bytes.Buffer{}, &errb, config.Config{MongoURI: secretURI}, open, userSpec); code != indexesError {
		t.Fatalf("código = %d, want 2", code)
	}
	if strings.Contains(errb.String(), "clave-super-secreta") || !strings.Contains(errb.String(), "deadline") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestRunIndexesSubcommand(t *testing.T) {
	never := func(config.Config) []check { t.Fatal("no debe revisar servicios"); return nil }
	badLoad := func() (config.Config, error) { return config.Config{}, errors.New("falta MONGO_URI") }
	if code := run([]string{"indexes"}, &bytes.Buffer{}, &bytes.Buffer{}, badLoad, never, nil, nil, nil); code != indexesError {
		t.Fatalf("config inválida: código = %d, want 2", code)
	}
	var out bytes.Buffer
	open := func(context.Context, config.Config) (indexSource, func(), error) {
		return fakeSource{idx: map[string][]bson.D{}}, func() {}, nil
	}
	code := run([]string{"indexes"}, &out, &bytes.Buffer{}, loadOK, never, nil, nil, open)
	// Base vacía: todos los índices del catálogo faltan.
	if code != indexesDiscrepancy || !strings.Contains(out.String(), "FALTANTE") {
		t.Fatalf("código = %d, salida:\n%s", code, out.String())
	}
}
