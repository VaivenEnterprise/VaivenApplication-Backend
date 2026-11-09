package usecase

import (
	"context"
	"errors"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
)

// StatusView es el resultado de RF-03 con la calificación propia ya lista.
type StatusView struct {
	Status domain.Status
	// MyRaterName es el nombre visible de quien consulta, para mostrar myRating.
	MyRaterName string
}

// Status indica si userID puede calificar la reserva, su calificación si ya la
// dio y cuándo vence la ventana (RF-03, plan P-03). Quien no es parte recibe
// 404.
func (s *Service) Status(ctx context.Context, userID, bookingID string) (StatusView, error) {
	b, err := s.d.Bookings.ByID(ctx, bookingID)
	if err != nil {
		return StatusView{}, err
	}
	if _, ok := b.RoleOf(userID); !ok {
		return StatusView{}, bookings.ErrNotFound
	}
	var st domain.Status
	if at, ok := domain.CompletedAt(b); ok {
		exp := domain.ExpiresAt(at)
		st.ExpiresAt = &exp
	}
	mine, found, err := s.d.Repo.ByBookingAndRater(ctx, b.ID, userID)
	if err != nil {
		return StatusView{}, err
	}
	if found {
		st.MyRating = &mine
	}
	err = domain.CanRate(b, userID, s.now())
	switch {
	case err == nil:
		st.CanRate = !found
	case errors.Is(err, domain.ErrNotCompleted), errors.Is(err, domain.ErrWindowClosed):
		st.CanRate = false
	default:
		return StatusView{}, err
	}
	out := StatusView{Status: st}
	if found {
		out.MyRaterName = s.name(ctx, userID)
	}
	return out, nil
}
