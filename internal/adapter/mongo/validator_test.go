package mongo

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
)

func TestHasErrorCode(t *testing.T) {
	cases := map[string]struct {
		err  error
		code int
		want bool
	}{
		"CommandError 13":          {drv.CommandError{Code: 13, Name: "Unauthorized"}, codeUnauthorized, true},
		"CommandError 13 envuelto": {fmt.Errorf("collMod: %w", drv.CommandError{Code: 13}), codeUnauthorized, true},
		"CommandError 121":         {drv.CommandError{Code: 121}, codeUnauthorized, false},
		"CommandError 48":          {drv.CommandError{Code: 48}, codeNamespaceExists, true},
		"error genérico":           {errors.New("unauthorized"), codeUnauthorized, false},
		"nil":                      {nil, codeUnauthorized, false},
	}
	for name, c := range cases {
		if got := hasErrorCode(c.err, c.code); got != c.want {
			t.Errorf("%s: hasErrorCode = %v, want %v", name, got, c.want)
		}
	}
}

func TestSameValidation(t *testing.T) {
	validator := bson.M{"$jsonSchema": bson.M{"bsonType": "object", "title": "x"}}
	opts := func(v bson.M, level, action string) bson.Raw {
		raw, err := bson.Marshal(bson.M{"validator": v, "validationLevel": level, "validationAction": action})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	if !sameValidation(opts(validator, "moderate", "error"), validator, "moderate", "error") {
		t.Error("el mismo validador debe considerarse igual")
	}
	if !sameValidation(opts(validator, "moderate", "warn"), validator, "moderate", "warn") {
		t.Error("el mismo validador con acción warn debe considerarse igual")
	}
	for name, raw := range map[string]bson.Raw{
		"otro esquema": opts(bson.M{"$jsonSchema": bson.M{"bsonType": "array"}}, "moderate", "error"),
		"otro nivel":   opts(validator, "strict", "error"),
		"otra acción":  opts(validator, "moderate", "warn"),
		"sin opciones": nil,
		"basura":       {0x01},
	} {
		if sameValidation(raw, validator, "moderate", "error") {
			t.Errorf("%s: no debe considerarse igual", name)
		}
	}
}

// Los esquemas de campos opcionales no aceptan null (RF-11), no exigen ningún
// campo y no cierran el documento.
func TestOptionalFieldSchemas(t *testing.T) {
	schemas := OptionalFieldSchemas()
	if len(schemas) != 15 {
		t.Errorf("colecciones con validador = %d, want 15", len(schemas))
	}
	for name, s := range schemas {
		if s["title"] != name || s["bsonType"] != "object" {
			t.Errorf("%s: title/bsonType = %v/%v", name, s["title"], s["bsonType"])
		}
		if _, ok := s["required"]; ok {
			t.Errorf("%s: un esquema de campos opcionales no exige campos", name)
		}
		if _, ok := s["additionalProperties"]; ok {
			t.Errorf("%s: no debe fijar additionalProperties", name)
		}
		js, err := canonicalJSON(s)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(js, `"null"`) {
			t.Errorf("%s: el esquema no debe aceptar null: %s", name, js)
		}
	}
}

// El arranque de la API aplica "warn": jamás endurece por su cuenta (T030). El
// modo estricto solo lo aplica `admin enforce-validators` tras verificar cero null.
func TestOptionalFieldsActionNeverHardensOnStartup(t *testing.T) {
	if OptionalFieldsAction != ValidationActionWarn {
		t.Errorf("OptionalFieldsAction = %q; el arranque debe aplicar warn", OptionalFieldsAction)
	}
	for _, sp := range OptionalFieldSpecs() {
		if sp.Action != ValidationActionWarn {
			t.Errorf("%s: acción tolerante = %q, want warn", sp.Collection, sp.Action)
		}
	}
}

// StrictSchema quita "null" de cualquier bsonType, a cualquier nivel (también en
// arreglos y en anyOf), sin tocar el original ni el resto del esquema.
func TestStrictSchemaRemovesNullEverywhere(t *testing.T) {
	tolerant := bson.M{
		"bsonType": "object",
		"properties": bson.M{
			"a": bson.M{"bsonType": bson.A{"string", "null"}},
			"b": bson.M{"bsonType": bson.A{"date", "null"}},
			"c": bson.M{"bsonType": bson.A{"int", "long", "null"}},
			"d": bson.M{"bsonType": "object", "properties": bson.M{"e": bson.M{"bsonType": bson.A{"string", "null"}}}},
			"f": bson.M{"bsonType": "array", "items": bson.M{"bsonType": bson.A{"object", "null"}}},
		},
		"anyOf":    bson.A{bson.M{"properties": bson.M{"g": bson.M{"bsonType": bson.A{"bool", "null"}}}}},
		"required": bson.A{"a"},
	}
	before, _ := canonicalJSON(tolerant)
	strict := StrictSchema(tolerant)
	if after, _ := canonicalJSON(tolerant); after != before {
		t.Fatal("StrictSchema modificó el esquema original")
	}
	js, err := canonicalJSON(strict)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(js, `"null"`) {
		t.Errorf("el estricto aún acepta null: %s", js)
	}
	props := strict["properties"].(bson.M)
	if props["a"].(bson.M)["bsonType"] != "string" || props["b"].(bson.M)["bsonType"] != "date" {
		t.Errorf("un solo tipo debe quedar como texto: %v", props)
	}
	if got := props["c"].(bson.M)["bsonType"].(bson.A); len(got) != 2 || got[0] != "int" || got[1] != "long" {
		t.Errorf("varios tipos se conservan menos null: %v", got)
	}
	if strict["required"].(bson.A)[0] != "a" || strict["anyOf"] == nil {
		t.Errorf("el resto del esquema debe conservarse: %v", strict)
	}
}

// Las colecciones de campos opcionales ya no aceptan null: su forma estricta es
// la misma y solo cambia la acción (de warn a error).
func TestOptionalFieldSpecsStrictOnlyChangesAction(t *testing.T) {
	for _, sp := range OptionalFieldSpecs() {
		a, _ := canonicalJSON(sp.Schema)
		b, _ := canonicalJSON(StrictSchema(sp.Schema))
		if a != b {
			t.Errorf("%s: el esquema de campos opcionales debe coincidir con su forma estricta", sp.Collection)
		}
	}
}
