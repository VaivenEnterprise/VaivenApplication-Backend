package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// reportRequest es ReportPaymentRequest; note es opcional. Pasa por el
// validador de la frontera (Art. VI.2; spec 016, T005); la longitud de la nota
// se mide en el dominio tras normalizar el texto (bookings.NormalizeText).
type reportRequest struct {
	Note *string `json:"note" validate:"omitempty"`
}

// decode lee un único objeto JSON. Un cuerpo vacío es válido (la nota es
// opcional) y deja dst sin cambios.
func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(dst)
	switch {
	case errors.Is(err, io.EOF):
		return nil
	case err != nil:
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return shared.NewValidation("El cuerpo de la petición es demasiado grande.")
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return shared.NewValidation(bookings.MessageInvalidData, shared.Detail{Field: typeErr.Field, Issue: "tipo de dato inválido"})
		}
		return shared.NewValidation("El cuerpo de la petición no es un JSON válido.")
	case dec.More():
		return shared.NewValidation("El cuerpo de la petición debe ser un solo objeto JSON.")
	}
	return nil
}
