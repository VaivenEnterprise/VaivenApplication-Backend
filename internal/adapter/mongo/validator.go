package mongo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Códigos de error del servidor que EnsureValidator trata de forma especial.
const (
	codeUnauthorized    = 13 // falta el privilegio (p. ej. collMod sin dbAdmin)
	codeNamespaceExists = 48 // otra réplica creó la colección entre la consulta y la creación
)

// Nivel y acción de validación (spec 021, plan §5.2).
//
// moderate: MongoDB valida las inserciones y las actualizaciones de documentos
// que ya cumplen el esquema, pero no revalida los documentos existentes que no
// lo cumplen; así un documento heredado no bloquea a la aplicación.
//
// error rechaza la escritura inválida (código 121); warn solo la registra.
const (
	ValidationLevelModerate = "moderate"
	ValidationActionError   = "error"
	ValidationActionWarn    = "warn"
)

// OptionalFieldsAction es la acción con la que el ARRANQUE de la API aplica los
// validadores de campos opcionales de las colecciones que no tenían uno (spec
// 021, T008): "warn" (un documento con null entra y deja un aviso en el registro
// de MongoDB). El arranque nunca endurece por su cuenta: pasar a modo estricto
// (sin null, acción "error") es una decisión explícita que toma
// `admin enforce-validators --apply` DESPUÉS de migrar y de verificar cero null
// en el entorno de destino (T030). Una vez endurecido, el arranque de la API lo
// respeta y no lo afloja (EnsureValidatorKeepStrict).
const OptionalFieldsAction = ValidationActionWarn

// ValidatorOutcome dice qué hizo EnsureValidator con una colección.
type ValidatorOutcome string

// Resultados de EnsureValidator.
const (
	ValidatorCreated   ValidatorOutcome = "creada"
	ValidatorUpdated   ValidatorOutcome = "actualizada"
	ValidatorUnchanged ValidatorOutcome = "sin_cambios"
	// ValidatorSkipped: el validador guardado es otro y el usuario de la app no
	// tiene permiso para collMod (rol dbAdmin); se aplica a mano
	// (docs/servicios.md §9) y el arranque sigue.
	ValidatorSkipped ValidatorOutcome = "omitida_sin_permiso"
)

// EnsureValidator aplica schema como $jsonSchema de collection con el nivel y
// la acción indicados. Es idempotente: si la colección no existe la crea con el
// validador (el usuario de la app puede); si ya tiene el mismo validador, nivel
// y acción no hace nada (así no necesita dbAdmin); si no, usa collMod y, sin
// permiso (código 13), devuelve ValidatorSkipped sin error. Cualquier otro
// error se devuelve.
func EnsureValidator(ctx context.Context, db *drv.Database, collection string, schema bson.M, level, action string) (ValidatorOutcome, error) {
	validator := bson.M{"$jsonSchema": schema}
	specs, err := db.ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: collection}})
	if err != nil {
		return "", fmt.Errorf("%s: listar colecciones: %w", collection, err)
	}
	if len(specs) == 1 && sameValidation(specs[0].Options, validator, level, action) {
		return ValidatorUnchanged, nil
	}
	if len(specs) == 0 {
		err := db.CreateCollection(ctx, collection, options.CreateCollection().
			SetValidator(validator).SetValidationLevel(level).SetValidationAction(action))
		if err == nil {
			return ValidatorCreated, nil
		}
		if !hasErrorCode(err, codeNamespaceExists) {
			return "", fmt.Errorf("%s: crear colección con validador: %w", collection, err)
		}
		// Otra instancia la creó a la vez: se aplica el validador con collMod.
	}
	err = db.RunCommand(ctx, bson.D{
		{Key: "collMod", Value: collection},
		{Key: "validator", Value: validator},
		{Key: "validationLevel", Value: level},
		{Key: "validationAction", Value: action},
	}).Err()
	switch {
	case err == nil:
		return ValidatorUpdated, nil
	case hasErrorCode(err, codeUnauthorized):
		return ValidatorSkipped, nil
	default:
		return "", fmt.Errorf("%s: aplicar validador con collMod: %w", collection, err)
	}
}

