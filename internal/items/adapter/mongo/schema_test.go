package mongo

import (
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
)

// El esquema tolerante de items acepta null en los opcionales (despliegue A); el
// estricto (T030) no.
func TestStrictItemsSchemaRejectsNull(t *testing.T) {
	tol := ItemsSchema()["properties"].(bson.M)
	if !slices.Contains(tol["usageRules"].(bson.M)["bsonType"].(bson.A), any("null")) {
		t.Fatal("el esquema tolerante debe aceptar null en usageRules")
	}
	strict := adaptermongo.StrictSchema(ItemsSchema())["properties"].(bson.M)
	if strict["usageRules"].(bson.M)["bsonType"] != "string" {
		t.Errorf("usageRules estricto = %v", strict["usageRules"])
	}
	zone := strict["zone"].(bson.M)["properties"].(bson.M)
	if zone["neighborhood"].(bson.M)["bsonType"] != "string" || zone["municipality"].(bson.M)["bsonType"] != "string" {
		t.Errorf("zone estricto = %v", zone)
	}
	if spec := ValidatorSpec(); spec.Collection != ItemsCollection || spec.Action != adaptermongo.ValidationActionError {
		t.Errorf("ValidatorSpec = %+v", spec)
	}
}
