package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
)

// List devuelve las conversaciones de userID con contraparte, objeto, último
// mensaje, mis no leídos y canWrite (RF-02). Las conversaciones sin reserva
// (la solicitud no llegó a guardarse) se omiten.
func (s *Service) List(ctx context.Context, userID string, after *domain.Cursor, limit int) (Page, error) {
	if limit <= 0 {
		limit = DefaultConversations
	}
	limit = min(limit, MaxConversations)
	convs, err := s.d.Conversations.ListByParticipant(ctx, userID, after, limit+1)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: make([]ConversationView, 0, len(convs))}
	if len(convs) > limit {
		convs = convs[:limit]
		last := convs[limit-1]
		page.Next = &domain.Cursor{At: last.LastMessageAt, ID: last.ID}
	}
	if len(convs) == 0 {
		return page, nil
	}
	bookingIDs := make([]string, 0, len(convs))
	others := make([]string, 0, len(convs))
	for _, c := range convs {
		bookingIDs = append(bookingIDs, c.BookingID)
		others = append(others, c.Counterpart(userID))
	}
	infos, err := s.d.Bookings.ByIDs(ctx, bookingIDs)
	if err != nil {
		return Page{}, err
	}
	profiles, err := s.d.Users.PublicUsers(ctx, others)
	if err != nil {
		return Page{}, err
	}
	active, err := s.d.Accounts.Active(ctx, others)
	if err != nil {
		return Page{}, err
	}
	for _, c := range convs {
		b, ok := infos[c.BookingID]
		if !ok {
			continue
		}
		other := c.Counterpart(userID)
		thumb, err := s.thumb(b)
		if err != nil {
			return Page{}, err
		}
		page.Items = append(page.Items, ConversationView{
			Conversation: c, Booking: b, Counterpart: profile(profiles, other), ThumbURL: thumb,
			CanWrite: domain.CanWrite(b.Status, active[other]), Unread: c.UnreadFor(userID),
		})
	}
	return page, nil
}

// UnreadCount suma mis no leídos (RF-09).
func (s *Service) UnreadCount(ctx context.Context, userID string) (int, error) {
	return s.d.Conversations.UnreadTotal(ctx, userID)
}
