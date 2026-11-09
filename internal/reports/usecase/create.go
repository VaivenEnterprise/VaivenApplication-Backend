package usecase

import (
	"context"
	"errors"
	"fmt"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
)

// Input es lo que envía quien reporta (RF-08).
type Input struct {
	TargetType string
	TargetID   string
	Reason     string
	Details    *string
}

// Create registra un reporte de reporterID (RF-08). Lo reportado debe existir:
// un mensaje solo lo reporta quien participa en su conversación y, si no, es
// 404 como si no existiera (CA-06). Nadie reporta su propio contenido (409).
// Repetir el mismo reporte devuelve el existente con created=false.
// moderation_decision (RF-09) no apunta a nada guardado: se acepta tal cual
// para seguimiento manual.
func (s *Service) Create(ctx context.Context, reporterID string, in Input) (rep domain.Report, created bool, err error) {
	tt, err := domain.ParseTargetType(in.TargetType)
	if err != nil {
		return domain.Report{}, false, err
	}
	reason, err := domain.ParseReason(in.Reason)
	if err != nil {
		return domain.Report{}, false, err
	}
	targetID, err := domain.NormalizeTargetID(tt, in.TargetID)
	if err != nil {
		return domain.Report{}, false, err
	}
	details, err := domain.NormalizeDetails(in.Details)
	if err != nil {
		return domain.Report{}, false, err
	}
	if err := s.checkTarget(ctx, reporterID, tt, targetID); err != nil {
		return domain.Report{}, false, err
	}
	return s.d.Repo.Create(ctx, domain.Report{
		ID: s.d.Repo.NewID(), ReporterID: reporterID, TargetType: tt, TargetID: targetID, Reason: reason,
		Details: details, Status: domain.StatusOpen, CreatedAt: s.now(),
	})
}

// checkTarget comprueba que lo reportado existe y que quien reporta puede
// verlo y no es su dueño.
func (s *Service) checkTarget(ctx context.Context, reporterID string, tt domain.TargetType, targetID string) error {
	switch tt {
	case domain.TargetItem:
		snap, err := s.d.Items.Snapshot(ctx, targetID)
		if err != nil {
			return mapNotFound(err)
		}
		if snap.OwnerID == reporterID {
			return domain.ErrOwnTarget
		}
	case domain.TargetMessage:
		ref, found, err := s.d.Messages.Reference(ctx, targetID)
		if err != nil {
			return fmt.Errorf("reports: leer mensaje: %w", err)
		}
		if !found || !ref.HasParticipant(reporterID) {
			return domain.ErrTargetNotFound
		}
		if ref.SenderID == reporterID {
			return domain.ErrOwnTarget
		}
	case domain.TargetUser:
		if targetID == reporterID {
			return domain.ErrOwnTarget
		}
		m, err := s.d.Users.PublicUsers(ctx, []string{targetID})
		if err != nil {
			return fmt.Errorf("reports: leer perfil: %w", err)
		}
		if _, ok := m[targetID]; !ok {
			return domain.ErrTargetNotFound
		}
	}
	return nil
}

// mapNotFound convierte un not_found de otro módulo en ErrTargetNotFound y deja
// pasar los demás errores.
func mapNotFound(err error) error {
	var de *shared.Error
	if errors.As(err, &de) && de.Code == shared.CodeNotFound {
		return domain.ErrTargetNotFound
	}
	return err
}
