package domain

import "time"

// LastMessage es el resumen del último mensaje de una conversación.
type LastMessage struct {
	Preview  string
	At       time.Time
	SenderID string
}

// Conversation es la conversación privada de una reserva, con exactamente
// dos participantes (RF-01).
type Conversation struct {
	ID           string
	BookingID    string
	Participants [2]string
	// Unread son los no leídos por persona; nunca se expone el de la otra
	// parte (plan P-04).
	Unread        map[string]int
	LastMessage   *LastMessage
	LastMessageAt time.Time
	CreatedAt     time.Time
}

// IsParticipant indica si userID participa.
func (c Conversation) IsParticipant(userID string) bool {
	return userID != "" && (c.Participants[0] == userID || c.Participants[1] == userID)
}

// Counterpart devuelve el otro participante.
func (c Conversation) Counterpart(userID string) string {
	if c.Participants[0] == userID {
		return c.Participants[1]
	}
	return c.Participants[0]
}

// UnreadFor devuelve los no leídos de userID.
func (c Conversation) UnreadFor(userID string) int { return c.Unread[userID] }

// openStatuses son los estados de reserva en que se puede escribir (RF-06).
func openStatuses() map[string]bool {
	return map[string]bool{"pending": true, "accepted": true, "in_progress": true}
}

// CanWrite indica si se puede escribir: la reserva está pending, accepted o
// in_progress (RF-06) y la contraparte sigue activa (RF-07).
func CanWrite(bookingStatus string, counterpartActive bool) bool {
	return counterpartActive && openStatuses()[bookingStatus]
}

// Cursor marca la posición de una página de GET /conversations (último
// mensaje descendente y, a igual fecha, id descendente).
type Cursor struct {
	At time.Time
	ID string
}
