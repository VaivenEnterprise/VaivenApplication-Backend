package domain

import shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"

// Errores de dominio del chat.
var (
	// ErrNotFound: la conversación no existe o quien pide no participa
	// (RF-05, CA-02).
	ErrNotFound = shared.NewNotFound("No encontramos la conversación.")
	// ErrClosed: la reserva ya no admite mensajes o la contraparte se dio de
	// baja (RF-06, RF-07).
	ErrClosed = shared.NewConflict("La conversación está cerrada.")
	// ErrDuplicateClientID: el clientId ya se usó (el repositorio lo
	// traduce en el mensaje existente).
	ErrDuplicateClientID = shared.NewConflict("Ya enviaste este mensaje.")
	// ErrUnknownMessage: el id de before/after no es un mensaje de esta
	// conversación (el usecase lo devuelve como validation_failed).
	ErrUnknownMessage = shared.NewValidation(MessageInvalidData, shared.Detail{Field: "cursor", Issue: "no es un mensaje de esta conversación"})
)