// StrictSchema devuelve una copia de schema sin "null" en ningún bsonType: un
// campo opcional declarado como ["date", "null"] pasa a "date" (T030, RF-11).
// No modifica schema. Es la fuente única del modo estricto: los esquemas
// tolerantes (los que acepta el arranque de la API) y los estrictos salen del
// mismo texto, así no se desincronizan.
func StrictSchema(schema bson.M) bson.M {
	return strictValue(schema).(bson.M)
}

func strictValue(v any) any {
	switch x := v.(type) {
	case bson.M:
		out := make(bson.M, len(x))
		for k, val := range x {
			if k == "bsonType" {
				out[k] = stripNullType(val)
				continue
			}
			out[k] = strictValue(val)
		}
		return out
	case bson.A:
		out := make(bson.A, len(x))
		for i, val := range x {
			out[i] = strictValue(val)
		}
		return out
	default:
		return v
	}
}

// stripNullType quita "null" de un bsonType; si queda un solo tipo lo deja como
// texto.
func stripNullType(t any) any {
	list, ok := t.(bson.A)
	if !ok {
		return t
	}
	kept := make(bson.A, 0, len(list))
	for _, e := range list {
		if e != "null" {
			kept = append(kept, e)
		}
	}
	if len(kept) == 1 {
		return kept[0]
	}
	return kept
}

// EnsureValidatorKeepStrict aplica el validador TOLERANTE (schema con action)
// salvo que la colección ya tenga el estricto vigente (StrictSchema(schema),
// nivel level y acción "error"): en ese caso no hace nada y devuelve
// ValidatorUnchanged. Es lo que usa el arranque de la API, para que ni un
// despliegue ni un reinicio afloje un endurecimiento ya hecho (T030) y, al
// revés, para que ninguno endurezca por su cuenta sobre datos sin migrar.
func EnsureValidatorKeepStrict(ctx context.Context, db *drv.Database, collection string, schema bson.M, level, action string) (ValidatorOutcome, error) {
	specs, err := db.ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: collection}})
	if err != nil {
		return "", fmt.Errorf("%s: listar colecciones: %w", collection, err)
	}
	if len(specs) == 1 && sameValidation(specs[0].Options, bson.M{"$jsonSchema": StrictSchema(schema)}, level, ValidationActionError) {
		return ValidatorUnchanged, nil
	}
	return EnsureValidator(ctx, db, collection, schema, level, action)
}

// Strictness es el estado de endurecimiento de la validación de una colección.
type Strictness string

// Estados de ValidatorState.
const (
	StrictnessStrict   Strictness = "estricto"      // sin null, acción error
	StrictnessTolerant Strictness = "tolerante"     // el esquema tolerante con su acción
	StrictnessOther    Strictness = "distinto"      // otro validador (o uno anterior)
	StrictnessMissing  Strictness = "sin_validador" // la colección no existe o no tiene validador
)

// ValidatorSpec describe el validador de una colección en su forma TOLERANTE
// (la que aplica el arranque): Schema puede aceptar null y Action es la acción
// tolerante ("error" para users, refresh_tokens e items; "warn" para el resto).
// La forma estricta se deriva con StrictSchema y la acción "error".
type ValidatorSpec struct {
	Collection string
	Schema     bson.M
	Action     string
}

// OptionalFieldSpecs devuelve las especificaciones de OptionalFieldSchemas con
// la acción tolerante OptionalFieldsAction, en orden alfabético.
func OptionalFieldSpecs() []ValidatorSpec {
	schemas := OptionalFieldSchemas()
	names := make([]string, 0, len(schemas))
	for n := range schemas {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]ValidatorSpec, 0, len(names))
	for _, n := range names {
		out = append(out, ValidatorSpec{Collection: n, Schema: schemas[n], Action: OptionalFieldsAction})
	}
	return out
}

