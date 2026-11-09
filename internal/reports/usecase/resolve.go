package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
)

// Resolve aplica la decisión de adminID a un reporte abierto (RF-10, RF-11):
//   - dismiss: sin falta, solo cierra el reporte;
//   - hide_item: bloquea el objeto (items.Moderation.Block, CA-05) y avisa a su
//     dueño con content_removed;
//   - suspend_user: suspende 7 días (provisional, plan P-07) a la persona
//     reportada, al dueño del objeto o a quien escribió el mensaje, y la avisa
//     con sanction_applied.
//
// La acción se aplica antes de cerrar el reporte; como Block y Suspend son
// idempotentes, un fallo intermedio se puede reintentar. Si dos administradores
// resuelven a la vez, uno recibe 409 (el cierre es condicional).
func (s *Service) Resolve(ctx context.Context, adminID, reportID, action, note string) (View, error) {
	act, err := domain.ParseAction(action)
	if err != nil {
		return View{}, err
	}
	n, err := domain.NormalizeNote(note)
	if err != nil {
		return View{}, err
	}
	rep, err := s.d.Repo.ByID(ctx, reportID)
	if err != nil {
		return View{}, err
	}
	if rep.Status != domain.StatusOpen {
		return View{}, domain.ErrAlreadyResolved
	}
	if err := domain.CheckAction(act, rep.TargetType); err != nil {
		return View{}, err
	}
	switch act {
	case domain.ActionHideItem:
		if err := s.hideItem(ctx, rep); err != nil {
			return View{}, err
		}
	case domain.ActionSuspendUser:
		if err := s.suspendUser(ctx, rep); err != nil {
			return View{}, err
		}
	}
	done, err := s.d.Repo.Resolve(ctx, rep.ID, act.Resolve(), domain.Resolution{Action: act, Note: n, ByAdminID: adminID, At: s.now()})
	if err != nil {
		return View{}, err
	}
	views, err := s.views(ctx, []domain.Report{done})
	if err != nil {
		return View{}, err
	}
	return views[0], nil
}

func (s *Service) hideItem(ctx context.Context, rep domain.Report) error {
	snap, err := s.d.Items.Snapshot(ctx, rep.TargetID)
	if err != nil {
		return mapNotFound(err)
	}
	if err := s.d.Items.Block(ctx, rep.TargetID); err != nil {
		return mapNotFound(err)
	}
	s.notify(ctx, snap.OwnerID, notify.TypeContentRemoved, map[string]string{"itemId": rep.TargetID})
	return nil
}

func (s *Service) suspendUser(ctx context.Context, rep domain.Report) error {
	offender, err := s.offender(ctx, rep)
	if err != nil {
		return err
	}
	until := s.now().Add(domain.SuspensionDays * 24 * time.Hour)
	if err := s.d.Admin.Suspend(ctx, offender, until); err != nil {
		return mapNotFound(err)
	}
	s.notify(ctx, offender, notify.TypeSanctionApplied, nil)
	return nil
}

// offender es la persona a la que apunta el reporte.
func (s *Service) offender(ctx context.Context, rep domain.Report) (string, error) {
	switch rep.TargetType {
	case domain.TargetUser:
		return rep.TargetID, nil
	case domain.TargetItem:
		snap, err := s.d.Items.Snapshot(ctx, rep.TargetID)
		if err != nil {
			return "", mapNotFound(err)
		}
		return snap.OwnerID, nil
	case domain.TargetMessage:
		ref, found, err := s.d.Messages.Reference(ctx, rep.TargetID)
		if err != nil {
			return "", fmt.Errorf("reports: leer mensaje: %w", err)
		}
		if !found {
			return "", domain.ErrTargetNotFound
		}
		return ref.SenderID, nil
	}
	return "", domain.ErrActionNotAllowed
}

// notify avisa a la persona; un fallo se registra y no deshace la decisión.
func (s *Service) notify(ctx context.Context, userID, typ string, data map[string]string) {
	if err := s.d.Notifier.Notify(ctx, userID, notify.Event{Type: typ, Data: data}); err != nil {
		s.d.Log.WarnContext(ctx, "reports: no se pudo enviar el aviso", "type", typ, "error", err)
	}
}
