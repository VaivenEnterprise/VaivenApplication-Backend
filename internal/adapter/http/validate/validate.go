// Package validate valida los cuerpos de entrada en la frontera HTTP con
// validator/v10 (Art. VI.2; spec 016, T005 y T006). Lo comparten los módulos
// para no repetir la misma lógica (Art. XII.6): cada DTO declara sus reglas
// con etiquetas `validate:"..."` y el módulo llama a Struct con su propio
// mensaje, así que el contrato no cambia: 400 validation_failed con un
// detalle por campo, en el orden de los campos del DTO y con el nombre JSON.
//
// Reglas propias, además de las de validator/v10:
//
//	nonempty     la cadena (o *string) no está vacía      → «es obligatorio»
//	nonnegative  el número (o puntero a número) es ≥ 0     → «no puede ser negativo»
package validate

import (
	"errors"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Issue es el texto (es-MX) de cada regla; las que no están aquí usan
// IssueInvalid.
var issues = map[string]string{
	"required":    "es obligatorio",
	"nonempty":    "es obligatorio",
	"nonnegative": "no puede ser negativo",
	"oneof":       "valor no permitido",
	"min":         "fuera del rango permitido",
	"max":         "fuera del rango permitido",
	"len":         "fuera del rango permitido",
	"gt":          "fuera del rango permitido",
	"gte":         "fuera del rango permitido",
	"lt":          "fuera del rango permitido",
	"lte":         "fuera del rango permitido",
	"hexadecimal": "formato inválido",
	"email":       "formato inválido",
	"url":         "formato inválido",
}

// IssueInvalid es el texto de una regla sin texto propio.
const IssueInvalid = "no es válido"

// Validator valida DTO; es seguro para uso concurrente.
type Validator struct {
	v *validator.Validate
}

// New crea un validador que nombra los campos por su etiqueta json.
func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			return ""
		}
		return name
	})
	// Los registros solo fallan con un nombre de etiqueta vacío o reservado.
	_ = v.RegisterValidation("nonempty", func(fl validator.FieldLevel) bool {
		return fl.Field().Kind() == reflect.String && strings.TrimSpace(fl.Field().String()) != ""
	})
	_ = v.RegisterValidation("nonnegative", func(fl validator.FieldLevel) bool {
		f := fl.Field()
		switch f.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return f.Int() >= 0
		case reflect.Float32, reflect.Float64:
			return f.Float() >= 0
		default:
			return true
		}
	})
	return &Validator{v: v}
}

// Struct valida dst. Devuelve nil si cumple y, si no, un error de dominio
// validation_failed con message y un detalle por campo. dst debe ser un struct
// o un puntero a struct.
func (x *Validator) Struct(dst any, message string) error {
	err := x.v.Struct(dst)
	if err == nil {
		return nil
	}
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return shared.NewValidation(message)
	}
	details := make([]shared.Detail, 0, len(verrs))
	for _, fe := range verrs {
		issue, ok := issues[fe.Tag()]
		if !ok {
			issue = IssueInvalid
		}
		details = append(details, shared.Detail{Field: fieldPath(fe.Namespace()), Issue: issue})
	}
	return shared.NewValidation(message, details...)
}

// fieldPath quita el tipo raíz: "createRequest.zone.municipality" →
// "zone.municipality"; "evidenceRequest.photos[0].publicId" →
// "photos[0].publicId".
func fieldPath(ns string) string {
	if _, rest, ok := strings.Cut(ns, "."); ok {
		return rest
	}
	return ns
}
