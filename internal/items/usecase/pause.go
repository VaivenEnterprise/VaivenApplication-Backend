package usecase

import "context"

// Pause pasa un objeto de active a paused (RF-08): no admite solicitudes
// nuevas y conserva los compromisos. Pausar uno ya pausado no cambia nada.
func (s *Service) Pause(ctx context.Context, viewer Viewer, id string) (ItemView, error) {
	return s.setPaused(ctx, viewer, id, true)
}

// Resume pasa un objeto de paused a active (RF-08). Reanudar uno activo no
// cambia nada; uno bloqueado por moderación responde conflict.
func (s *Service) Resume(ctx context.Context, viewer Viewer, id string) (ItemView, error) {
	return s.setPaused(ctx, viewer, id, false)
}

func (s *Service) setPaused(ctx context.Context, viewer Viewer, id string, pause bool) (ItemView, error) {
	it, err := s.d.Items.ByID(ctx, id)
	if err != nil {
		return ItemView{}, err
	}
	if err := it.CheckOwnerAction(viewer.ID); err != nil {
		return ItemView{}, err
	}
	target, changed := it.WithStatus(pause)
	if changed {
		it.Status = target
		it.UpdatedAt = s.now()
		if it, err = s.d.Items.Update(ctx, it); err != nil {
			return ItemView{}, err
		}
	}
	return s.view(ctx, it, viewer)
}
