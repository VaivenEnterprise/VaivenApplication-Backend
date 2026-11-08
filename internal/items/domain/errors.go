package domain

import shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"

// Errores de dominio de objetos; el adaptador HTTP los traduce con el
// envoltorio del Art. IV.
var (
	// ErrNotFound: el objeto no existe, está eliminado o quien consulta no
	// puede verlo (RF-06).
	ErrNotFound = shared.NewNotFound("No encontramos el objeto.")
	// ErrNotOwner: solo el dueño puede editar, pausar o eliminar (CA-05).
	ErrNotOwner = shared.NewForbidden("Solo quien publicó el objeto puede hacer esto.")
	// ErrVersionConflict: la versión enviada no es la guardada (plan P-05).
	ErrVersionConflict = shared.NewConflict("El objeto cambió mientras lo editabas. Vuelve a cargarlo.")
	// ErrBlocked: un objeto bloqueado por moderación no se edita, pausa ni
	// reanuda; solo moderación lo desbloquea (spec 011).
	ErrBlocked = shared.NewConflict("Este objeto está bloqueado por moderación y no se puede modificar.")
	// ErrHasActiveBookings: no se elimina con reservas aceptadas o en curso
	// (RF-09, CA-07).
	ErrHasActiveBookings = shared.NewConflict("No puedes eliminar el objeto mientras tenga reservas aceptadas o en curso.")
)

// MessageInvalidData es el mensaje general de validation_failed.
const MessageInvalidData = "Revisa los datos marcados."
