// Package domain contiene las entidades y reglas de negocio de VAIVÉN.
// Solo importa la biblioteca estándar (Art. III).
package domain

// Code identifica el tipo de error de dominio. Sus valores coinciden con los
// códigos del contrato de la API (Art. IV.3); la capa HTTP los traduce a estados.
type Code string

// Códigos de error de dominio. rate_limited, internal_error y
// service_unavailable no son de dominio: los produce la capa HTTP.
const (
	CodeValidation   Code = "validation_failed"
	CodeNotFound     Code = "not_found"
	CodeConflict     Code = "conflict"
	CodeUnauthorized Code = "unauthorized"
	CodeForbidden    Code = "forbidden"
)

// Detail describe un problema concreto de un campo.
type Detail struct {
	Field string
	Issue string
}

// Error es un error de dominio con un mensaje apto para mostrar (es-MX).
type Error struct {
	Code    Code
	Message string
	Details []Detail
	Err     error // causa interna; nunca se muestra al usuario
}

func (e *Error) Error() string {
	return string(e.Code) + ": " + e.Message
}

// Unwrap devuelve la causa interna para errors.Is y errors.As.
func (e *Error) Unwrap() error {
	return e.Err
}

// Wrap devuelve una copia del error con la causa interna adjunta.
func (e *Error) Wrap(cause error) *Error {
	c := *e
	c.Err = cause
	return &c
}

// NewValidation crea un error de datos inválidos, con detalles por campo.
func NewValidation(message string, details ...Detail) *Error {
	return &Error{Code: CodeValidation, Message: message, Details: details}
}

// NewNotFound crea un error de recurso inexistente.
func NewNotFound(message string) *Error {
	return &Error{Code: CodeNotFound, Message: message}
}

// NewConflict crea un error de conflicto con el estado actual (p. ej. duplicado).
func NewConflict(message string) *Error {
	return &Error{Code: CodeConflict, Message: message}
}

// NewUnauthorized crea un error de identidad ausente o inválida.
func NewUnauthorized(message string) *Error {
	return &Error{Code: CodeUnauthorized, Message: message}
}

// NewForbidden crea un error de permiso insuficiente.
func NewForbidden(message string) *Error {
	return &Error{Code: CodeForbidden, Message: message}
}
