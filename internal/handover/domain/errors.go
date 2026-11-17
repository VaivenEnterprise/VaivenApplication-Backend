package domain

import shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"

// Errores de dominio de entrega y devolución; el adaptador HTTP los traduce
// con el envoltorio del Art. IV.
var (
	// ErrForbidden: la persona es parte de la reserva pero la acción no le
	// toca en esta etapa (plan §4).
	ErrForbidden = shared.NewForbidden("No te corresponde esta acción en esta etapa.")
	// ErrWrongStatus: la reserva no está en el estado que pide la etapa.
	ErrWrongStatus = shared.NewConflict("La reserva no está en el estado correcto para esta etapa.")
	// ErrAlreadyConfirmed: la etapa ya se confirmó; la evidencia ya no cambia
	// (RF-13).
	ErrAlreadyConfirmed = shared.NewConflict("Esta etapa ya fue confirmada y no se puede modificar.")
	// ErrNoEvidence: falta registrar el estado y las fotos antes del QR.
	ErrNoEvidence = shared.NewConflict("Primero registra el estado del objeto y sus fotos.")
	// ErrQRInvalid: el token no corresponde al QR vigente de esta etapa
	// (otra etapa, otra reserva, reemplazado o inexistente).
	ErrQRInvalid = shared.NewConflict("El código QR no es válido. Pide uno nuevo.")
	// ErrQRExpired: el QR venció (POL-06); se puede generar otro.
	ErrQRExpired = shared.NewConflict("El código QR venció. Pide uno nuevo.")
	// ErrQRUsed: el QR ya se usó (un solo uso).
	ErrQRUsed = shared.NewConflict("Este código QR ya se usó.")
	// ErrOutsideWindow: la entrega solo se confirma desde un día antes del
	// inicio y hasta el último día de la reserva (plan P-04).
	ErrOutsideWindow = shared.NewConflict("La entrega solo se puede confirmar desde un día antes del inicio y hasta el último día de la reserva.")
)

const messageInvalidData = "Revisa los datos marcados."

// ErrConditionNotes indica que el estado del objeto debe tener de 3 a 500 caracteres.
var ErrConditionNotes = shared.NewValidation(messageInvalidData,
	shared.Detail{Field: "conditionNotes", Issue: "debe tener entre 3 y 500 caracteres"})

// ErrPhotoCount indica que la evidencia lleva de 1 a 5 fotos (POL-13).
var ErrPhotoCount = shared.NewValidation(messageInvalidData,
	shared.Detail{Field: "photos", Issue: "debe tener entre 1 y 5 fotos"})

// ErrTokenRequired indica que la confirmación exige el token del QR.
var ErrTokenRequired = shared.NewValidation(messageInvalidData,
	shared.Detail{Field: "token", Issue: "es obligatorio"})
