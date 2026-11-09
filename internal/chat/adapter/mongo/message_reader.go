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

// MessageReader lee un mensaje junto con los participantes de su conversación
// (solo lectura) para que los reportes (spec 011, RF-08) comprueben que quien
// reporta participa en ella.
type MessageReader struct{ messages, conversations *drv.Collection }

// NewMessageReader crea el lector sobre db.
func NewMessageReader(db *drv.Database) MessageReader {
	return MessageReader{messages: db.Collection(MessagesCollection), conversations: db.Collection(ConversationsCollection)}
}

// Reference busca el mensaje id. found=false si el id no es válido, el mensaje
// no existe o su conversación ya no existe.
func (r MessageReader) Reference(ctx context.Context, id string) (domain.MessageRef, bool, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.MessageRef{}, false, nil
	}
	var m messageDoc
	err = r.messages.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&m)
	if errors.Is(err, drv.ErrNoDocuments) {
		return domain.MessageRef{}, false, nil
	}
	if err != nil {
		return domain.MessageRef{}, false, fmt.Errorf("chat: leer mensaje: %w", err)
	}
	convID, err := bson.ObjectIDFromHex(m.ConversationID)
	if err != nil {
		return domain.MessageRef{}, false, nil
	}
	var c struct {
		ParticipantIDs []string `bson:"participantIds"`
	}
	err = r.conversations.FindOne(ctx, bson.D{{Key: "_id", Value: convID}},
		options.FindOne().SetProjection(bson.D{{Key: "participantIds", Value: 1}})).Decode(&c)
	if errors.Is(err, drv.ErrNoDocuments) {
		return domain.MessageRef{}, false, nil
	}
	if err != nil {
		return domain.MessageRef{}, false, fmt.Errorf("chat: leer conversación: %w", err)
	}
	ref := domain.MessageRef{ID: m.ID.Hex(), SenderID: m.SenderID, Body: m.Body}
	for i := 0; i < len(c.ParticipantIDs) && i < 2; i++ {
		ref.Participants[i] = c.ParticipantIDs[i]
	}
	return ref, true, nil
}
