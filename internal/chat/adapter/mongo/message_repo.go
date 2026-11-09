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
// seq (orden de las páginas), y el único parcial de idempotencia por clientId.
func (r *MessageRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, []drv.IndexModel{
		{Keys: bson.D{{Key: "conversationId", Value: 1}, {Key: "seq", Value: -1}, {Key: "_id", Value: -1}}, Options: options.Index().SetName("conversation_seq")},
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

// seqOf devuelve el seq del mensaje id de la conversación; si no existe,
// domain.ErrUnknownMessage.
func (r *MessageRepo) seqOf(ctx context.Context, conversationID, id string) (int64, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return 0, domain.ErrUnknownMessage
	}
	var d messageDoc
	err = r.col.FindOne(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "conversationId", Value: conversationID}},
		options.FindOne().SetProjection(bson.D{{Key: "seq", Value: 1}})).Decode(&d)
	if errors.Is(err, drv.ErrNoDocuments) {
		return 0, domain.ErrUnknownMessage
	}
	if err != nil {
		return 0, fmt.Errorf("chat: leer cursor: %w", err)
	}
	return d.Seq, nil
}

// Before devuelve hasta limit mensajes anteriores al mensaje before (o los
// más nuevos si before es ""), del más nuevo al más viejo (RF-03).
func (r *MessageRepo) Before(ctx context.Context, conversationID, before string, limit int) ([]domain.Message, error) {
	f := bson.D{{Key: "conversationId", Value: conversationID}}
	if before != "" {
		seq, err := r.seqOf(ctx, conversationID, before)
		if err != nil {
			return nil, err
		}
		f = append(f, bson.E{Key: "seq", Value: bson.D{{Key: "$lt", Value: seq}}})
	}
	return r.find(ctx, f, -1, limit)
}

// After devuelve hasta limit mensajes posteriores al mensaje after, en orden
// ascendente (consulta periódica, RF-03). Ordena por seq, que se confirma en
// orden: no hay huecos aunque dos personas escriban a la vez.
func (r *MessageRepo) After(ctx context.Context, conversationID, after string, limit int) ([]domain.Message, error) {
	seq, err := r.seqOf(ctx, conversationID, after)
	if err != nil {
		return nil, err
	}
	return r.find(ctx, bson.D{{Key: "conversationId", Value: conversationID}, {Key: "seq", Value: bson.D{{Key: "$gt", Value: seq}}}}, 1, limit)
}

func (r *MessageRepo) find(ctx context.Context, f bson.D, dir, limit int) ([]domain.Message, error) {
	cur, err := r.col.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "seq", Value: dir}, {Key: "_id", Value: dir}}).SetLimit(int64(limit)))
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
