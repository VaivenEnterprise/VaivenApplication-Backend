package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
)

// Nivel y acción de validación que usan todas las colecciones del módulo
// (adaptermongo.EnsureValidator, spec 021, T008).
//
// validationLevel "moderate": MongoDB valida todas las inserciones y las
// actualizaciones de documentos que ya cumplen el esquema, pero NO revalida
// las actualizaciones de documentos existentes que no lo cumplen. Así un
// documento heredado (anterior al validador) sigue pudiéndose actualizar y no
// bloquea a la aplicación, mientras que todo documento nuevo o ya válido
// queda protegido.
//
// validationAction "error": el servidor rechaza la escritura inválida (código
// 121, DocumentValidationFailure) en lugar de solo registrarla.
const (
	schemaValidationLevel  = adaptermongo.ValidationLevelModerate
	schemaValidationAction = adaptermongo.ValidationActionError
)

// SchemaResult resume lo que hizo EnsureSchemas.
type SchemaResult struct {
	// Created son las colecciones creadas con su validador.
	Created []string
	// Updated son las colecciones existentes a las que se aplicó el
	// validador con collMod.
	Updated []string
	// Unchanged son las colecciones que ya tenían el validador esperado.
	Unchanged []string
	// Skipped son las colecciones existentes cuyo validador no se pudo
	// aplicar por falta de permisos (collMod exige el rol dbAdmin); cada
	// elemento es "<colección>: <motivo>". El llamador lo registra como
	// advertencia y el arranque continúa.
	Skipped []string
}

// Las funciones de esquema son deliberadamente permisivas: la aplicación ya
// valida reglas de negocio (formato de correo, longitudes, etc.); el
// $jsonSchema protege la integridad estructural (campos obligatorios, tipos y
// enumeraciones) ante escrituras de otros procesos o errores de programación
// (Art. 6.6 de la constitución).
//
// No se usa additionalProperties: false a propósito: specs futuras pueden
// añadir campos a estos documentos y un esquema cerrado obligaría a migrar el
// validador antes de desplegar cada cambio, con riesgo de rechazar escrituras
// legítimas durante el despliegue.

// UsersSchema devuelve el $jsonSchema de la colección users. Coincide con
// userDoc: los int64 se guardan como long (se admite también int) y time.Time
// como date.
//
// Es condicional por estado (spec 021, plan §5.2; MongoDB no tiene if/then, así
// que usa anyOf): una cuenta active exige el conjunto completo; una cuenta
// deactivated solo el mínimo que conserva la baja (§5.2 de la spec: sin
// passwordHash, role ni ratingSum/ratingCount) y puede llevar deletionPending
// (booleano, ausente cuando no hay pasos pendientes). Es la forma TOLERANTE
// (acepta null en los opcionales); la estricta (T030) sale de
// adaptermongo.StrictSchema(UsersSchema()).
func UsersSchema() bson.M {
	return bson.M{
		"bsonType": "object",
		"title":    "users",
		"required": bson.A{
			"_id", "email", "displayName", "status", "acceptedTermsAt", "createdAt", "updatedAt", "version",
		},
		"properties": bson.M{
			"_id":             bson.M{"bsonType": "objectId"},
			"email":           bson.M{"bsonType": "string", "minLength": 3},
			"passwordHash":    bson.M{"bsonType": "string", "minLength": 1},
			"displayName":     bson.M{"bsonType": "string", "minLength": 1},
			"phone":           bson.M{"bsonType": bson.A{"string", "null"}},
			"role":            bson.M{"bsonType": "string", "enum": bson.A{"user", "admin"}},
			"status":          bson.M{"bsonType": "string", "enum": bson.A{"active", "deactivated"}},
			"acceptedTermsAt": bson.M{"bsonType": "date"},
			"ratingSum":       bson.M{"bsonType": bson.A{"int", "long"}, "minimum": 0},
			"ratingCount":     bson.M{"bsonType": bson.A{"int", "long"}, "minimum": 0},
			"suspendedUntil":  bson.M{"bsonType": bson.A{"date", "null"}},
			"createdAt":       bson.M{"bsonType": "date"},
			"updatedAt":       bson.M{"bsonType": "date"},
			"version":         bson.M{"bsonType": bson.A{"int", "long"}, "minimum": 0},
			"deactivatedAt":   bson.M{"bsonType": bson.A{"date", "null"}},
			"deletionPending": bson.M{"bsonType": "bool"},
		},
		"anyOf": bson.A{
			bson.M{
				"properties": bson.M{"status": bson.M{"enum": bson.A{"active"}}},
				"required":   bson.A{"passwordHash", "role", "ratingSum", "ratingCount"},
			},
			bson.M{
				"properties": bson.M{"status": bson.M{"enum": bson.A{"deactivated"}}},
				"required":   bson.A{"deactivatedAt"},
			},
		},
	}
}

