package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Service implementa los casos de uso de las incidencias.
type Service struct{ d Deps }

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Repo == nil || d.Bookings == nil || d.Images == nil || d.Users == nil || d.Notifier == nil || d.Log == nil {
		return nil, errors.New("incidents: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Location == nil {
		loc, err := bookings.Location()
		if err != nil {
			return nil, fmt.Errorf("incidents: zona horaria: %w", err)
		}
		d.Location = loc
	}
	return &Service{d: d}, nil
}

// now devuelve la hora en UTC truncada a milisegundos (precisión de MongoDB).
func (s *Service) now() time.Time { return s.d.Now().UTC().Truncate(time.Millisecond) }

// views arma las vistas de una lista de incidencias: lee en un solo lote los
// perfiles públicos de ambas partes y firma las URL de la evidencia (P-08).
func (s *Service) views(ctx context.Context, list []domain.Incident, adminView bool) ([]View, error) {
	profiles, err := s.profiles(ctx, list)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(list))
	for _, i := range list {
		v := View{Incident: i, Reporter: profileOf(profiles, i.ReporterID), Reported: profileOf(profiles, i.ReportedID), AdminView: adminView}
		for _, img := range i.Evidence {
			u, err := s.d.Images.URLs(img)
			if err != nil {
				return nil, fmt.Errorf("incidents: URL de evidencia: %w", err)
			}
			v.URLs = append(v.URLs, u)
		}
		v.Notes = i.VisibleNotes()
		if adminView {
			v.Notes = append([]domain.Note{}, i.Notes...)
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) profiles(ctx context.Context, list []domain.Incident) (map[string]users.PublicUser, error) {
	seen := map[string]bool{}
	ids := make([]string, 0, 2*len(list))
	for _, i := range list {
		for _, id := range []string{i.ReporterID, i.ReportedID} {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return map[string]users.PublicUser{}, nil
	}
	m, err := s.d.Users.PublicUsers(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("incidents: leer perfiles: %w", err)
	}
	return m, nil
}

// profileOf devuelve el perfil de id o uno genérico si la cuenta ya no existe.
func profileOf(m map[string]users.PublicUser, id string) users.PublicUser {
	if u, ok := m[id]; ok {
		return u
	}
	return users.PublicUser{ID: id, DisplayName: DeletedName}
}

// one arma la vista de una sola incidencia.
func (s *Service) one(ctx context.Context, i domain.Incident, adminView bool) (View, error) {
	vs, err := s.views(ctx, []domain.Incident{i}, adminView)
	if err != nil {
		return View{}, err
	}
	return vs[0], nil
}

// notify avisa a to; un fallo se registra y no deshace nada (RF-10). El evento
// solo lleva ids (Art. VI.5).
func (s *Service) notify(ctx context.Context, to string, i domain.Incident, typ, title, body string) {
	err := s.d.Notifier.Notify(ctx, to, notify.Event{
		Type: typ, Title: title, Body: body,
		Data: map[string]string{"incidentId": i.ID, "bookingId": i.BookingID},
	})
	if err != nil {
		s.d.Log.WarnContext(ctx, "incidents: no se pudo notificar", "type", typ, "incidentId", i.ID, "error", err)
	}
}

// notifyParties avisa a quien reporta y a la parte reportada.
func (s *Service) notifyParties(ctx context.Context, i domain.Incident, typ, title, body string) {
	s.notify(ctx, i.ReporterID, i, typ, title, body)
	s.notify(ctx, i.ReportedID, i, typ, title, body)
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultPage
	case limit > MaxPage:
		return MaxPage
	}
	return limit
}
