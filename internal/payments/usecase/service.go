package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

// Service implementa los casos de uso de pagos y retrasos.
type Service struct{ d Deps }

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Repo == nil || d.Bookings == nil || d.Notifier == nil || d.Log == nil {
		return nil, errors.New("payments: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Location == nil {
		loc, err := bookings.Location()
		if err != nil {
			return nil, fmt.Errorf("payments: zona horaria: %w", err)
		}
		d.Location = loc
	}
	return &Service{d: d}, nil
}

// now devuelve la hora en UTC truncada a milisegundos (precisión de MongoDB).
func (s *Service) now() time.Time { return s.d.Now().UTC().Truncate(time.Millisecond) }

// load lee la reserva; quien no es parte recibe not_found (no se revela que existe).
func (s *Service) load(ctx context.Context, v Viewer, id string) (bookings.Booking, error) {
	b, err := s.d.Bookings.ByID(ctx, id)
	if err != nil {
		return bookings.Booking{}, err
	}
	if _, ok := b.RoleOf(v.ID); !ok {
		return bookings.Booking{}, bookings.ErrNotFound
	}
	return b, nil
}

// notifyUser avisa a userID; un fallo se registra y no deshace nada. El evento
// solo lleva ids (Art. VI.5).
func (s *Service) notifyUser(ctx context.Context, userID, typ, title, body string, bookingID, itemID string) {
	err := s.d.Notifier.Notify(ctx, userID, notify.Event{
		Type: typ, Title: title, Body: body,
		Data: map[string]string{"bookingId": bookingID, "itemId": itemID},
	})
	if err != nil {
		s.d.Log.WarnContext(ctx, "payments: no se pudo notificar", "type", typ, "bookingId", bookingID, "error", err)
	}
}
