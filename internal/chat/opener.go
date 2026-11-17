// Package chat declara el puerto compartido chat.Opener (spec 007, T009; Art.
// XII.4): abre la conversación privada de una reserva. Hasta la spec 008 se
// usa Noop, que no abre nada; la 008 lo reemplaza sin cambiar a quien lo
// consume.
package chat

import "context"

// Opener abre la conversación de una reserva entre sus dos partes y devuelve
// su id ("" si no hay chat todavía).
type Opener interface {
	Open(ctx context.Context, bookingID, ownerID, requesterID string) (conversationID string, err error)
}

// Noop es la implementación vacía: no abre nada y nunca falla.
type Noop struct{}

// Open devuelve un id vacío.
func (Noop) Open(context.Context, string, string, string) (string, error) { return "", nil }
