// Package domain contiene las reglas de los avisos y de los tokens de push
// (spec 011, T003/T004): forma de los datos, validación del token de Expo y
// de las ids. Solo biblioteca estándar y los dominios compartidos (Art. III).
package domain

import (
	"strings"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// MessageInvalidData es el mensaje general de validation_failed.
const MessageInvalidData = "Revisa los datos marcados."

// Límites (plan §2, RF-03, RF-04).
const (
	MaxTokenLen   = 200
	MaxReadIDs    = 100
	RetentionDays = 90 // RNF-01: los avisos se conservan 90 días
)

// Plataformas de push admitidas.
const (
	PlatformAndroid = "android"
	PlatformIOS     = "ios"
)

// Notification es un aviso dentro de la app (RF-01). ReadAt es nil mientras no
// se lea. Data lleva solo ids (Art. VI.5).
type Notification struct {
	ID        string
	UserID    string
	Type      string
	Title     string
	Body      string
	Data      map[string]string
	ReadAt    *time.Time
	CreatedAt time.Time
}

// PushToken es el token de Expo de un dispositivo de una persona (RF-04).
type PushToken struct {
	Token      string
	UserID     string
	Platform   string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// ValidatePlatform exige android o ios.
func ValidatePlatform(p string) error {
	if p == PlatformAndroid || p == PlatformIOS {
		return nil
	}
	return shared.NewValidation(MessageInvalidData, shared.Detail{Field: "platform", Issue: "debe ser android o ios"})
}

// NormalizeToken recorta el token y exige el formato de Expo
// (ExponentPushToken[...] o ExpoPushToken[...]) de hasta 200 caracteres.
func NormalizeToken(token string) (string, error) {
	t := strings.TrimSpace(token)
	if !validToken(t) {
		return "", shared.NewValidation(MessageInvalidData, shared.Detail{Field: "token", Issue: "no es un token de Expo válido"})
	}
	return t, nil
}

// IsExpoToken indica si t tiene la forma de un token de Expo.
func IsExpoToken(t string) bool { return validToken(t) }

func validToken(t string) bool {
	if t == "" || len(t) > MaxTokenLen {
		return false
	}
	for _, prefix := range []string{"ExponentPushToken[", "ExpoPushToken["} {
		if strings.HasPrefix(t, prefix) && strings.HasSuffix(t, "]") && len(t) > len(prefix)+1 {
			inner := t[len(prefix) : len(t)-1]
			return !strings.ContainsAny(inner, "[] \t\r\n")
		}
	}
	return false
}

// IsObjectID indica si s es un id de 24 caracteres hexadecimales en minúscula.
func IsObjectID(s string) bool {
	if len(s) != 24 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
