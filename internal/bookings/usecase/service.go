package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Deps son las dependencias del Service.
type Deps struct {
	Repo     Repository
	Images   imgport.Service
	Users    users.Directory
	Chat     chat.Opener
	Notifier notify.Notifier
	Log      *slog.Logger
	Now      func() time.Time // nil = time.Now
	Location *time.Location   // nil = America/Mexico_City
}

// Service implementa los casos de uso de reservas.
type Service struct {
	d Deps
}

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Repo == nil || d.Images == nil || d.Users == nil || d.Chat == nil || d.Notifier == nil || d.Log == nil {
		return nil, errors.New("bookings: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Location == nil {
		loc, err := domain.Location()
		if err != nil {
			return nil, fmt.Errorf("bookings: zona horaria: %w", err)
		}
		d.Location = loc
	}
	return &Service{d: d}, nil
}

// now devuelve la hora en UTC truncada a milisegundos (precisión de MongoDB).
func (s *Service) now() time.Time { return s.d.Now().UTC().Truncate(time.Millisecond) }

func (s *Service) today() domain.Date { return domain.Today(s.d.Now(), s.d.Location) }

// notify avisa a userID; un fallo se registra y no deshace la operación
// (plan P-07). El evento solo lleva ids (Art. VI.5).
func (s *Service) notify(ctx context.Context, userID, typ, title, body string, b domain.Booking) {
	err := s.d.Notifier.Notify(ctx, userID, notify.Event{
		Type: typ, Title: title, Body: body,
		Data: map[string]string{"bookingId": b.ID, "itemId": b.ItemID},
	})
	if err != nil {
		s.d.Log.WarnContext(ctx, "bookings: no se pudo notificar", "type", typ, "bookingId", b.ID, "error", err)
	}
}

// load lee la reserva y el papel de viewer. Quien no es parte (ni admin si
// allowAdmin) recibe not_found, para no revelar que existe.
func (s *Service) load(ctx context.Context, viewer Viewer, id string, allowAdmin bool) (domain.Booking, domain.Role, error) {
	b, err := s.d.Repo.ByID(ctx, id)
	if err != nil {
		return domain.Booking{}, "", err
	}
	if role, ok := b.RoleOf(viewer.ID); ok {
		return b, role, nil
	}
	if allowAdmin && viewer.Admin {
		return b, domain.RoleAdmin, nil
	}
	return domain.Booking{}, "", domain.ErrNotFound
}

// view arma la respuesta con los perfiles públicos y la miniatura.
func (s *Service) view(ctx context.Context, b domain.Booking, role domain.Role) (BookingView, error) {
	profiles, err := s.d.Users.PublicUsers(ctx, []string{b.OwnerID, b.RequesterID})
	if err != nil {
		return BookingView{}, fmt.Errorf("bookings: perfiles: %w", err)
	}
	return s.viewWith(b, role, profiles)
}

func (s *Service) viewWith(b domain.Booking, role domain.Role, profiles map[string]users.PublicUser) (BookingView, error) {
	b.Status = b.EffectiveStatus(s.now())
	if b.Status != domain.StatusPending {
		b.ExpiresAt = nil
	}
	v := BookingView{
		Booking:          b,
		Owner:            profile(profiles, b.OwnerID),
		Requester:        profile(profiles, b.RequesterID),
		ViewerRole:       role,
		ShowMeetingPoint: role == domain.RoleOwner || role == domain.RoleRequester,
	}
	if p := b.Item.FirstPhoto; p != nil {
		u, err := s.d.Images.URLs(*p)
		if err != nil {
			return BookingView{}, fmt.Errorf("bookings: miniatura: %w", err)
		}
		v.ThumbURL = &u.ThumbURL
	}
	if role == domain.RoleOwner || role == domain.RoleRequester {
		var err error
		if v.Delivery, err = s.handoverView(b.Delivery); err != nil {
			return BookingView{}, err
		}
		if v.Return, err = s.handoverView(b.Return); err != nil {
			return BookingView{}, err
		}
	}
	return v, nil
}

// handoverView arma una etapa con las URL firmadas de sus fotos; nil si la
// etapa no tiene datos (spec 009, RF-05).
func (s *Service) handoverView(h *domain.Handover) (*HandoverView, error) {
	if h == nil {
		return nil, nil
	}
	v := &HandoverView{Stage: *h, QR: h.QRStateAt(s.now())}
	for _, p := range h.Photos {
		u, err := s.d.Images.URLs(p)
		if err != nil {
			return nil, fmt.Errorf("bookings: URL de evidencia: %w", err)
		}
		v.URLs = append(v.URLs, u)
	}
	if h.QR != nil && v.QR != domain.QRNone {
		exp := h.QR.ExpiresAt
		v.QRExpiresAt = &exp
	}
	return v, nil
}

func profile(m map[string]users.PublicUser, id string) users.PublicUser {
	if p, ok := m[id]; ok {
		return p
	}
	return users.PublicUser{ID: id, DisplayName: DeletedName}
}
