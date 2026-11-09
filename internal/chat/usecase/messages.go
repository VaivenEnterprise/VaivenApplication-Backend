package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

// Messages devuelve mensajes de una conversación a quien participa (RF-03,
// RF-05): sin After, del más nuevo al más viejo con cursor hacia atrás; con
// After, los más nuevos en orden ascendente (máximo 100). El historial se lee
// aunque la conversación esté cerrada (CA-04).
func (s *Service) Messages(ctx context.Context, userID, conversationID string, q MessageQuery) (MessagePage, error) {
	if q.Before != "" && q.After != "" {
		return MessagePage{}, shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: "after", Issue: "no se combina con before"})
	}
	c, _, err := s.access(ctx, userID, conversationID)
	if err != nil {
		return MessagePage{}, err
	}
	if q.After != "" {
		items, err := s.d.Messages.After(ctx, c.ID, q.After, MaxAfter)
		if err != nil {
			return MessagePage{}, cursorError(err, "after")
		}
		return MessagePage{Items: items}, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultMessages
	}
	limit = min(limit, MaxMessages)
	items, err := s.d.Messages.Before(ctx, c.ID, q.Before, limit+1)
	if err != nil {
		return MessagePage{}, cursorError(err, "before")
	}
	page := MessagePage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		next := page.Items[limit-1].ID
		page.Next = &next
	}
	return page, nil
}

// Send envía un mensaje (RF-04, RF-06, RF-07, RF-10, RF-12). Con clientID
// repetido devuelve el mensaje existente y created=false (CA-03). Avisa a la
// contraparte solo si no tenía no leídos (P-05).
func (s *Service) Send(ctx context.Context, userID, conversationID, body string, clientID *string) (domain.Message, bool, error) {
	text, err := domain.NormalizeBody(body)
	if err != nil {
		return domain.Message{}, false, err
	}
	cid, err := domain.NormalizeClientID(clientID)
	if err != nil {
		return domain.Message{}, false, err
	}
	c, b, err := s.access(ctx, userID, conversationID)
	if err != nil {
		return domain.Message{}, false, err
	}
	if cid != nil {
		if m, ok, err := s.d.Messages.ByClientID(ctx, c.ID, userID, *cid); err != nil || ok {
			return m, false, err
		}
	}
	ok, err := s.canWrite(ctx, c, b, userID)
	if err != nil {
		return domain.Message{}, false, err
	}
	if !ok {
		return domain.Message{}, false, domain.ErrClosed
	}
	res, err := s.d.Moderation.Check(ctx, text)
	if err != nil {
		return domain.Message{}, false, fmt.Errorf("chat: moderación: %w", err)
	}
	if !res.Allowed {
		reason := res.Reason
		if reason == "" {
			reason = "contiene contenido no permitido"
		}
		return domain.Message{}, false, shared.NewValidation("El mensaje no cumple las reglas de contenido.", shared.Detail{Field: "body", Issue: reason})
	}
	now := s.now()
	other := c.Counterpart(userID)
	// Mensaje, no leídos, último mensaje y seq se guardan juntos (A-01): si
	// algo falla no queda un mensaje sin contar ni avisar.
	m, prev, err := s.d.Sender.Send(ctx,
		domain.Message{ConversationID: c.ID, SenderID: userID, Body: text, ClientID: cid, CreatedAt: now},
		other, domain.LastMessage{Preview: domain.Preview(text), At: now, SenderID: userID})
	if errors.Is(err, domain.ErrDuplicateClientID) && cid != nil {
		// Dos envíos simultáneos con el mismo clientId: gana el primero.
		existing, ok, err := s.d.Messages.ByClientID(ctx, c.ID, userID, *cid)
		if err != nil {
			return domain.Message{}, false, err
		}
		if !ok {
			return domain.Message{}, false, errors.New("chat: clientId repetido sin mensaje guardado")
		}
		return existing, false, nil
	}
	if err != nil {
		return domain.Message{}, false, err
	}
	if prev == 0 {
		s.notify(ctx, other, c, b)
	}
	return m, true, nil
}

// notify avisa del primer mensaje sin leer (RF-12). Un fallo se registra y no
// deshace el envío. El aviso no lleva el texto del mensaje (Art. VI.5).
func (s *Service) notify(ctx context.Context, to string, c domain.Conversation, b domain.BookingInfo) {
	err := s.d.Notifier.Notify(ctx, to, notify.Event{
		Type: EventMessageReceived, Title: "Nuevo mensaje", Body: "Tienes un mensaje sobre " + b.ItemTitle + ".",
		Data: map[string]string{"conversationId": c.ID, "bookingId": c.BookingID},
	})
	if err != nil {
		s.d.Log.WarnContext(ctx, "chat: no se pudo notificar", "conversationId", c.ID, "error", err)
	}
}

// Read pone en cero mis no leídos (RF-08). No registra nada de la otra parte.
func (s *Service) Read(ctx context.Context, userID, conversationID string) error {
	c, _, err := s.access(ctx, userID, conversationID)
	if err != nil {
		return err
	}
	return s.d.Conversations.MarkRead(ctx, c.ID, userID)
}

// cursorError traduce un cursor que no es de la conversación en
// validation_failed sobre el parámetro usado.
func cursorError(err error, field string) error {
	if errors.Is(err, domain.ErrUnknownMessage) {
		return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: field, Issue: "no es un mensaje de esta conversación"})
	}
	return err
}
