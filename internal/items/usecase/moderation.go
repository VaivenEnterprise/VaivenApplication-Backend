package usecase

import (
	"context"
	"errors"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/port"
)

// Service cumple items.Moderation (spec 011).
var _ port.Moderation = (*Service)(nil)

// blockAttempts acota los reintentos de Block si el dueño edita a la vez.
const blockAttempts = 3

// Block pasa el objeto a blocked (port.Moderation). Si el dueño lo cambia a
// la vez, vuelve a leerlo y reintenta.
func (s *Service) Block(ctx context.Context, itemID string) error {
	for range blockAttempts {
		it, err := s.d.Items.ByID(ctx, itemID)
		if err != nil {
			return err
		}
		switch it.Status {
		case domain.StatusRemoved:
			return domain.ErrNotFound
		case domain.StatusBlocked:
			return nil
		}
		it.Status = domain.StatusBlocked
		it.UpdatedAt = s.now()
		_, err = s.d.Items.Update(ctx, it)
		if !errors.Is(err, domain.ErrVersionConflict) {
			return err
		}
	}
	return domain.ErrVersionConflict
}

// Snapshot devuelve el contenido del objeto (port.Moderation).
func (s *Service) Snapshot(ctx context.Context, itemID string) (port.Snapshot, error) {
	it, err := s.d.Items.ByID(ctx, itemID)
	if err != nil {
		return port.Snapshot{}, err
	}
	snap := port.Snapshot{
		ID: it.ID, OwnerID: it.OwnerID, Title: it.Title,
		Description: it.Description, Status: string(it.Status),
	}
	if it.UsageRules != nil {
		snap.UsageRules = *it.UsageRules
	}
	return snap, nil
}
