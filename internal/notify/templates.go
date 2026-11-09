package notify

import "strings"

// Tipos de aviso (spec 011, RF-02). Son identificadores estables en inglés con
// guion bajo; las specs anteriores emiten algunos con punto ("booking.requested")
// y Normalize los reconoce.
const (
	TypeBookingRequested  = "booking_requested"
	TypeBookingAccepted   = "booking_accepted"
	TypeBookingRejected   = "booking_rejected"
	TypeBookingCancelled  = "booking_cancelled"
	TypeBookingExpired    = "booking_expired"
	TypeMessageReceived   = "message_received"
	TypeDeliveryConfirmed = "delivery_confirmed"
	TypeReturnConfirmed   = "return_confirmed"
	TypePaymentReported   = "payment_reported"
	TypePaymentConfirmed  = "payment_confirmed"
	TypeLateReturn        = "late_return"
	TypeBookingAffected   = "booking_affected"
	TypeIncidentOpened    = "incident_opened"
	TypeIncidentNote      = "incident_note"
	TypeIncidentClosed    = "incident_closed"
	TypeRatingReceived    = "rating_received"
	TypeSanctionApplied   = "sanction_applied"
	TypeContentRemoved    = "content_removed"
)

// Template es el texto en es-MX de un tipo de aviso. No lleva parámetros: el
// aviso y el push solo exponen texto genérico y los ids van en Event.Data
// (Art. VI.5, plan P-04).
type Template struct {
	Title string
	Body  string
}

// templates tiene exactamente un texto por cada tipo del RF-02.
var templates = map[string]Template{
	TypeBookingRequested:  {"Nueva solicitud", "Alguien quiere uno de tus objetos."},
	TypeBookingAccepted:   {"Solicitud aceptada", "Aceptaron tu solicitud."},
	TypeBookingRejected:   {"Solicitud rechazada", "Tu solicitud no pudo ser aceptada."},
	TypeBookingCancelled:  {"Reserva cancelada", "Se canceló una de tus reservas."},
	TypeBookingExpired:    {"Solicitud vencida", "Tu solicitud venció sin respuesta."},
	TypeMessageReceived:   {"Nuevo mensaje", "Tienes un mensaje nuevo."},
	TypeDeliveryConfirmed: {"Entrega confirmada", "La entrega del objeto quedó confirmada."},
	TypeReturnConfirmed:   {"Devolución confirmada", "La devolución del objeto quedó confirmada."},
	TypePaymentReported:   {"Pago reportado", "Reportaron el pago de una reserva."},
	TypePaymentConfirmed:  {"Pago confirmado", "Confirmaron el pago de tu reserva."},
	TypeLateReturn:        {"Devolución tardía", "Un objeto no se devolvió a tiempo."},
	TypeBookingAffected:   {"Tu reserva se retrasa", "Una devolución tardía afecta una de tus reservas."},
	TypeIncidentOpened:    {"Incidencia abierta", "Abrieron una incidencia sobre una de tus reservas."},
	TypeIncidentNote:      {"Novedad en una incidencia", "Hay una nota nueva en una incidencia."},
	TypeIncidentClosed:    {"Incidencia cerrada", "El equipo cerró una incidencia con una respuesta final."},
	TypeRatingReceived:    {"Nueva calificación", "Recibiste una calificación."},
	TypeSanctionApplied:   {"Aviso de tu cuenta", "El equipo de VAIVÉN registró una medida en tu cuenta."},
	TypeContentRemoved:    {"Contenido retirado", "Retiramos una publicación tuya por incumplir las reglas."},
}

// aliases traduce los nombres que algunas specs anteriores usaban a los del
// RF-02.
var aliases = map[string]string{
	"booking_late": TypeLateReturn,
}

// Types devuelve los 18 tipos de aviso.
func Types() []string {
	out := make([]string, 0, len(templates))
	for t := range templates {
		out = append(out, t)
	}
	return out
}

// Normalize devuelve el tipo canónico de typ (sin importar si llegó con punto
// o con guion bajo) y si es uno de los 18 del RF-02.
func Normalize(typ string) (string, bool) {
	t := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(typ)), ".", "_")
	if a, ok := aliases[t]; ok {
		t = a
	}
	_, ok := templates[t]
	return t, ok
}

// TemplateFor devuelve el texto del tipo canónico typ.
func TemplateFor(typ string) (Template, bool) {
	t, ok := templates[typ]
	return t, ok
}
