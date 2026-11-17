package usecase

import (
	"context"
	"errors"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
)

// Close cierra la incidencia con la respuesta final y el resultado (RF-08).
// Solo se cierra una abierta; ambas partes reciben aviso y ven la respuesta
// (RF-10, CA-07). El cierre solo registra la decisión: no cambia la reserva ni
// aplica sanciones. Devuelve la vista de administración.
func (s *Service) Close(ctx context.Context, adminID, id, finalResponse string, outcome domain.Outcome) (View, error) {
	inc, err := s.d.Repo.ByID(ctx, id)
	if err != nil {
		return View{}, err
	}
	if err := domain.CanClose(inc); err != nil {
		return View{}, err
	}
	final, err := domain.NewFinalResponse(finalResponse, outcome, adminID, s.now())
	if err != nil {
		return View{}, err
	}
	updated, err := s.d.Repo.Close(ctx, id, final)
	if err != nil {
		if !errors.Is(err, ErrNoMatch) {
			return View{}, err
		}
		return View{}, s.explainClose(ctx, id)
	}
	s.notifyParties(ctx, updated, EventClosed, "Tu incidencia fue cerrada",
		"El equipo de VAIVÉN cerró la incidencia y dejó una respuesta final.")
	return s.one(ctx, updated, true)
}

func (s *Service) explainClose(ctx context.Context, id string) error {
	fresh, err := s.d.Repo.ByID(ctx, id)
	if err != nil {
		return err
	}
	if ruleErr := domain.CanClose(fresh); ruleErr != nil {
		return ruleErr
	}
	return domain.ErrChanged
}
