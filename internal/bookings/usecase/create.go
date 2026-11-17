package usecase

import (
	"context"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// QuoteInput son los parámetros de GET /items/{id}/quote.
type QuoteInput struct {
	ItemID      string
	Start, End  string
	PaymentMode domain.PaymentMode
}

// Quote cotiza un periodo sin crear nada (RF-01). El objeto debe estar
// activo y no ser de quien cotiza.
func (s *Service) Quote(ctx context.Context, viewer Viewer, in QuoteInput) (domain.Quote, error) {
	_, q, err := s.quote(ctx, viewer, in)
	return q, err
}

func (s *Service) quote(ctx context.Context, viewer Viewer, in QuoteInput) (domain.ItemForBooking, domain.Quote, error) {
	it, err := s.d.Repo.ItemForBooking(ctx, in.ItemID)
	if err != nil {
		return it, domain.Quote{}, err
	}
	if it.OwnerID == viewer.ID {
		return it, domain.Quote{}, domain.ErrOwnItem
	}
	if it.Status != "active" {
		return it, domain.Quote{}, domain.ErrItemUnavailable
	}
	q, err := domain.NewQuote(domain.QuoteInput{
		ItemID: it.ID, Modality: it.Modality, PricePerDayCents: it.PricePerDayCents,
		Start: in.Start, End: in.End, PaymentMode: in.PaymentMode, Today: s.today(),
	})
	return it, q, err
}

// CreateInput es el cuerpo de POST /bookings.
type CreateInput struct {
	QuoteInput
	AcceptedTotalCents int64
}

// Create crea una solicitud pending (RF-02 a RF-05, RF-12). Si el total
// cambió desde la cotización responde quote_changed (CA-03). Abre la
// conversación antes de guardar: si falla, la solicitud no se crea (P-06).
func (s *Service) Create(ctx context.Context, viewer Viewer, in CreateInput) (BookingView, error) {
	it, q, err := s.quote(ctx, viewer, in.QuoteInput)
	if err != nil {
		return BookingView{}, err
	}
	if q.TotalCents != in.AcceptedTotalCents {
		return BookingView{}, domain.ErrQuoteChanged
	}
	now := s.now()
	b, err := domain.NewRequest(s.d.Repo.NewID(), viewer.ID, it, q, now, s.d.Location)
	if err != nil {
		return BookingView{}, err
	}
	if taken, err := s.d.Repo.HasBlockingOverlap(ctx, b.ItemID, b.Start, b.End); err != nil {
		return BookingView{}, err
	} else if taken {
		return BookingView{}, domain.ErrPeriodTaken
	}
	if dup, err := s.d.Repo.HasPendingOverlap(ctx, b.ItemID, viewer.ID, b.Start, b.End, now); err != nil {
		return BookingView{}, err
	} else if dup {
		return BookingView{}, domain.ErrDuplicatePending
	}
	convID, err := s.d.Chat.Open(ctx, b.ID, b.OwnerID, b.RequesterID)
	if err != nil {
		return BookingView{}, shared.NewUnavailable("No pudimos abrir la conversación. Intenta de nuevo.").Wrap(fmt.Errorf("chat: %w", err))
	}
	if convID != "" {
		b.ConversationID = &convID
	}
	if err := s.d.Repo.Create(ctx, b); err != nil {
		return BookingView{}, err
	}
	s.notify(ctx, b.OwnerID, "booking.requested", "Nueva solicitud", "Alguien quiere "+b.Item.Title+".", b)
	return s.view(ctx, b, domain.RoleRequester)
}