// RefreshTokensSchema devuelve el $jsonSchema de la colección refresh_tokens
// (coincide con refreshDoc).
func RefreshTokensSchema() bson.M {
	return bson.M{
		"bsonType": "object",
		"title":    "refresh_tokens",
		"required": bson.A{"_id", "userId", "familyId", "tokenHash", "expiresAt", "createdAt"},
		"properties": bson.M{
			"_id":       bson.M{"bsonType": "objectId"},
			"userId":    bson.M{"bsonType": "string", "minLength": 1},
			"familyId":  bson.M{"bsonType": "string", "minLength": 1},
			"tokenHash": bson.M{"bsonType": "string", "minLength": 1},
			"expiresAt": bson.M{"bsonType": "date"},
			"usedAt":    bson.M{"bsonType": bson.A{"date", "null"}},
			"revokedAt": bson.M{"bsonType": bson.A{"date", "null"}},
			"createdAt": bson.M{"bsonType": "date"},
		},
	}
}

// ValidatorSpecs son las especificaciones (forma tolerante) de las colecciones
// del módulo para `admin enforce-validators` (T030).
func ValidatorSpecs() []adaptermongo.ValidatorSpec {
	out := []adaptermongo.ValidatorSpec{}
	for _, c := range collectionSchemas() {
		out = append(out, adaptermongo.ValidatorSpec{Collection: c.name, Schema: c.schema, Action: schemaValidationAction})
	}
	return out
}

// collectionSchemas lista, en orden, las colecciones del módulo y su esquema.
func collectionSchemas() []struct {
	name   string
	schema bson.M
} {
	return []struct {
		name   string
		schema bson.M
	}{
		{UsersCollection, UsersSchema()},
		{RefreshTokensCollection, RefreshTokensSchema()},
	}
}

// EnsureSchemas aplica el validador $jsonSchema a users y refresh_tokens con
// adaptermongo.EnsureValidator. Es idempotente: si la colección no existe la
// crea con el validador; si existe, lo reemplaza con collMod.
//
// El usuario de la aplicación en staging tiene el rol readWrite, que permite
// createCollection y listCollections pero no collMod (rol dbAdmin). Si collMod
// falla por permisos (código 13, Unauthorized), la colección se anota en
// SchemaResult.Skipped y no se devuelve error, para no impedir el arranque;
// cualquier otro error sí se devuelve.
//
// Aplica los esquemas TOLERANTES (aceptan null en los opcionales) y no endurece
// por su cuenta: si la colección ya tiene el estricto (lo aplicó
// `admin enforce-validators` tras migrar y verificar cero null, T030) lo respeta
// y no lo afloja (adaptermongo.EnsureValidatorKeepStrict).
func EnsureSchemas(ctx context.Context, db *drv.Database) (SchemaResult, error) {
	var res SchemaResult
	for _, c := range collectionSchemas() {
		out, err := adaptermongo.EnsureValidatorKeepStrict(ctx, db, c.name, c.schema, schemaValidationLevel, schemaValidationAction)
		if err != nil {
			return res, err
		}
		switch out {
		case adaptermongo.ValidatorCreated:
			res.Created = append(res.Created, c.name)
		case adaptermongo.ValidatorUpdated:
			res.Updated = append(res.Updated, c.name)
		case adaptermongo.ValidatorUnchanged:
			res.Unchanged = append(res.Unchanged, c.name)
		case adaptermongo.ValidatorSkipped:
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: sin permiso para collMod (requiere dbAdmin)", c.name))
		}
	}
	return res, nil
}
