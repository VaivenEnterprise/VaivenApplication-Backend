package mongo

import (
	"errors"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
)

func TestIsUnauthorized(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"CommandError 13":          {drv.CommandError{Code: 13, Name: "Unauthorized"}, true},
		"CommandError 13 envuelto": {fmt.Errorf("collMod: %w", drv.CommandError{Code: 13}), true},
		"CommandError 121":         {drv.CommandError{Code: 121}, false},
		"error genérico":           {errors.New("unauthorized"), false},
		"nil":                      {nil, false},
	}
	for name, c := range cases {
		if got := isUnauthorized(c.err); got != c.want {
			t.Errorf("%s: isUnauthorized = %v, want %v", name, got, c.want)
		}
	}
	if !hasErrorCode(drv.CommandError{Code: 48}, codeNamespaceExists) {
		t.Error("hasErrorCode(48) = false")
	}
}

// Los campos obligatorios del esquema deben existir en properties.
func TestSchemasRequiredFieldsHaveProperties(t *testing.T) {
	for name, s := range map[string]bson.M{"users": UsersSchema(), "refresh_tokens": RefreshTokensSchema()} {
		props, ok := s["properties"].(bson.M)
		if !ok {
			t.Fatalf("%s: properties no es bson.M", name)
		}
		for _, f := range s["required"].(bson.A) {
			if _, ok := props[f.(string)]; !ok {
				t.Errorf("%s: el campo obligatorio %q no está en properties", name, f)
			}
		}
		if _, ok := s["additionalProperties"]; ok {
			t.Errorf("%s: no debe fijar additionalProperties", name)
		}
	}
	if len(collectionSchemas()) != 2 {
		t.Error("collectionSchemas debe cubrir users y refresh_tokens")
	}
}

func TestSameValidation(t *testing.T) {
	validator := bson.M{"$jsonSchema": UsersSchema()}
	opts := func(v bson.M, level, action string) bson.Raw {
		raw, err := bson.Marshal(bson.M{"validator": v, "validationLevel": level, "validationAction": action})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	if !sameValidation(opts(validator, "moderate", "error"), validator) {
		t.Error("el mismo validador debe considerarse igual")
	}
	for name, raw := range map[string]bson.Raw{
		"otro esquema": opts(bson.M{"$jsonSchema": bson.M{"bsonType": "object"}}, "moderate", "error"),
		"otro nivel":   opts(validator, "strict", "error"),
		"otra acción":  opts(validator, "moderate", "warn"),
		"sin opciones": nil,
		"basura":       {0x01},
	} {
		if sameValidation(raw, validator) {
			t.Errorf("%s: no debe considerarse igual", name)
		}
	}
}
