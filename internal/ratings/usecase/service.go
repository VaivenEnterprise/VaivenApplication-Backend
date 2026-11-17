package usecase

import (
	"errors"
	"time"
)

// Service implementa los casos de uso de las calificaciones.
type Service struct{ d Deps }

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Repo == nil || d.Bookings == nil || d.Users == nil || d.Notifier == nil || d.Log == nil {
		return nil, errors.New("ratings: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}, nil
}

// now devuelve la hora en UTC truncada a milisegundos (precisión de MongoDB).
func (s *Service) now() time.Time { return s.d.Now().UTC().Truncate(time.Millisecond) }
