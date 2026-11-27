package domain

import "time"

// AccountDeletedReason es el motivo con el que se cierra una solicitud
// pendiente cuando una de las partes da de baja su cuenta (spec 021, RF-01).
const AccountDeletedReason = "cuenta eliminada"

// Notice es el aviso que recibe la contraparte de una solicitud cerrada por la
// baja de la otra cuenta. Los textos son genéricos y nunca llevan datos de la
// cuenta eliminada; Data lleva solo identificadores (Art. VI.5).
type Notice struct {
	UserID string
	Type   string
	Title  string
	Body   string
	Data   map[string]string
}

// CloseOnAccountDeletion cierra la solicitud pendiente b porque deletedID, una
// de sus partes, dio de baja su cuenta (spec 021, §5.3), con las transiciones
// que ya permite la máquina de estados:
//
//   - si deletedID es quien la envió: pending → cancelled (papel requester) y
//     CancelReason / CancelledBy;
//   - si es la dueña del objeto: pending → rejected (papel owner) y RejectReason.
//
// Devuelve el aviso para la contraparte. Una solicitud que no está pendiente, o
// que ya venció aunque la tarea periódica aún no la haya marcado, devuelve
// ErrInvalidTransition (la máquina de estados no admite el cambio) y no se toca;
// una persona que no es parte, ErrForbidden.
func CloseOnAccountDeletion(b *Booking, deletedID string, at time.Time) (Notice, error) {
	role, ok := b.RoleOf(deletedID)
	if !ok {
		return Notice{}, ErrForbidden
	}
	if b.Status != StatusPending {
		// Una aceptada también se podría cancelar, pero esas son las que
		// impiden la baja (409): aquí solo se cierran las pendientes.
		return Notice{}, ErrInvalidTransition
	}
	reason := AccountDeletedReason
	data := map[string]string{"bookingId": b.ID, "itemId": b.ItemID}
	switch role {
	case RoleRequester:
		if err := b.Transition(StatusCancelled, RoleRequester, at); err != nil {
			return Notice{}, err
		}
		by := RoleRequester
		b.CancelReason, b.CancelledBy = &reason, &by
		return Notice{
			UserID: b.OwnerID, Type: "booking.cancelled", Title: "Solicitud cancelada",
			Body: "Se canceló la solicitud de " + b.Item.Title + " porque la cuenta de quien la envió fue eliminada.",
			Data: data,
		}, nil
	default: // RoleOwner
		if err := b.Transition(StatusRejected, RoleOwner, at); err != nil {
			return Notice{}, err
		}
		b.RejectReason = &reason
		return Notice{
			UserID: b.RequesterID, Type: "booking.rejected", Title: "Solicitud rechazada",
			Body: "Tu solicitud de " + b.Item.Title + " ya no está disponible porque la cuenta de quien publicó el objeto fue eliminada.",
			Data: data,
		}, nil
	}
}
