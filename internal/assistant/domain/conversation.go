package domain

import (
	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// ConversationTTLSeconds: una conversación se borra 24 h después de su último
// mensaje (spec 014, RF-13).
const ConversationTTLSeconds = 24 * 60 * 60

// ErrConversationNotFound: la conversación no existe, caducó o es de otra
// persona (CA-13). No se distingue el caso para no revelar nada.
var ErrConversationNotFound = shared.NewNotFound("No encontramos esa conversación. Empieza una nueva.")

// Conversation es una conversación con el asistente. Solo guarda los turnos
// (máximo MaxTurns): nunca la ubicación ni las coincidencias de la búsqueda
// (RF-13 y RF-18).
type Conversation struct {
	ID     string
	UserID string
	Turns  []aidomain.Turn
}
