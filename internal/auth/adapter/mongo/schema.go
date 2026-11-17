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

// Códigos de error del servidor que EnsureSchemas trata de forma especial.
const (
	codeUnauthorized    = 13 // falta el privilegio (p. ej. collMod sin dbAdmin)
	codeNamespaceExists = 48 // otra réplica creó la colección entre la consulta y la creación
)

// Nivel y acción de validación que usan todas las colecciones del módulo.
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
	schemaValidationLevel  = "moderate"
	schemaValidationAction = "error"
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
// userDoc: los int64 se guardan como long (se admite también int), los
// punteros nil como null y time.Time como date.
func UsersSchema() bson.M {
	return bson.M{
		"bsonType": "object",
		"title":    "users",
		"required": bson.A{
			"_id", "email", "passwordHash", "displayName", "role", "status",
			"acceptedTermsAt", "ratingSum", "ratingCount", "createdAt", "updatedAt", "version",
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

// EnsureSchemas aplica el validador $jsonSchema a users y refresh_tokens. Es
// idempotente: si la colección no existe la crea con el validador; si existe,
// lo reemplaza con collMod.
//
// El usuario de la aplicación en staging tiene el rol readWrite, que permite
// createCollection y listCollections pero no collMod (rol dbAdmin). Si collMod
// falla por permisos (código 13, Unauthorized), la colección se anota en
// SchemaResult.Skipped y no se devuelve error, para no impedir el arranque;
// cualquier otro error sí se devuelve.
func EnsureSchemas(ctx context.Context, db *drv.Database) (SchemaResult, error) {
	var res SchemaResult
	for _, c := range collectionSchemas() {
		validator := bson.M{"$jsonSchema": c.schema}

		specs, err := db.ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: c.name}})
		if err != nil {
			return res, fmt.Errorf("%s: listar colecciones: %w", c.name, err)
		}
		if len(specs) == 1 && sameValidation(specs[0].Options, validator) {
			// Ya tiene este validador (p. ej. lo aplicó un dbAdmin): no hace
			// falta collMod, que el usuario de la app no puede ejecutar.
			res.Unchanged = append(res.Unchanged, c.name)
			continue
		}
		if len(specs) == 0 {
			err := db.CreateCollection(ctx, c.name, options.CreateCollection().
				SetValidator(validator).
				SetValidationLevel(schemaValidationLevel).
				SetValidationAction(schemaValidationAction))
			if err == nil {
				res.Created = append(res.Created, c.name)
				continue
			}
			if !hasErrorCode(err, codeNamespaceExists) {
				return res, fmt.Errorf("%s: crear colección con validador: %w", c.name, err)
			}
			// Otra instancia la creó a la vez: se aplica el validador con collMod.
		}

		err = db.RunCommand(ctx, bson.D{
			{Key: "collMod", Value: c.name},
			{Key: "validator", Value: validator},
			{Key: "validationLevel", Value: schemaValidationLevel},
			{Key: "validationAction", Value: schemaValidationAction},
		}).Err()
		switch {
		case err == nil:
			res.Updated = append(res.Updated, c.name)
		case isUnauthorized(err):
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: sin permiso para collMod (requiere dbAdmin): %v", c.name, err))
		default:
			return res, fmt.Errorf("%s: aplicar validador con collMod: %w", c.name, err)
		}
	}
	return res, nil
}

// sameValidation indica si las opciones de una colección ya tienen el
// validador, el nivel y la acción esperados. Compara el contenido normalizado
// (el orden de las claves de un bson.M no es estable).
func sameValidation(opts bson.Raw, validator bson.M) bool {
	var current struct {
		Validator        bson.M `bson:"validator"`
		ValidationLevel  string `bson:"validationLevel"`
		ValidationAction string `bson:"validationAction"`
	}
	if len(opts) == 0 || bson.Unmarshal(opts, &current) != nil {
		return false
	}
	if current.ValidationLevel != schemaValidationLevel || current.ValidationAction != schemaValidationAction {
		return false
	}
	a, errA := canonicalJSON(current.Validator)
	b, errB := canonicalJSON(validator)
	return errA == nil && errB == nil && a == b
}

// canonicalJSON serializa v (bson.M anidados) a JSON con las claves ordenadas
// y los números como float64, para comparar contenido sin depender del orden
// ni del tipo entero (int32 del servidor frente a int del código).
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

// isUnauthorized indica si err es un error del servidor por falta de
// privilegios (código 13).
func isUnauthorized(err error) bool {
	return hasErrorCode(err, codeUnauthorized)
}

func hasErrorCode(err error, code int) bool {
	var se drv.ServerError
	return errors.As(err, &se) && se.HasErrorCode(code)
}
