package mongo

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
)

// Sender guarda un mensaje y actualiza su conversación en una sola
// transacción (ajuste A-01 del plan 008, revisión T009). Cumple usecase.Sender.
type Sender struct {
	client *drv.Client
	convs  *drv.Collection
	msgs   *drv.Collection
}

// NewSender crea el escritor sobre db (replica set: usa transacciones).
func NewSender(db *drv.Database) *Sender {
	return &Sender{client: db.Client(), convs: db.Collection(ConversationsCollection), msgs: db.Collection(MessagesCollection)}
}

// Send guarda el mensaje, dentro de una transacción:
//
//  1. Incrementa conversations.seq y los no leídos de receiverID y fija el
//     último mensaje. Esa escritura bloquea la conversación hasta el commit:
//     un segundo envío choca (WriteConflict) y el driver lo reintenta después,
//     así que los seq se confirman en el mismo orden en que se asignan y la
//     consulta periódica con after nunca se salta un mensaje.
//  2. Inserta el mensaje con ese seq. Un clientId repetido aborta todo
//     (domain.ErrDuplicateClientID): no suma no leídos ni cambia el último.
//
// Devuelve el mensaje guardado y los no leídos que tenía receiverID antes,
// para avisar solo en el primero (P-05).
func (s *Sender) Send(ctx context.Context, m domain.Message, receiverID string, lm domain.LastMessage) (domain.Message, int, error) {
	convOID, err := bson.ObjectIDFromHex(m.ConversationID)
	if err != nil {
		return domain.Message{}, 0, domain.ErrNotFound
	}
	field, err := unreadField(receiverID)
	if err != nil {
		return domain.Message{}, 0, err
	}
	sess, err := s.client.StartSession()
	if err != nil {
		return domain.Message{}, 0, fmt.Errorf("chat: iniciar sesión de MongoDB: %w", err)
	}
	defer sess.EndSession(ctx)

	var saved domain.Message
	var prevUnread int
	_, err = sess.WithTransaction(ctx, func(tx context.Context) (any, error) {
		var before conversationDoc
		err := s.convs.FindOneAndUpdate(tx, bson.D{{Key: "_id", Value: convOID}}, bson.D{
			{Key: "$inc", Value: bson.D{{Key: "seq", Value: int64(1)}, {Key: field, Value: 1}}},
			{Key: "$set", Value: bson.D{
				{Key: "lastMessage", Value: lastMessageDoc{Preview: lm.Preview, At: lm.At.UTC(), SenderID: lm.SenderID}},
				{Key: "lastMessageAt", Value: lm.At.UTC()},
			}},
		}, options.FindOneAndUpdate().SetReturnDocument(options.Before)).Decode(&before)
		if errors.Is(err, drv.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		d := messageDoc{
			ID: bson.NewObjectID(), ConversationID: m.ConversationID, SenderID: m.SenderID, Seq: before.Seq + 1,
			Body: m.Body, ClientID: m.ClientID, CreatedAt: m.CreatedAt.UTC(),
		}
		if _, err := s.msgs.InsertOne(tx, d); err != nil {
			if drv.IsDuplicateKeyError(err) {
				return nil, domain.ErrDuplicateClientID
			}
			return nil, err
		}
		saved, prevUnread = d.toDomain(), before.Unread[receiverID]
		return nil, nil
	})
	if err != nil {
		if errors.Is(err, domain.ErrDuplicateClientID) || errors.Is(err, domain.ErrNotFound) {
			return domain.Message{}, 0, err
		}
		return domain.Message{}, 0, fmt.Errorf("chat: guardar mensaje: %w", err)
	}
	return saved, prevUnread, nil
}
