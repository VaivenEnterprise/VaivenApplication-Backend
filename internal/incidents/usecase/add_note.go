package usecase

import (
	"context"
	"errors"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
)

// AddNote agrega una nota de adminID a la incidencia (RF-08): máximo 10 y solo
// mientras está abierta. Si es visible para las partes, ambas reciben aviso
// (RF-10). Devuelve la vista de administración.
func (s *Service) AddNote(ctx context.Context, adminID, id, text string, visibleToParties bool) (View, error) {
	inc, err := s.d.Repo.ByID(ctx, id)
	if err != nil {
		return View{}, err
	}
	if err := domain.CanAddNote(inc); err != nil {
		return View{}, err
	}
	note, err := domain.NewNote(text, visibleToParties, adminID, s.now())
	if err != nil {
		return View{}, err
	}
	updated, err := s.d.Repo.AddNote(ctx, id, note)
	if err != nil {
		if !errors.Is(err, ErrNoMatch) {
			return View{}, err
		}
		return View{}, s.explainNote(ctx, id)
	}
	if visibleToParties {
		s.notifyParties(ctx, updated, EventNote, "Hay una nota en tu incidencia",
			"El equipo de VAIVÉN agregó una nota a la incidencia. Ábrela para leerla.")
	}
	return s.one(ctx, updated, true)
}

// explainNote traduce un ErrNoMatch: vuelve a leer la incidencia y aplica la
// misma regla para devolver el error preciso; si ya pasa, otra operación
// cambió la incidencia a la vez.
func (s *Service) explainNote(ctx context.Context, id string) error {
	fresh, err := s.d.Repo.ByID(ctx, id)
	if err != nil {
		return err
	}
	if ruleErr := domain.CanAddNote(fresh); ruleErr != nil {
		return ruleErr
	}
	return domain.ErrChanged
}
