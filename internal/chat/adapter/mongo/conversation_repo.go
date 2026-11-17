package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
)

// ConversationRepo guarda las conversaciones.
type ConversationRepo struct{ col *drv.Collection }

// NewConversationRepo crea el repositorio sobre db.
func NewConversationRepo(db *drv.Database) *ConversationRepo {
	return &ConversationRepo{col: db.Collection(ConversationsCollection)}
}

// ConversationIndexes declara los índices de ConversationsCollection: es la fuente única
// de verdad que consumen EnsureIndexes y el catálogo de `admin indexes`
// (spec 012, T004). Solo datos: no abre conexiones.
func ConversationIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(ConversationsCollection, []drv.IndexModel{
		{Keys: bson.D{{Key: "bookingId", Value: 1}}, Options: options.Index().SetName("bookingId_unique").SetUnique(true)},
		{Keys: bson.D{{Key: "participantIds", Value: 1}, {Key: "lastMessageAt", Value: -1}, {Key: "_id", Value: -1}},
			Options: options.Index().SetName("participant_lastMessageAt")},
	}...)
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *ConversationRepo) Indexes() []adaptermongo.IndexSpec { return ConversationIndexes() }

// EnsureIndexes crea los índices de conversations (plan §3).
func (r *ConversationRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes()))
	if err != nil {
		return fmt.Errorf("chat: crear índices de conversaciones: %w", err)
	}
	return nil
}

// unreadField devuelve la ruta unread.<userId>. Solo acepta ids de 24
// caracteres hexadecimales en minúsculas (ObjectID), para que ningún valor
// pueda alterar la ruta del documento (revisión T009, observación 6).
func unreadField(userID string) (string, error) {
	if !isObjectIDHex(userID) {
		return "", errors.New("chat: id de usuario inválido")
	}
	return "unread." + userID, nil
}

func isObjectIDHex(s string) bool {
	if len(s) != 24 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Open crea la conversación de una reserva o devuelve la existente
// (idempotente por bookingId, plan P-07).
func (r *ConversationRepo) Open(ctx context.Context, bookingID, ownerID, requesterID string, now time.Time) (domain.Conversation, error) {
	update := bson.D{{Key: "$setOnInsert", Value: bson.D{
		{Key: "participantIds", Value: bson.A{ownerID, requesterID}},
		{Key: "unread", Value: bson.D{}},
		{Key: "lastMessage", Value: nil},
		{Key: "lastMessageAt", Value: now.UTC()},
		{Key: "createdAt", Value: now.UTC()},
	}}}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	for attempt := 0; attempt < 2; attempt++ {
		var d conversationDoc
		err := r.col.FindOneAndUpdate(ctx, bson.D{{Key: "bookingId", Value: bookingID}}, update, opts).Decode(&d)
		if err == nil {
			return d.toDomain(), nil
		}
		// Dos aperturas simultáneas: la segunda choca con el índice único y
		// al reintentar encuentra la primera.
		if !drv.IsDuplicateKeyError(err) {
			return domain.Conversation{}, fmt.Errorf("chat: abrir conversación: %w", err)
		}
	}
	return domain.Conversation{}, errors.New("chat: no se pudo abrir la conversación")
}

// ByID busca una conversación; si no existe, domain.ErrNotFound.
func (r *ConversationRepo) ByID(ctx context.Context, id string) (domain.Conversation, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Conversation{}, domain.ErrNotFound
	}
	var d conversationDoc
	if err := r.col.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&d); err != nil {
		if errors.Is(err, drv.ErrNoDocuments) {
			return domain.Conversation{}, domain.ErrNotFound
		}
		return domain.Conversation{}, fmt.Errorf("chat: buscar conversación: %w", err)
	}
	return d.toDomain(), nil
}

// ListByParticipant devuelve las conversaciones de userID, la de mensaje más
// reciente primero, desde after (exclusivo).
func (r *ConversationRepo) ListByParticipant(ctx context.Context, userID string, after *domain.Cursor, limit int) ([]domain.Conversation, error) {
	f := bson.D{{Key: "participantIds", Value: userID}}
	if after != nil {
		oid, err := bson.ObjectIDFromHex(after.ID)
		if err != nil {
			return nil, fmt.Errorf("chat: cursor inválido: %w", err)
		}
		at := after.At.UTC()
		f = append(f, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "lastMessageAt", Value: bson.D{{Key: "$lt", Value: at}}}},
			bson.D{{Key: "lastMessageAt", Value: at}, {Key: "_id", Value: bson.D{{Key: "$lt", Value: oid}}}},
		}})
	}
	cur, err := r.col.Find(ctx, f, options.Find().
		SetSort(bson.D{{Key: "lastMessageAt", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("chat: listar conversaciones: %w", err)
	}
	var docs []conversationDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("chat: leer conversaciones: %w", err)
	}
	out := make([]domain.Conversation, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

// MarkRead pone en cero los no leídos de userID (RF-08).
func (r *ConversationRepo) MarkRead(ctx context.Context, conversationID, userID string) error {
	oid, err := bson.ObjectIDFromHex(conversationID)
	if err != nil {
		return domain.ErrNotFound
	}
	field, err := unreadField(userID)
	if err != nil {
		return err
	}
	if _, err := r.col.UpdateOne(ctx, bson.D{{Key: "_id", Value: oid}}, bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: 0}}}}); err != nil {
		return fmt.Errorf("chat: marcar como leída: %w", err)
	}
	return nil
}

// UnreadTotal suma los no leídos de userID en todas sus conversaciones (RF-09).
func (r *ConversationRepo) UnreadTotal(ctx context.Context, userID string) (int, error) {
	field, err := unreadField(userID)
	if err != nil {
		return 0, err
	}
	cur, err := r.col.Aggregate(ctx, bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "participantIds", Value: userID}}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$" + field, 0}}}}}},
		}}},
	})
	if err != nil {
		return 0, fmt.Errorf("chat: contar no leídos: %w", err)
	}
	var res []struct {
		Count int `bson:"count"`
	}
	if err := cur.All(ctx, &res); err != nil {
		return 0, fmt.Errorf("chat: contar no leídos: %w", err)
	}
	if len(res) == 0 {
		return 0, nil
	}
	return res[0].Count, nil
}
