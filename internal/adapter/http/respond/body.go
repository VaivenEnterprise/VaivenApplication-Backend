package respond

import (
	"errors"
	"net/http"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Mensajes de un cuerpo de petición que no se pudo leer (spec 012, RF-04).
const (
	MessageBodyTooLarge = "El cuerpo de la petición es demasiado grande."
	MessageInvalidJSON  = "El cuerpo de la petición no es un JSON válido."
)

// InvalidBody convierte un error al decodificar el cuerpo en el 400 del
// contrato: «demasiado grande» si se pasó del límite (del módulo o el global
// de 1 MiB) y, si no, «no es un JSON válido». Ningún módulo responde 413
// (spec 012-T003, CA-05).
func InvalidBody(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return domain.NewValidation(MessageBodyTooLarge)
	}
	return domain.NewValidation(MessageInvalidJSON)
}
