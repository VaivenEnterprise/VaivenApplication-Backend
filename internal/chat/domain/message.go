// Package domain contiene las reglas del chat privado de cada reserva (spec
// 008, T002): validación del mensaje, conversaciones, quién puede escribir y
// la vista previa. Solo biblioteca estándar (Art. III).
package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// Message es un mensaje de texto plano.
type Message struct {
	ID             string
	ConversationID string
	SenderID       string
	// Seq es el orden del mensaje en su conversación (1, 2, 3…), asignado al
	// confirmar el envío; el cliente usa ID como cursor.
	Seq       int64
	Body      string
	ClientID  *string
	CreatedAt time.Time
}

// MaxClientIDLen acota el clientId de idempotencia.
const MaxClientIDLen = 64

// MessageInvalidData es el mensaje general de validation_failed.
const MessageInvalidData = "Revisa los datos marcados."

// NormalizeBody recorta el cuerpo y valida 1 a 1000 caracteres (runas), sin
// ser solo espacios (RF-04, POL-11, plan P-08). Siempre es texto plano.
func NormalizeBody(body string) (string, error) {
	t := strings.TrimSpace(body)
	n := utf8.RuneCountInString(t)
	if n == 0 {
		return "", shared.NewValidation(MessageInvalidData, shared.Detail{Field: "body", Issue: "no puede estar vacío"})
	}
	if n > policy.POL11MaxMessageChars {
		return "", shared.NewValidation(MessageInvalidData, shared.Detail{Field: "body", Issue: "debe tener como máximo 1000 caracteres"})
	}
	return t, nil
}

// NormalizeClientID valida el clientId opcional. Vacío = nil.
func NormalizeClientID(id *string) (*string, error) {
	if id == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*id)
	if t == "" {
		return nil, nil
	}
	if len(t) > MaxClientIDLen {
		return nil, shared.NewValidation(MessageInvalidData, shared.Detail{Field: "clientId", Issue: "debe tener como máximo 64 caracteres"})
	}
	return &t, nil
}

// PreviewLen es el largo de la vista previa del último mensaje (RF-02).
const PreviewLen = 80

// Preview recorta el cuerpo a 80 caracteres; si lo corta agrega «…».
func Preview(body string) string {
	if utf8.RuneCountInString(body) <= PreviewLen {
		return body
	}
	r := []rune(body)
	return string(r[:PreviewLen]) + "…"
}
