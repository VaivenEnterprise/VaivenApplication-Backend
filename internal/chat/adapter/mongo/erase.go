package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

// DeletedMessageText es el texto que reemplaza a los mensajes (y al extracto
// del último mensaje) de una cuenta eliminada (spec 021, D-1, N-4): viaja como
// texto normal, sin campo nuevo en el contrato.
const DeletedMessageText = "Mensaje eliminado"

// Eraser es la parte de chat de la baja de cuenta (spec 021, D-1; Art. XII):
// cumple erasure.Eraser sobre conversations y messages, y solo sobre ellas.
// Se llama DENTRO de la transacción de la baja, con su ctx.
//
// Reemplaza el texto de los mensajes de la persona por «Mensaje eliminado» y el
// extracto de la conversación si el último mensaje es suyo. Orden, seq, fechas,
// autor (id seudónimo) y no leídos no cambian: la contraparte lee el historial
// completo. El chat con una cuenta eliminada queda de solo lectura: enviar ya
// responde 409 «La conversación está cerrada.» porque AccountReader.Active ve
// la cuenta dada de baja (RF-08).
type Eraser struct {
	convs *drv.Collection
	msgs  *drv.Collection
}

// NewEraser crea el eraser sobre db.
func NewEraser(db *drv.Database) *Eraser {
	return &Eraser{convs: db.Collection(ConversationsCollection), msgs: db.Collection(MessagesCollection)}
}

// Name identifica al eraser en los errores de la baja.
func (e *Eraser) Name() string { return "chat" }

// Erase aplica la baja de userID al chat. Es idempotente: filtra por texto
// distinto de DeletedMessageText, así repetirla no toca nada.
func (e *Eraser) Erase(ctx context.Context, userID string, _ time.Time) (erasure.EraseResult, error) {
	res := erasure.EraseResult{Counts: map[string]int64{}}

	// Conversaciones de la persona (índice participant_lastMessageAt).
	cur, err := e.convs.Find(ctx, bson.D{{Key: "participantIds", Value: userID}},
		options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return res, fmt.Errorf("chat: buscar conversaciones: %w", err)
	}
	var convs []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &convs); err != nil {
		return res, fmt.Errorf("chat: leer conversaciones: %w", err)
	}
	if len(convs) == 0 {
		return res, nil
	}
	ids := make(bson.A, 0, len(convs))
	for _, c := range convs {
		ids = append(ids, c.ID.Hex()) // messages.conversationId guarda el id en texto
	}

	m, err := e.msgs.UpdateMany(ctx, bson.D{
		{Key: "conversationId", Value: bson.D{{Key: "$in", Value: ids}}},
		{Key: "senderId", Value: userID},
		{Key: "body", Value: bson.D{{Key: "$ne", Value: DeletedMessageText}}},
	}, bson.D{{Key: "$set", Value: bson.D{{Key: "body", Value: DeletedMessageText}}}})
	if err != nil {
		return res, fmt.Errorf("chat: borrar texto de mensajes: %w", err)
	}
	res.Counts["messages.erased"] = m.ModifiedCount

	c, err := e.convs.UpdateMany(ctx, bson.D{
		{Key: "participantIds", Value: userID},
		{Key: "lastMessage.senderId", Value: userID},
		{Key: "lastMessage.preview", Value: bson.D{{Key: "$ne", Value: DeletedMessageText}}},
	}, bson.D{{Key: "$set", Value: bson.D{{Key: "lastMessage.preview", Value: DeletedMessageText}}}})
	if err != nil {
		return res, fmt.Errorf("chat: borrar extracto de conversaciones: %w", err)
	}
	res.Counts["conversations.previewErased"] = c.ModifiedCount
	return res, nil
}

// Comprobación en compilación del puerto de la baja.
var _ erasure.Eraser = (*Eraser)(nil)
