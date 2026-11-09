// Package usecase contiene los casos de uso del chat (spec 008, T004–T005):
// abrir la conversación de una reserva, listar, leer y enviar mensajes,
// marcar como leída y contar no leídos. Depende solo de puertos (Art. III).
package usecase

import (
	"context"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Conversations guarda las conversaciones (adapter/mongo.ConversationRepo).
type Conversations interface {
	Open(ctx context.Context, bookingID, ownerID, requesterID string, now time.Time) (domain.Conversation, error)
	ByID(ctx context.Context, id string) (domain.Conversation, error)
	ListByParticipant(ctx context.Context, userID string, after *domain.Cursor, limit int) ([]domain.Conversation, error)
	// RecordMessage devuelve los no leídos que tenía receiverID antes.
	RecordMessage(ctx context.Context, conversationID, receiverID string, lm domain.LastMessage) (int, error)
	MarkRead(ctx context.Context, conversationID, userID string) error
	UnreadTotal(ctx context.Context, userID string) (int, error)
}

// Messages guarda los mensajes (adapter/mongo.MessageRepo).
type Messages interface {
	Insert(ctx context.Context, m domain.Message) (domain.Message, error)
	ByClientID(ctx context.Context, conversationID, senderID, clientID string) (domain.Message, bool, error)
	Before(ctx context.Context, conversationID, before string, limit int) ([]domain.Message, error)
	After(ctx context.Context, conversationID, after string, limit int) ([]domain.Message, error)
}

// Bookings lee de bookings el estado, las partes y el objeto (plan P-03).
type Bookings interface {
	ByIDs(ctx context.Context, ids []string) (map[string]domain.BookingInfo, error)
}

// Accounts indica qué cuentas siguen activas (RF-07).
type Accounts interface {
	Active(ctx context.Context, ids []string) (map[string]bool, error)
}

// ConversationView es una conversación lista para responder a quien
// participa. Unread es solo el propio (plan P-04).
type ConversationView struct {
	Conversation domain.Conversation
	Booking      domain.BookingInfo
	Counterpart  users.PublicUser
	ThumbURL     *string
	CanWrite     bool
	Unread       int
}

// Page es una página de conversaciones; Next es nil al final.
type Page struct {
	Items []ConversationView
	Next  *domain.Cursor
}

// MessageQuery son los parámetros de GET /conversations/{id}/messages.
type MessageQuery struct {
	Before, After string
	Limit         int
}

// MessagePage es una página de mensajes; Next es el id para seguir hacia
// atrás (nil al final o en consulta con After).
type MessagePage struct {
	Items []domain.Message
	Next  *string
}

// Límites de las páginas (plan §2).
const (
	DefaultConversations = 20
	MaxConversations     = 50
	DefaultMessages      = 30
	MaxMessages          = 50
	MaxAfter             = 100
	// EventMessageReceived es el tipo de aviso del primer no leído (RF-12).
	EventMessageReceived = "message_received"
	// DeletedName es el nombre que se muestra si la contraparte ya no existe.
	DeletedName = "Cuenta eliminada"
)
