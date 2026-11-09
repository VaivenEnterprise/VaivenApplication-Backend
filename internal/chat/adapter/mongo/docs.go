// Package mongo implementa los repositorios del chat con MongoDB (spec 008,
// T003; plan §3). Lee bookings y users solo en modo lectura.
package mongo

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
)

// Colecciones.
const (
	ConversationsCollection = "conversations"
	MessagesCollection      = "messages"
	BookingsCollection      = "bookings"
	UsersCollection         = "users"
)

type lastMessageDoc struct {
	Preview  string    `bson:"preview"`
	At       time.Time `bson:"at"`
	SenderID string    `bson:"senderId"`
}

// conversationDoc es el documento de conversations (plan §3). unread es un
// mapa userId → no leídos.
type conversationDoc struct {
	ID             bson.ObjectID   `bson:"_id"`
	BookingID      string          `bson:"bookingId"`
	ParticipantIDs []string        `bson:"participantIds"`
	Unread         map[string]int  `bson:"unread"`
	LastMessage    *lastMessageDoc `bson:"lastMessage"`
	LastMessageAt  time.Time       `bson:"lastMessageAt"`
	CreatedAt      time.Time       `bson:"createdAt"`
}

func (d conversationDoc) toDomain() domain.Conversation {
	c := domain.Conversation{
		ID: d.ID.Hex(), BookingID: d.BookingID, Unread: d.Unread,
		LastMessageAt: d.LastMessageAt.UTC(), CreatedAt: d.CreatedAt.UTC(),
	}
	if c.Unread == nil {
		c.Unread = map[string]int{}
	}
	if len(d.ParticipantIDs) == 2 {
		c.Participants = [2]string{d.ParticipantIDs[0], d.ParticipantIDs[1]}
	}
	if lm := d.LastMessage; lm != nil {
		c.LastMessage = &domain.LastMessage{Preview: lm.Preview, At: lm.At.UTC(), SenderID: lm.SenderID}
	}
	return c
}

// messageDoc es el documento de messages. El _id (ObjectID) crece con el
// tiempo y sirve de cursor (plan P-01).
type messageDoc struct {
	ID             bson.ObjectID `bson:"_id"`
	ConversationID string        `bson:"conversationId"`
	SenderID       string        `bson:"senderId"`
	Body           string        `bson:"body"`
	ClientID       *string       `bson:"clientId,omitempty"`
	CreatedAt      time.Time     `bson:"createdAt"`
}

func (d messageDoc) toDomain() domain.Message {
	return domain.Message{
		ID: d.ID.Hex(), ConversationID: d.ConversationID, SenderID: d.SenderID,
		Body: d.Body, ClientID: d.ClientID, CreatedAt: d.CreatedAt.UTC(),
	}
}
