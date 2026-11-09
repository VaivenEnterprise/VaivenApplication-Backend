package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
)

// Rate guarda la calificación de userID a la otra parte de la reserva
// (RF-01, RF-02). El orden de las respuestas evita revelar reservas ajenas:
// quien no es parte recibe 404 antes de cualquier otra validación. Una
// segunda calificación propia, incluso simultánea, responde 409 (CA-03,
// RNF-01). Avisa a la persona calificada (RF-10); un fallo al avisar no deshace
// la calificación.
func (s *Service) Rate(ctx context.Context, userID, bookingID string, score int, comment *string) (RatingView, error) {
	b, err := s.d.Bookings.ByID(ctx, bookingID)
	if err != nil {
		return RatingView{}, err
	}
	now := s.now()
	if err := domain.CanRate(b, userID, now); err != nil {
		return RatingView{}, err
	}
	r, err := domain.New(s.d.Repo.NewID(), b, userID, score, comment, now)
	if err != nil {
		return RatingView{}, err
	}
	saved, err := s.d.Repo.Create(ctx, r)
	if err != nil {
		return RatingView{}, err
	}
	s.notifyReceived(ctx, saved)
	return RatingView{Rating: saved, RaterName: s.name(ctx, saved.RaterID)}, nil
}

// notifyReceived avisa a la persona calificada. El evento solo lleva ids
// (Art. VI.5).
func (s *Service) notifyReceived(ctx context.Context, r domain.Rating) {
	err := s.d.Notifier.Notify(ctx, r.RateeID, notify.Event{
		Type:  EventRatingReceived,
		Title: "Recibiste una calificación",
		Body:  "Alguien calificó tu participación en una reserva.",
		Data:  map[string]string{"bookingId": r.BookingID, "ratingId": r.ID},
	})
	if err != nil {
		s.d.Log.WarnContext(ctx, "ratings: no se pudo notificar", "type", EventRatingReceived, "bookingId", r.BookingID, "error", err)
	}
}

// name devuelve el nombre visible de userID, o «Cuenta eliminada» si no se
// encuentra; un fallo de lectura también degrada al nombre genérico.
func (s *Service) name(ctx context.Context, userID string) string {
	m, err := s.d.Users.PublicUsers(ctx, []string{userID})
	if err != nil {
		s.d.Log.WarnContext(ctx, "ratings: no se pudo leer el perfil", "error", err)
		return DeletedName
	}
	if u, ok := m[userID]; ok && u.DisplayName != "" {
		return u.DisplayName
	}
	return DeletedName
}
