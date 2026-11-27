package mongo

import (
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
)

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

// UsersSchema es condicional por estado: active exige el conjunto completo y
// deactivated solo lo que conserva la baja (spec 021, T013).
// El esquema tolerante acepta null en los opcionales (despliegue A); el estricto
// (T030) no, y conserva el esquema condicional por estado.
func TestStrictAuthSchemasRejectNull(t *testing.T) {
	for _, c := range []struct {
		name   string
		schema bson.M
		nulls  []string
	}{
		{"users", UsersSchema(), []string{"phone", "suspendedUntil", "deactivatedAt"}},
		{"refresh_tokens", RefreshTokensSchema(), []string{"usedAt", "revokedAt"}},
	} {
		strict := adaptermongo.StrictSchema(c.schema)
		for _, f := range c.nulls {
			tol := c.schema["properties"].(bson.M)[f].(bson.M)["bsonType"].(bson.A)
			if !slices.Contains(tol, any("null")) {
				t.Errorf("%s.%s: el esquema tolerante debe aceptar null", c.name, f)
			}
			if got := strict["properties"].(bson.M)[f].(bson.M)["bsonType"]; got != "string" && got != "date" {
				t.Errorf("%s.%s: el estricto debe tener un solo tipo sin null, got %v", c.name, f, got)
			}
		}
	}
	if len(adaptermongo.StrictSchema(UsersSchema())["anyOf"].(bson.A)) != 2 {
		t.Error("el estricto de users debe conservar el esquema condicional por estado")
	}
	specs := ValidatorSpecs()
	if len(specs) != 2 || specs[0].Collection != UsersCollection || specs[1].Collection != RefreshTokensCollection || specs[0].Action != "error" {
		t.Errorf("ValidatorSpecs = %+v", specs)
	}
}

func TestUsersSchemaIsConditionalByStatus(t *testing.T) {
	s := UsersSchema()
	props := s["properties"].(bson.M)
	branches := s["anyOf"].(bson.A)
	if len(branches) != 2 {
		t.Fatalf("anyOf debe tener una rama por estado, tiene %d", len(branches))
	}
	want := map[string][]string{
		"active":      {"passwordHash", "role", "ratingSum", "ratingCount"},
		"deactivated": {"deactivatedAt"},
	}
	for _, b := range branches {
		br := b.(bson.M)
		status := br["properties"].(bson.M)["status"].(bson.M)["enum"].(bson.A)[0].(string)
		req := br["required"].(bson.A)
		if len(req) != len(want[status]) {
			t.Errorf("%s: required = %v, want %v", status, req, want[status])
		}
		for i, f := range req {
			if f.(string) != want[status][i] {
				t.Errorf("%s: required[%d] = %v, want %s", status, i, f, want[status][i])
			}
			if _, ok := props[f.(string)]; !ok {
				t.Errorf("%s: %q no está en properties", status, f)
			}
		}
	}
	// Lo que borra la baja nunca es obligatorio en todos los estados.
	for _, f := range s["required"].(bson.A) {
		switch f.(string) {
		case "passwordHash", "role", "ratingSum", "ratingCount", "phone", "suspendedUntil":
			t.Errorf("%q no puede ser obligatorio para todo estado", f)
		}
	}
	if dp := props["deletionPending"].(bson.M)["bsonType"]; dp != "bool" {
		t.Errorf("deletionPending = %v, want bool (sin null)", dp)
	}
}

// El índice parcial de deletionPending está declarado (y por tanto entra al
// catálogo de `admin indexes`).
func TestUserIndexesDeclareDeletionPending(t *testing.T) {
	var found bool
	for _, ix := range UserIndexes() {
		var o options.IndexOptions
		for _, set := range ix.Model.Options.List() {
			if err := set(&o); err != nil {
				t.Fatal(err)
			}
		}
		if o.Name != nil && *o.Name == "deletionPending_partial" {
			found = true
			if o.PartialFilterExpression == nil || o.Unique != nil {
				t.Errorf("deletionPending debe ser parcial y no único: %+v", o)
			}
		}
	}
	if !found {
		t.Error("UserIndexes debe declarar el índice parcial de deletionPending")
	}
}