// ValidatorState dice si la colección de spec tiene el validador estricto, el
// tolerante, otro o ninguno. Solo lee.
func ValidatorState(ctx context.Context, db *drv.Database, spec ValidatorSpec) (Strictness, error) {
	specs, err := db.ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: spec.Collection}})
	if err != nil {
		return "", fmt.Errorf("%s: listar colecciones: %w", spec.Collection, err)
	}
	if len(specs) == 0 {
		return StrictnessMissing, nil
	}
	opts := specs[0].Options
	switch {
	case sameValidation(opts, bson.M{"$jsonSchema": StrictSchema(spec.Schema)}, ValidationLevelModerate, ValidationActionError):
		return StrictnessStrict, nil
	case sameValidation(opts, bson.M{"$jsonSchema": spec.Schema}, ValidationLevelModerate, spec.Action):
		return StrictnessTolerant, nil
	case len(opts) == 0 || opts.Lookup("validator").Type != bson.TypeEmbeddedDocument:
		return StrictnessMissing, nil
	default:
		return StrictnessOther, nil
	}
}

// EnforceValidators aplica a todas las colecciones de specs el validador
// ESTRICTO (strict = true: sin null, nivel moderate, acción error) o, para
// volver atrás, el tolerante (strict = false). Es la única vía para endurecer
// (T030): la llama `admin enforce-validators` tras verificar cero null. Sigue
// con las demás si una se omite por falta de permiso (collMod exige dbAdmin) y
// se detiene en cualquier otro error. Es idempotente.
func EnforceValidators(ctx context.Context, db *drv.Database, specs []ValidatorSpec, strict bool) (ValidatorsResult, error) {
	var res ValidatorsResult
	for _, sp := range specs {
		schema, action := sp.Schema, sp.Action
		if strict {
			schema, action = StrictSchema(sp.Schema), ValidationActionError
		}
		out, err := EnsureValidator(ctx, db, sp.Collection, schema, ValidationLevelModerate, action)
		if err != nil {
			return res, err
		}
		res.add(sp.Collection, out)
	}
	return res, nil
}

func (r *ValidatorsResult) add(name string, out ValidatorOutcome) {
	switch out {
	case ValidatorCreated:
		r.Created = append(r.Created, name)
	case ValidatorUpdated:
		r.Updated = append(r.Updated, name)
	case ValidatorUnchanged:
		r.Unchanged = append(r.Unchanged, name)
	case ValidatorSkipped:
		r.Skipped = append(r.Skipped, name)
	}
}

// ValidatorsResult resume EnsureOptionalFieldValidators: nombres de colección
// por resultado, en orden alfabético.
type ValidatorsResult struct {
	Created, Updated, Unchanged, Skipped []string
}

// EnsureOptionalFieldValidators aplica a cada colección de OptionalFieldSchemas
// su validador de campos opcionales con nivel moderate y la acción indicada
// (normalmente OptionalFieldsAction). No afloja una colección ya endurecida
// (EnsureValidatorKeepStrict). Sigue con las demás si una se omite por falta de
// permiso; se detiene en cualquier otro error.
func EnsureOptionalFieldValidators(ctx context.Context, db *drv.Database, action string) (ValidatorsResult, error) {
	var res ValidatorsResult
	for _, sp := range OptionalFieldSpecs() {
		out, err := EnsureValidatorKeepStrict(ctx, db, sp.Collection, sp.Schema, ValidationLevelModerate, action)
		if err != nil {
			return res, err
		}
		res.add(sp.Collection, out)
	}
	return res, nil
}

