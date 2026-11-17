// Package mongo guarda las conversaciones con el asistente de proyectos
// (spec 014, RF-13; plan P-09) en la colección ai_conversations:
//
//	{ _id, userId, turns: [{role, text}], createdAt, updatedAt }
//
// Cada operación filtra por _id y userId: una conversación de otra persona se
// comporta como inexistente (CA-13). turns es un arreglo acotado ($slice a
// MaxTurns, constitución §6.2) y el TTL borra la conversación 24 h después del
// último mensaje.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/domain"
)

// ConversationsCollection es la colección de conversaciones.
const ConversationsCollection = "ai_conversations"

// ConversationIndexes declara el TTL sobre updatedAt. Las búsquedas son por
// _id (con userId en el filtro), así que no hace falta otro índice.
func ConversationIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(ConversationsCollection, drv.IndexModel{
		Keys:    bson.D{{Key: "updatedAt", Value: 1}},
		Options: options.Index().SetName("updated_ttl").SetExpireAfterSeconds(domain.ConversationTTLSeconds),
	})
}

type turnDoc struct {
	Role string `bson:"role"`
	Text string `bson:"text"`
}

type conversationDoc struct {
	ID        bson.ObjectID `bson:"_id"`
	UserID    string        `bson:"userId"`
	Turns     []turnDoc     `bson:"turns"`
	CreatedAt time.Time     `bson:"createdAt"`
	UpdatedAt time.Time     `bson:"updatedAt"`
}

// ConversationRepo es el repositorio de conversaciones.
type ConversationRepo struct {
	col *drv.Collection
}

// NewConversationRepo crea el repositorio sobre db.
func NewConversationRepo(db *drv.Database) *ConversationRepo {
	return &ConversationRepo{col: db.Collection(ConversationsCollection)}
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *ConversationRepo) Indexes() []adaptermongo.IndexSpec { return ConversationIndexes() }

// EnsureIndexes crea los índices de forma idempotente.
func (r *ConversationRepo) EnsureIndexes(ctx context.Context) error {
	if _, err := r.col.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes())); err != nil {
		return fmt.Errorf("ai_conversations: crear índices: %w", err)
	}
	return nil
}

// Create abre una conversación de userID con sus primeros turnos.
func (r *ConversationRepo) Create(ctx context.Context, userID string, turns []aidomain.Turn, now time.Time) (string, error) {
	now = now.UTC()
	doc := conversationDoc{ID: bson.NewObjectID(), UserID: userID, Turns: toDocs(lastTurns(turns)), CreatedAt: now, UpdatedAt: now}
	if _, err := r.col.InsertOne(ctx, doc); err != nil {
		return "", fmt.Errorf("ai_conversations: crear: %w", err)
	}
	return doc.ID.Hex(), nil
}

// Get devuelve la conversación id de userID.
func (r *ConversationRepo) Get(ctx context.Context, id, userID string) (domain.Conversation, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Conversation{}, domain.ErrConversationNotFound
	}
	var doc conversationDoc
	err = r.col.FindOne(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "userId", Value: userID}}).Decode(&doc)
	if errors.Is(err, drv.ErrNoDocuments) {
		return domain.Conversation{}, domain.ErrConversationNotFound
	}
	if err != nil {
		return domain.Conversation{}, fmt.Errorf("ai_conversations: leer: %w", err)
	}
	c := domain.Conversation{ID: doc.ID.Hex(), UserID: doc.UserID}
	for _, t := range doc.Turns {
		c.Turns = append(c.Turns, aidomain.Turn{Role: t.Role, Text: t.Text})
	}
	return c, nil
}

// Append agrega turnos a la conversación id de userID y conserva solo los
// últimos MaxTurns ($push con $slice).
func (r *ConversationRepo) Append(ctx context.Context, id, userID string, turns []aidomain.Turn, now time.Time) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrConversationNotFound
	}
	update := bson.D{
		{Key: "$push", Value: bson.D{{Key: "turns", Value: bson.D{
			{Key: "$each", Value: toDocs(turns)},
			{Key: "$slice", Value: -domain.MaxTurns},
		}}}},
		{Key: "$set", Value: bson.D{{Key: "updatedAt", Value: now.UTC()}}},
	}
	res, err := r.col.UpdateOne(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "userId", Value: userID}}, update)
	if err != nil {
		return fmt.Errorf("ai_conversations: agregar turnos: %w", err)
	}
	if res.MatchedCount == 0 {
		return domain.ErrConversationNotFound
	}
	return nil
}

// Delete borra la conversación id de userID (DELETE de la API, RF-13).
func (r *ConversationRepo) Delete(ctx context.Context, id, userID string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrConversationNotFound
	}
	res, err := r.col.DeleteOne(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "userId", Value: userID}})
	if err != nil {
		return fmt.Errorf("ai_conversations: borrar: %w", err)
	}
	if res.DeletedCount == 0 {
		return domain.ErrConversationNotFound
	}
	return nil
}

// toDocs copia los turnos recortando cada texto a MaxTurnRunes (defensa extra:
// el caso de uso ya los limpia).
func toDocs(turns []aidomain.Turn) []turnDoc {
	out := make([]turnDoc, 0, len(turns))
	for _, t := range turns {
		text := t.Text
		if utf8.RuneCountInString(text) > domain.MaxTurnRunes {
			text = string([]rune(text)[:domain.MaxTurnRunes])
		}
		out = append(out, turnDoc{Role: t.Role, Text: text})
	}
	return out
}

func lastTurns(turns []aidomain.Turn) []aidomain.Turn {
	if len(turns) > domain.MaxTurns {
		return turns[len(turns)-domain.MaxTurns:]
	}
	return turns
}
