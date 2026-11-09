package domain

import shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"

// MessageInvalidData es el mensaje general de validation_failed.
const MessageInvalidData = "Revisa los datos marcados."

// Errores de dominio de reservas; el adaptador HTTP los traduce con el
// envoltorio del Art. IV.
var (
	// ErrNotFound: la reserva no existe o quien consulta no es parte.
	ErrNotFound = shared.NewNotFound("No encontramos la solicitud.")
	// ErrItemNotFound: el objeto no existe o no está disponible para solicitar.
	ErrItemNotFound = shared.NewNotFound("No encontramos el objeto.")
	// ErrForbidden: la acción no le corresponde a quien la pide.
	ErrForbidden = shared.NewForbidden("No puedes hacer esta acción en esta solicitud.")
	// ErrOwnItem: no se solicita un objeto propio (CA-04).
	ErrOwnItem = shared.NewForbidden("No puedes solicitar tu propio objeto.")
	// ErrItemUnavailable: el objeto no está activo (pausado, bloqueado).
	ErrItemUnavailable = shared.NewConflict("Este objeto no está disponible por ahora.")
	// ErrPeriodTaken: el periodo se cruza con una reserva aceptada o en curso.
	ErrPeriodTaken = shared.NewConflict("El objeto ya está reservado en esas fechas.")
	// ErrDuplicatePending: ya hay una solicitud propia pendiente que se cruza.
	ErrDuplicatePending = shared.NewConflict("Ya tienes una solicitud pendiente para este objeto en esas fechas.")
	// ErrInvalidTransition: el estado actual no permite la acción (RF-11).
	ErrInvalidTransition = shared.NewConflict("La solicitud ya no admite esta acción.")
	// ErrVersionConflict: otra operación cambió la reserva a la vez.
	ErrVersionConflict = shared.NewConflict("La solicitud cambió mientras la procesabas. Vuelve a cargarla.")
)

// ErrQuoteChanged: el total cambió desde la cotización (RF-02, CA-03). Es
// 409 conflict con el detalle acceptedTotalCents: quote_changed.
var ErrQuoteChanged = &shared.Error{
	Code:    shared.CodeConflict,
	Message: "El precio cambió desde que lo cotizaste. Vuelve a cotizar.",
	Details: []shared.Detail{{Field: "acceptedTotalCents", Issue: "quote_changed"}},
}

// ErrReasonRequired: cancelar una reserva aceptada exige motivo (RF-08).
var ErrReasonRequired = shared.NewValidation(MessageInvalidData,
	shared.Detail{Field: "reason", Issue: "es obligatorio (3 a 200 caracteres) al cancelar una reserva aceptada"})