// OptionalFieldSchemas devuelve, por colección, un $jsonSchema que describe
// solo los campos OPCIONALES con su tipo real y sin "null" (RF-10, RF-11): un
// campo opcional sin valor no existe en el documento. No exige ningún campo ni
// cierra el documento (sin additionalProperties: false), para que una spec
// nueva pueda agregar campos sin migrar el validador antes de desplegar. Las
// colecciones de users, refresh_tokens e items tienen su propio esquema
// (módulos auth e items).
//
// Los nombres de colección se repiten aquí como texto porque este paquete no
// puede importar a los módulos (ellos lo importan a él); una prueba externa
// comprueba que coinciden con las constantes de cada módulo.
func OptionalFieldSchemas() map[string]bson.M {
	str := bson.M{"bsonType": "string"}
	date := bson.M{"bsonType": "date"}
	obj := bson.M{"bsonType": "object"}
	schema := func(title string, props bson.M) bson.M {
		s := bson.M{"bsonType": "object", "title": title}
		if len(props) > 0 {
			s["properties"] = props
		}
		return s
	}
	handover := bson.M{
		"bsonType": "object",
		"properties": bson.M{
			"recordedAt":    date,
			"recordedById":  str,
			"confirmedAt":   date,
			"confirmedById": str,
			"qr": bson.M{
				"bsonType":   "object",
				"properties": bson.M{"usedAt": date},
			},
		},
	}
	return map[string]bson.M{
		"bookings": schema("bookings", bson.M{
			"expiresAt":      date,
			"meetingPoint":   str,
			"cancelReason":   str,
			"cancelledBy":    str,
			"rejectReason":   str,
			"conversationId": str,
			"delivery":       handover,
			"return":         handover,
			"lateAt":         date,
			"affectedBy":     str,
			"itemSnapshot": bson.M{
				"bsonType":   "object",
				"properties": bson.M{"usageRules": str, "firstPhoto": obj},
			},
			"payments": bson.M{
				"bsonType": "array",
				"items": bson.M{
					"bsonType":   "object",
					"properties": bson.M{"reportedAt": date, "reportedNote": str, "confirmedAt": date},
				},
			},
		}),
		"conversations": schema("conversations", bson.M{"lastMessage": obj, "unread": obj}),
		"messages":      schema("messages", bson.M{"clientId": str}),
		"ratings":       schema("ratings", bson.M{"comment": str}),
		"incidents":     schema("incidents", bson.M{"finalResponse": obj, "closedAt": date, "purgeAt": date}),
		"reports":       schema("reports", bson.M{"details": str, "resolution": obj, "purgeAt": date}),
		"sanctions":     schema("sanctions", bson.M{"endsAt": date, "incidentId": str, "purgeAt": date}),
		"notifications": schema("notifications", bson.M{"readAt": date, "data": obj}),
		"push_tokens":   schema("push_tokens", nil),
		"ai_usage":      schema("ai_usage", nil),
		"ai_conversations": schema("ai_conversations", bson.M{
			"turns": bson.M{"bsonType": "array"},
		}),
		"reco_interactions":  schema("reco_interactions", nil),
		"reco_states":        schema("reco_states", bson.M{"builtAt": date}),
		"reco_neighbors":     schema("reco_neighbors", bson.M{"neighbors": bson.M{"bsonType": "array"}}),
		"reco_popular_lists": schema("reco_popular_lists", bson.M{"items": bson.M{"bsonType": "array"}}),
	}
}

// sameValidation indica si las opciones de una colección ya tienen el
// validador, el nivel y la acción esperados. Compara el contenido normalizado
// (el orden de las claves de un bson.M no es estable).
func sameValidation(opts bson.Raw, validator bson.M, level, action string) bool {
	var current struct {
		Validator        bson.M `bson:"validator"`
		ValidationLevel  string `bson:"validationLevel"`
		ValidationAction string `bson:"validationAction"`
	}
	if len(opts) == 0 || bson.Unmarshal(opts, &current) != nil {
		return false
	}
	if current.ValidationLevel != level || current.ValidationAction != action {
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

func hasErrorCode(err error, code int) bool {
	var se drv.ServerError
	return errors.As(err, &se) && se.HasErrorCode(code)
}
