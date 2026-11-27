package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
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

// ValidatorSpec es la especificación (forma tolerante) de items para
// `admin enforce-validators` (T030).
func ValidatorSpec() adaptermongo.ValidatorSpec {
	return adaptermongo.ValidatorSpec{Collection: ItemsCollection, Schema: ItemsSchema(), Action: adaptermongo.ValidationActionError}
}

// EnsureSchema aplica el validador a items (nivel moderate, acción error) con
// adaptermongo.EnsureValidatorKeepStrict. Si la colección no existe la crea con
// él (el usuario de la app puede); si ya tiene el mismo validador no hace nada;
// si no, intenta collMod y, sin permiso, devuelve SchemaSkipped sin error. Aplica
// el esquema TOLERANTE (acepta null en los opcionales) y no endurece por su
// cuenta: si items ya tiene el estricto (lo aplicó `admin enforce-validators`
// tras migrar y verificar cero null, T030) lo respeta y no lo afloja.
func EnsureSchema(ctx context.Context, db *drv.Database) (SchemaOutcome, error) {
	out, err := adaptermongo.EnsureValidatorKeepStrict(ctx, db, ItemsCollection, ItemsSchema(),
		adaptermongo.ValidationLevelModerate, adaptermongo.ValidationActionError)
	if err != nil {
		return "", err
	}
	switch out {
	case adaptermongo.ValidatorCreated:
		return SchemaCreated, nil
	case adaptermongo.ValidatorUpdated:
		return SchemaUpdated, nil
	case adaptermongo.ValidatorSkipped:
		return SchemaSkipped, nil
	default:
		return SchemaUnchanged, nil
	}
}
