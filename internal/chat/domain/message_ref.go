package domain

// MessageRef es lo mínimo de un mensaje que necesita la moderación (spec 011,
// RF-08): quién lo escribió, quiénes participan en su conversación y el texto
// para mostrar un resumen al revisor. Es de solo lectura.
type MessageRef struct {
	ID           string
	SenderID     string
	Participants [2]string
	Body         string
}

// HasParticipant indica si userID participa en la conversación del mensaje.
func (m MessageRef) HasParticipant(userID string) bool {
	return userID != "" && (m.Participants[0] == userID || m.Participants[1] == userID)
}
