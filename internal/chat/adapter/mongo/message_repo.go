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

// MessageRepo guarda los mensajes.
type MessageRepo struct{ col *drv.Collection }

// NewMessageRepo crea el repositorio sobre db.
func NewMessageRepo(db *drv.Database) *MessageRepo {
	return &MessageRepo{col: db.Collection(MessagesCollection)}
}

// EnsureIndexes crea los índices de messages (plan §3): por conversación y
// _id, y el único parcial de idempotencia por clientId.
func (r *MessageRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, []drv.IndexModel{
		{Keys: bson.D{{Key: "conversationId", Value: 1}, {Key: "_id", Value: -1}}, Options: options.Index().SetName("conversation_id")},
		{
			Keys: bson.D{{Key: "conversationId", Value: 1}, {Key: "senderId", Value: 1}, {Key: "clientId", Value: 1}},
			Options: options.Index().SetName("conversation_sender_clientId_unique").SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: "clientId", Value: bson.D{{Key: "$type", Value: "string"}}}}),
		},
	})
	if err != nil {
		return fmt.Errorf("chat: crear índices de mensajes: %w", err)
	}
	return nil
}

// Insert guarda el mensaje con un id nuevo. Un clientId repetido del mismo
// remitente devuelve domain.ErrDuplicateClientID.
func (r *MessageRepo) Insert(ctx context.Context, m domain.Message) (domain.Message, error) {
	d := messageDoc{
		ID: bson.NewObjectID(), ConversationID: m.ConversationID, SenderID: m.SenderID,
		Body: m.Body, ClientID: m.ClientID, CreatedAt: m.CreatedAt.UTC(),
	}
	if _, err := r.col.InsertOne(ctx, d); err != nil {
		if drv.IsDuplicateKeyError(err) {
			return domain.Message{}, domain.ErrDuplicateClientID
		}
		return domain.Message{}, fmt.Errorf("chat: guardar mensaje: %w", err)
	}
	return d.toDomain(), nil
}

// ByClientID busca el mensaje que senderID ya envió con clientID.
func (r *MessageRepo) ByClientID(ctx context.Context, conversationID, senderID, clientID string) (domain.Message, bool, error) {
	var d messageDoc
	err := r.col.FindOne(ctx, bson.D{
		{Key: "conversationId", Value: conversationID}, {Key: "senderId", Value: senderID}, {Key: "clientId", Value: clientID},
	}).Decode(&d)
	if errors.Is(err, drv.ErrNoDocuments) {
		return domain.Message{}, false, nil
	}
	if err != nil {
		return domain.Message{}, false, fmt.Errorf("chat: buscar mensaje: %w", err)
	}
	return d.toDomain(), true, nil
}

// Before devuelve hasta limit mensajes anteriores a before (o los más nuevos
// si before es ""), del más nuevo al más viejo (RF-03).
func (r *MessageRepo) Before(ctx context.Context, conversationID, before string, limit int) ([]domain.Message, error) {
	f := bson.D{{Key: "conversationId", Value: conversationID}}
	if before != "" {
		oid, err := bson.ObjectIDFromHex(before)
		if err != nil {
			return nil, fmt.Errorf("chat: cursor inválido: %w", err)
		}
		f = append(f, bson.E{Key: "_id", Value: bson.D{{Key: "$lt", Value: oid}}})
	}
	return r.find(ctx, f, -1, limit)
}

// After devuelve hasta limit mensajes posteriores a after, en orden
// ascendente (consulta periódica, RF-03).
func (r *MessageRepo) After(ctx context.Context, conversationID, after string, limit int) ([]domain.Message, error) {
	oid, err := bson.ObjectIDFromHex(after)
	if err != nil {
		return nil, fmt.Errorf("chat: cursor inválido: %w", err)
	}
	return r.find(ctx, bson.D{{Key: "conversationId", Value: conversationID}, {Key: "_id", Value: bson.D{{Key: "$gt", Value: oid}}}}, 1, limit)
}

func (r *MessageRepo) find(ctx context.Context, f bson.D, dir, limit int) ([]domain.Message, error) {
	cur, err := r.col.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "_id", Value: dir}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("chat: leer mensajes: %w", err)
	}
	var docs []messageDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("chat: leer mensajes: %w", err)
	}
	out := make([]domain.Message, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}
