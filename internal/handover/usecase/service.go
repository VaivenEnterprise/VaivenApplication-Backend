package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

// Service implementa los casos de uso de la entrega y la devolución.
type Service struct{ d Deps }

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Repo == nil || d.Bookings == nil || d.Images == nil || d.Notifier == nil || d.Log == nil {
		return nil, errors.New("handover: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Location == nil {
		loc, err := bookings.Location()
		if err != nil {
			return nil, fmt.Errorf("handover: zona horaria: %w", err)
		}
		d.Location = loc
	}
	return &Service{d: d}, nil
}

// now devuelve la hora en UTC truncada a milisegundos (precisión de MongoDB).
func (s *Service) now() time.Time { return s.d.Now().UTC().Truncate(time.Millisecond) }

// load lee la reserva. Quien no es parte recibe not_found, también el
// administrador: la evidencia es solo de las dos partes (RF-05, CA-09).
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

func checkKind(k domain.Kind) error {
	if !k.Valid() {
		return bookings.ErrNotFound
	}
	return nil
}

// stageView arma la etapa con las URL firmadas de sus fotos (plan P-03).
func (s *Service) stageView(k domain.Kind, h *bookings.Handover, at time.Time) (StageView, error) {
	v := StageView{Kind: k, QR: domain.StateOf(h, at)}
	if h == nil {
		return v, nil
	}
	v.Stage = *h
	for _, p := range h.Photos {
		u, err := s.d.Images.URLs(p)
		if err != nil {
			return StageView{}, fmt.Errorf("handover: URL de imagen: %w", err)
		}
		v.URLs = append(v.URLs, u)
	}
	if h.QR != nil && v.QR != domain.QRNone {
		exp := h.QR.ExpiresAt
		v.QRExpiresAt = &exp
	}
	return v, nil
}

// notifyOther avisa a quien no hizo la acción (RF-12). Un fallo se registra y
// no deshace nada. El evento solo lleva ids (Art. VI.5).
func (s *Service) notifyOther(ctx context.Context, b bookings.Booking, actorID, typ, title, body string) {
	to := b.OwnerID
	if actorID == b.OwnerID {
		to = b.RequesterID
	}
	err := s.d.Notifier.Notify(ctx, to, notify.Event{
		Type: typ, Title: title, Body: body,
		Data: map[string]string{"bookingId": b.ID, "itemId": b.ItemID},
	})
	if err != nil {
		s.d.Log.WarnContext(ctx, "handover: no se pudo notificar", "type", typ, "bookingId", b.ID, "error", err)
	}
}
