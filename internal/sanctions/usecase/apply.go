package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/domain"
)

// Service implementa el caso de uso de las sanciones.
type Service struct{ d Deps }

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Repo == nil || d.Incidents == nil || d.Directory == nil || d.Admin == nil || d.Notifier == nil || d.Log == nil {
		return nil, errors.New("sanctions: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}, nil
}

var (
	errUserNotFound     = shared.NewNotFound("No encontramos a esa persona.")
	errIncidentNotFound = shared.NewValidation(domain.MessageInvalidData,
		shared.Detail{Field: "incidentId", Issue: "no existe esa incidencia"})
)

// Apply registra la sanción de adminID a userID (RF-09, P-07):
//
//  1. valida los datos y que la persona y la incidencia existan;
//  2. si es suspensión, llama a users.Admin.Suspend (una advertencia solo
//     notifica); se hace antes de guardar para que un reintento tras un fallo
//     sea seguro (Suspend es idempotente) y nunca quede un registro de una
//     suspensión que no se aplicó;
//  3. guarda el registro y avisa a la persona (RF-10).
//
// Las sanciones nunca modifican las calificaciones. Si ya había una
// suspensión, la fecha de la última sanción es la que rige.
func (s *Service) Apply(ctx context.Context, adminID, userID string, in Input) (domain.Sanction, error) {
	now := s.d.Now().UTC().Truncate(time.Millisecond)
	sanction, err := domain.New(s.d.Repo.NewID(), userID, in.Type, in.Reason, in.EndsAt, in.IncidentID, adminID, now)
	if err != nil {
		return domain.Sanction{}, err
	}
	if err := s.checkReferences(ctx, sanction); err != nil {
		return domain.Sanction{}, err
	}
	if sanction.Type == domain.TypeSuspension {
		if err := s.d.Admin.Suspend(ctx, userID, *sanction.EndsAt); err != nil {
			return domain.Sanction{}, fmt.Errorf("sanctions: suspender cuenta: %w", err)
		}
	}
	saved, err := s.d.Repo.Create(ctx, sanction)
	if err != nil {
		return domain.Sanction{}, err
	}
	s.notifyUser(ctx, saved)
	return saved, nil
}

// checkReferences comprueba que la persona (404) y la incidencia (400, es un
// dato del cuerpo) existan.
func (s *Service) checkReferences(ctx context.Context, sn domain.Sanction) error {
	profiles, err := s.d.Directory.PublicUsers(ctx, []string{sn.UserID})
	if err != nil {
		return fmt.Errorf("sanctions: leer perfil: %w", err)
	}
	if _, ok := profiles[sn.UserID]; !ok {
		return errUserNotFound
	}
	if sn.IncidentID == nil {
		return nil
	}
	ok, err := s.d.Incidents.Exists(ctx, *sn.IncidentID)
	if err != nil {
		return fmt.Errorf("sanctions: buscar incidencia: %w", err)
	}
	if !ok {
		return errIncidentNotFound
	}
	return nil
}

// notifyUser avisa a la persona sancionada. El evento solo lleva ids y el tipo
// (Art. VI.5); el motivo no viaja en el aviso.
func (s *Service) notifyUser(ctx context.Context, sn domain.Sanction) {
	title, body := "Recibiste una advertencia", "El equipo de VAIVÉN registró una advertencia en tu cuenta."
	if sn.Type == domain.TypeSuspension {
		title, body = "Tu cuenta fue suspendida", "El equipo de VAIVÉN suspendió tu cuenta temporalmente. No podrás publicar objetos ni hacer solicitudes hasta que termine."
	}
	data := map[string]string{"sanctionId": sn.ID, "type": string(sn.Type)}
	if sn.IncidentID != nil {
		data["incidentId"] = *sn.IncidentID
	}
	if err := s.d.Notifier.Notify(ctx, sn.UserID, notify.Event{Type: EventApplied, Title: title, Body: body, Data: data}); err != nil {
		s.d.Log.WarnContext(ctx, "sanctions: no se pudo notificar", "type", EventApplied, "sanctionId", sn.ID, "error", err)
	}
}
