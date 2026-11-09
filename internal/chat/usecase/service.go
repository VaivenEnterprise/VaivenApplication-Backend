package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Deps son las dependencias del Service.
type Deps struct {
	Conversations Conversations
	Messages      Messages
	Sender        Sender
	Bookings      Bookings
	Accounts      Accounts
	Users         users.Directory
	Images        imgport.Service
	Moderation    moderation.Checker
	Notifier      notify.Notifier
	Log           *slog.Logger
	Now           func() time.Time // nil = time.Now
}

// Service implementa el chat. También cumple chat.Opener (reemplaza el
// vacío de la spec 007, plan P-07).
type Service struct{ d Deps }

var _ chat.Opener = (*Service)(nil)

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Conversations == nil || d.Messages == nil || d.Sender == nil || d.Bookings == nil || d.Accounts == nil ||
		d.Users == nil || d.Images == nil || d.Moderation == nil || d.Notifier == nil || d.Log == nil {
		return nil, errors.New("chat: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}, nil
}

func (s *Service) now() time.Time { return s.d.Now().UTC().Truncate(time.Millisecond) }

// Open abre la conversación de una reserva (RF-01, CA-01). Es idempotente
// por bookingID.
func (s *Service) Open(ctx context.Context, bookingID, ownerID, requesterID string) (string, error) {
	c, err := s.d.Conversations.Open(ctx, bookingID, ownerID, requesterID, s.now())
	if err != nil {
		return "", err
	}
	return c.ID, nil
}

// access lee la conversación y su reserva. Quien no participa recibe
// not_found (RF-05, CA-02); una conversación sin reserva (huérfana) también.
func (s *Service) access(ctx context.Context, userID, conversationID string) (domain.Conversation, domain.BookingInfo, error) {
	c, err := s.d.Conversations.ByID(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, domain.BookingInfo{}, err
	}
	if !c.IsParticipant(userID) {
		return domain.Conversation{}, domain.BookingInfo{}, domain.ErrNotFound
	}
	infos, err := s.d.Bookings.ByIDs(ctx, []string{c.BookingID})
	if err != nil {
		return domain.Conversation{}, domain.BookingInfo{}, err
	}
	b, ok := infos[c.BookingID]
	if !ok {
		return domain.Conversation{}, domain.BookingInfo{}, domain.ErrNotFound
	}
	return c, b, nil
}

// canWrite calcula si userID puede escribir (RF-06, RF-07).
func (s *Service) canWrite(ctx context.Context, c domain.Conversation, b domain.BookingInfo, userID string) (bool, error) {
	other := c.Counterpart(userID)
	active, err := s.d.Accounts.Active(ctx, []string{other})
	if err != nil {
		return false, err
	}
	return domain.CanWrite(b.Status, active[other]), nil
}

// thumb calcula la miniatura del objeto, si tiene foto.
func (s *Service) thumb(b domain.BookingInfo) (*string, error) {
	if b.ItemPhoto == nil {
		return nil, nil
	}
	u, err := s.d.Images.URLs(*b.ItemPhoto)
	if err != nil {
		return nil, fmt.Errorf("chat: miniatura: %w", err)
	}
	return &u.ThumbURL, nil
}

func profile(m map[string]users.PublicUser, id string) users.PublicUser {
	if p, ok := m[id]; ok {
		return p
	}
	return users.PublicUser{ID: id, DisplayName: DeletedName}
}
