package usecase

import (
	"context"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

// Delete marca el objeto como removed (RF-09; sin borrado físico). Solo el
// dueño; un objeto bloqueado también se puede dar de baja. Con reservas
// aceptadas o en curso responde conflict (CA-07).
func (s *Service) Delete(ctx context.Context, viewer Viewer, id string) error {
	it, err := s.d.Items.ByID(ctx, id)
	if err != nil {
		return err
	}
	if err := it.CheckOwner(viewer.ID); err != nil {
		return err
	}
	busy, err := s.d.Bookings.HasActiveBookings(ctx, it.ID)
	if err != nil {
		return fmt.Errorf("items: consultar reservas: %w", err)
	}
	if busy {
		return domain.ErrHasActiveBookings
	}
	it.Status = domain.StatusRemoved
	it.UpdatedAt = s.now()
	_, err = s.d.Items.Update(ctx, it)
	return err
}
