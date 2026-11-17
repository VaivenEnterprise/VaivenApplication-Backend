package usecase

import (
	"context"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
)

// Open abre una incidencia de userID contra la otra parte de la reserva (RF-05).
// Orden de las respuestas: quien no es parte recibe 404 sin más validación;
// luego los datos (400), después las reglas de la reserva (409) y por último la
// evidencia (que consulta a Cloudinary). Abrirla no cambia la reserva (RF-06,
// CA-04). Avisa a la parte reportada (RF-10).
func (s *Service) Open(ctx context.Context, userID, bookingID string, in OpenInput) (View, error) {
	b, err := s.d.Bookings.ByID(ctx, bookingID)
	if err != nil {
		return View{}, err
	}
	if _, ok := b.RoleOf(userID); !ok {
		return View{}, bookings.ErrNotFound
	}
	desc, err := domain.ValidateInput(in.Reason, in.Description, len(in.Evidence))
	if err != nil {
		return View{}, err
	}
	open, err := s.d.Repo.CountOpen(ctx, b.ID)
	if err != nil {
		return View{}, err
	}
	now := s.now()
	if err := domain.CanOpen(b, userID, in.Reason, open, now, s.d.Location); err != nil {
		return View{}, err
	}
	var evidence []images.Image
	if len(in.Evidence) > 0 {
		evidence, err = s.d.Images.Resolve(ctx, userID, images.PurposeEvidence, "evidence", in.Evidence)
		if err != nil {
			return View{}, err
		}
	}
	inc, err := domain.New(s.d.Repo.NewID(), b, userID, in.Reason, desc, evidence, now)
	if err != nil {
		return View{}, err
	}
	saved, err := s.d.Repo.Create(ctx, inc)
	if err != nil {
		return View{}, err
	}
	s.notify(ctx, saved.ReportedID, saved, EventOpened, "Reportaron una incidencia",
		"Se abrió una incidencia en una de tus reservas. Revísala en tus incidencias.")
	return s.one(ctx, saved, false)
}
