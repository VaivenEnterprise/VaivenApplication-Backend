package domain

import "time"

// Status es el estado de una solicitud o reserva (RF-11).
type Status string

// Estados.
const (
	StatusPending    Status = "pending"
	StatusAccepted   Status = "accepted"
	StatusRejected   Status = "rejected"
	StatusExpired    Status = "expired"
	StatusCancelled  Status = "cancelled"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

// Valid indica si el estado existe.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusAccepted, StatusRejected, StatusExpired, StatusCancelled, StatusInProgress, StatusCompleted:
		return true
	}
	return false
}

// Blocks indica si el estado ocupa el periodo (RF-03): aceptada o en curso.
func (s Status) Blocks() bool { return s == StatusAccepted || s == StatusInProgress }

// transitions es la tabla del plan §4: desde → hacia → papeles permitidos.
func transitions() map[Status]map[Status][]Role {
	return map[Status]map[Status][]Role{
		StatusPending: {
			StatusAccepted:  {RoleOwner},
			StatusRejected:  {RoleOwner},
			StatusCancelled: {RoleRequester},
			StatusExpired:   {RoleSystem},
		},
		StatusAccepted: {
			StatusCancelled:  {RoleOwner, RoleRequester},
			StatusInProgress: {RoleSystem}, // 009: confirmación de entrega
		},
		StatusInProgress: {
			StatusCompleted: {RoleSystem}, // 009: confirmación de devolución
		},
	}
}

// CanTransition indica si by puede pasar la reserva de from a to.
func CanTransition(from, to Status, by Role) bool {
	for _, r := range transitions()[from][to] {
		if r == by {
			return true
		}
	}
	return false
}

// ExistsTransition indica si from → to existe para algún papel (para
// distinguir 403 de 409).
func ExistsTransition(from, to Status) bool {
	_, ok := transitions()[from][to]
	return ok
}

// Transition cambia el estado si la tabla lo permite: un papel que existe en
// otra transición pero no en esta recibe ErrForbidden; un estado que no
// admite el cambio, ErrInvalidTransition. Agrega la entrada al historial
// (máximo MaxHistory, se descartan las más antiguas) y actualiza UpdatedAt.
// La versión la incrementa el repositorio al guardar.
func (b *Booking) Transition(to Status, by Role, at time.Time) error {
	from := b.EffectiveStatus(at)
	if from != b.Status && to != StatusExpired {
		// Una pendiente vencida ya no admite aceptar, rechazar ni cancelar.
		return ErrInvalidTransition
	}
	if !ExistsTransition(b.Status, to) {
		return ErrInvalidTransition
	}
	if !CanTransition(b.Status, to, by) {
		return ErrForbidden
	}
	b.Status = to
	b.UpdatedAt = at
	b.History = append(b.History, HistoryEntry{Status: to, At: at, By: by})
	if len(b.History) > MaxHistory {
		b.History = b.History[len(b.History)-MaxHistory:]
	}
	if to != StatusPending {
		b.ExpiresAt = nil
	}
	return nil
}
