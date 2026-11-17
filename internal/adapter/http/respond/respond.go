// Package respond escribe las respuestas JSON de la API y traduce los errores
// al envoltorio único del Art. IV (spec 002, plan §4.2, RF-08 y RF-10).
//
// Es el único punto que convierte un error en estado HTTP (plan P-05): los
// handlers y el middleware devuelven errores y llaman a Error.
package respond

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

const (
	contentType = "application/json; charset=utf-8"

	// codeInternal es el código de cualquier fallo que no es de dominio.
	codeInternal = "internal_error"

	// CodeServiceUnavailable (503) lo usan los handlers cuando una dependencia
	// no responde, por ejemplo MongoDB en /readyz.
	CodeServiceUnavailable = "service_unavailable"

	// messageInternal es el mensaje genérico de internal_error: nunca expone
	// la causa (Art. VI, RF-10).
	messageInternal = "Ocurrió un error inesperado. Intenta de nuevo."
)

// requestIDKey es la clave privada con la que se guarda el requestId en el
// contexto. Vive aquí porque respond lo lee y el middleware RequestID (T010,
// que depende de este paquete) lo escribe con WithRequestID.
type requestIDKey struct{}

// WithRequestID devuelve una copia de ctx que lleva el requestId de la petición.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID devuelve el requestId guardado en ctx, o "" si no hay ninguno.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// errorEnvelope es el envoltorio único de error: {"error": {...}}.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	Details   []detailBody `json:"details,omitempty"`
	RequestID string       `json:"requestId"`
}

type detailBody struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// statusByCode traduce los códigos de dominio a estados HTTP (Art. IV.3).
var statusByCode = map[domain.Code]int{
	domain.CodeValidation:   http.StatusBadRequest,
	domain.CodeUnauthorized: http.StatusUnauthorized,
	domain.CodeForbidden:    http.StatusForbidden,
	domain.CodeNotFound:     http.StatusNotFound,
	domain.CodeConflict:     http.StatusConflict,
	domain.CodeUnavailable:  http.StatusServiceUnavailable,
}

// JSON escribe v como JSON con el estado indicado. Si v es nil solo escribe
// el estado. Si v no se puede codificar responde 500 internal_error.
func JSON(w http.ResponseWriter, status int, v any) {
	if v == nil {
		w.WriteHeader(status)
		return
	}
	body, err := json.Marshal(v)
	if err != nil {
		write(w, http.StatusInternalServerError, errorEnvelope{Error: errorBody{
			Code:    codeInternal,
			Message: messageInternal,
		}})
		return
	}
	writeRaw(w, status, body)
}

// Error escribe err con el envoltorio único. Un *domain.Error (también si va
// envuelto con %w) usa su código, mensaje y detalles; cualquier otro error, o
// un código de dominio desconocido, produce 500 internal_error con un mensaje
// genérico. La causa interna nunca llega a la respuesta.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	body := errorBody{
		Code:      codeInternal,
		Message:   messageInternal,
		RequestID: RequestID(r.Context()),
	}
	status := http.StatusInternalServerError

	var de *domain.Error
	if errors.As(err, &de) {
		if s, ok := statusByCode[de.Code]; ok {
			status = s
			body.Code = string(de.Code)
			body.Message = de.Message
			for _, d := range de.Details {
				body.Details = append(body.Details, detailBody{Field: d.Field, Issue: d.Issue})
			}
		}
	}

	write(w, status, errorEnvelope{Error: body})
}

// Failure escribe un error HTTP que no es de dominio (503, 429, 404/405 del
// router…) con el mismo envoltorio único que Error. message debe estar en
// español y no incluir detalles internos.
func Failure(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	write(w, status, errorEnvelope{Error: errorBody{
		Code:      code,
		Message:   message,
		RequestID: RequestID(r.Context()),
	}})
}

// write codifica v y lo escribe; v siempre es un tipo propio de este paquete,
// por lo que la codificación no puede fallar.
func write(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, messageInternal, http.StatusInternalServerError)
		return
	}
	writeRaw(w, status, body)
}

func writeRaw(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
