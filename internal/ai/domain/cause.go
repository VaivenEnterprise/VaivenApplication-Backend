package domain

import (
	"errors"
	"fmt"
)

// Cause clasifica por qué falló una llamada al proveedor de IA (spec 014,
// T018). Es un valor cerrado y seguro para los registros: nunca lleva cuerpos,
// claves, URLs ni texto del modelo.
type Cause string

// Causas conocidas.
const (
	CauseTimeout       Cause = "timeout"        // se agotó el tiempo del contexto o del cliente
	CauseHTTPStatus    Cause = "http_status"    // el proveedor respondió un estado HTTP de error
	CauseEmptyResponse Cause = "empty_response" // 200 sin contenido o truncado (finish_reason=length)
	CauseMalformedJSON Cause = "malformed_json" // el contenido no es un JSON utilizable
	CauseContentFilter Cause = "content_filter" // el proveedor bloqueó el contenido
	CauseNetwork       Cause = "network"        // fallo de red distinto de un tiempo de espera
	CauseImageFetch    Cause = "image_fetch"    // no se pudo descargar la miniatura
)

// CauseError es un error del proveedor con su causa y, si aplica, el estado
// HTTP. Envuelve a ErrUnavailable o ErrBlocked, así que errors.Is sigue
// funcionando igual que antes.
type CauseError struct {
	Cause  Cause
	Status int // 0 si no aplica
	Base   error
}

// WithCause devuelve base anotado con la causa y el estado HTTP (0 si no hay).
func WithCause(base error, cause Cause, status int) error {
	return &CauseError{Cause: cause, Status: status, Base: base}
}

// Error no incluye nada del contenido: solo la causa y el estado.
func (e *CauseError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("%v (causa %s, HTTP %d)", e.Base, e.Cause, e.Status)
	}
	return fmt.Sprintf("%v (causa %s)", e.Base, e.Cause)
}

// Unwrap permite errors.Is(err, ErrUnavailable) y errors.Is(err, ErrBlocked).
func (e *CauseError) Unwrap() error { return e.Base }

// CauseOf extrae la causa y el estado HTTP de err, si los tiene.
func CauseOf(err error) (cause Cause, status int, ok bool) {
	var ce *CauseError
	if errors.As(err, &ce) {
		return ce.Cause, ce.Status, true
	}
	return "", 0, false
}
