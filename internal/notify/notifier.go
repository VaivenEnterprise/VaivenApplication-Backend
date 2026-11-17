// Package notify declara el puerto compartido de notificaciones (spec 003,
// RF-12; Art. XII.4). Hasta la spec 011 se usa Noop, que no envía nada; la 011
// lo reemplaza sin cambiar a quien lo consume.
package notify

import "context"

// Event describe una notificación para una persona. Type es un identificador
// estable en inglés (p. ej. "booking.requested"); Title y Body van en es-MX.
// Data lleva identificadores para que el móvil abra la pantalla correcta; nunca
// datos personales ni secretos (Art. VI.5).
type Event struct {
	Type  string
	Title string
	Body  string
	Data  map[string]string
}

// Notifier envía una notificación a userID. Un fallo al notificar no debe
// deshacer la operación de negocio: quien llama lo registra y sigue.
type Notifier interface {
	Notify(ctx context.Context, userID string, e Event) error
}

// Noop es la implementación vacía: no envía nada y nunca falla.
type Noop struct{}

// Notify no hace nada.
func (Noop) Notify(context.Context, string, Event) error { return nil }
