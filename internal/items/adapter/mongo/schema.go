package mongo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SchemaOutcome dice qué hizo EnsureSchema con la colección items.
type SchemaOutcome string

// Resultados de EnsureSchema.
const (
	SchemaCreated   SchemaOutcome = "creada"
	SchemaUpdated   SchemaOutcome = "actualizada"
	SchemaUnchanged SchemaOutcome = "sin_cambios"
	// SchemaSkipped: el validador guardado es otro y el usuario de la app no
	// tiene permiso para collMod (rol dbAdmin); se aplica a mano
	// (docs/servicios.md §9) y el arranque sigue.
	SchemaSkipped SchemaOutcome = "omitida_sin_permiso"
)

const (
	codeUnauthorized    = 13
	codeNamespaceExists = 48
)

// ItemsSchema es el $jsonSchema mínimo de items (plan §3): campos
// obligatorios, tipos, enumeraciones y rangos de dinero (Art. V y 6.6). No
// usa additionalProperties: false para que la spec 006 y las siguientes puedan
// agregar campos sin migrar el validador antes de desplegar.
func ItemsSchema() bson.M {
	str := func(minLen int) bson.M { return bson.M{"bsonType": "string", "minLength": minLen} }
	cents := func(minimum, maximum int64) bson.M {
		return bson.M{"bsonType": bson.A{"int", "long"}, "minimum": minimum, "maximum": maximum}
	}
	return bson.M{
		"bsonType": "object",
		"title":    "items",
		"required": bson.A{
			"_id", "ownerId", "title", "description", "category", "condition", "modality",
			"declaredValueCents", "pricePerDayCents", "priceSource", "zone", "photos",
			"status", "createdAt", "updatedAt", "version",
		},
		"properties": bson.M{
			"_id":                bson.M{"bsonType": "objectId"},
			"ownerId":            str(1),
			"title":              str(1),
			"description":        bson.M{"bsonType": "string"},
			"category":           str(1),
			"condition":          bson.M{"enum": bson.A{"new", "like_new", "good", "fair", "worn"}},
			"modality":           bson.M{"enum": bson.A{"loan", "rent"}},
			"declaredValueCents": cents(5_000, 10_000_000),
			"pricePerDayCents":   cents(0, 10_000_000),
			"priceSource":        bson.M{"enum": bson.A{"manual", "suggested"}},
			"usageRules":         bson.M{"bsonType": bson.A{"string", "null"}},
			"zone": bson.M{
				"bsonType": "object",
				"required": bson.A{"municipality"},
				"properties": bson.M{
					"municipality": str(1),
					"neighborhood": bson.M{"bsonType": bson.A{"string", "null"}},
				},
			},
			"location": bson.M{
				"bsonType": "object",
				"required": bson.A{"type", "coordinates"},
				"properties": bson.M{
					"type":        bson.M{"enum": bson.A{"Point"}},
					"coordinates": bson.M{"bsonType": "array", "minItems": 2, "maxItems": 2},
				},
			},
			"photos": bson.M{
				"bsonType": "array", "minItems": 1, "maxItems": 5,
				"items": bson.M{
					"bsonType":   "object",
					"required":   bson.A{"publicId"},
					"properties": bson.M{"publicId": str(1)},
				},
			},
			"status":    bson.M{"enum": bson.A{"active", "paused", "blocked", "removed"}},
			"createdAt": bson.M{"bsonType": "date"},
			"updatedAt": bson.M{"bsonType": "date"},
			"version":   bson.M{"bsonType": bson.A{"int", "long"}, "minimum": 1},
		},
	}
}

// EnsureSchema aplica el validador a items (nivel moderate, acción error). Si
// la colección no existe la crea con él (el usuario de la app puede); si ya
// tiene el mismo validador no hace nada; si no, intenta collMod y, sin
// permiso, devuelve SchemaSkipped sin error.
func EnsureSchema(ctx context.Context, db *drv.Database) (SchemaOutcome, error) {
	validator := bson.M{"$jsonSchema": ItemsSchema()}
	specs, err := db.ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: ItemsCollection}})
	if err != nil {
		return "", fmt.Errorf("items: listar colecciones: %w", err)
	}
	if len(specs) == 1 && sameValidation(specs[0].Options, validator) {
		return SchemaUnchanged, nil
	}
	if len(specs) == 0 {
		err := db.CreateCollection(ctx, ItemsCollection, options.CreateCollection().
			SetValidator(validator).SetValidationLevel("moderate").SetValidationAction("error"))
		if err == nil {
			return SchemaCreated, nil
		}
		if !hasCode(err, codeNamespaceExists) {
			return "", fmt.Errorf("items: crear colección con validador: %w", err)
		}
	}
	err = db.RunCommand(ctx, bson.D{
		{Key: "collMod", Value: ItemsCollection},
		{Key: "validator", Value: validator},
		{Key: "validationLevel", Value: "moderate"},
		{Key: "validationAction", Value: "error"},
	}).Err()
	switch {
	case err == nil:
		return SchemaUpdated, nil
	case hasCode(err, codeUnauthorized):
		return SchemaSkipped, nil
	default:
		return "", fmt.Errorf("items: aplicar validador con collMod: %w", err)
	}
}

// sameValidation compara validador, nivel y acción guardados con los
// esperados, normalizando el orden de claves y el tipo de los números.
func sameValidation(opts bson.Raw, validator bson.M) bool {
	var cur struct {
		Validator        bson.M `bson:"validator"`
		ValidationLevel  string `bson:"validationLevel"`
		ValidationAction string `bson:"validationAction"`
	}
	if len(opts) == 0 || bson.Unmarshal(opts, &cur) != nil ||
		cur.ValidationLevel != "moderate" || cur.ValidationAction != "error" {
		return false
	}
	a, errA := canonicalJSON(cur.Validator)
	b, errB := canonicalJSON(validator)
	return errA == nil && errB == nil && a == b
}

func canonicalJSON(v any) (string, error) {
	b, err := bson.MarshalExtJSON(bson.M{"v": v}, false, false)
	if err != nil {
		return "", err
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return "", err
	}
	out, err := json.Marshal(generic)
	return string(out), err
}

func hasCode(err error, code int) bool {
	var se drv.ServerError
	return errors.As(err, &se) && se.HasErrorCode(code)
}
