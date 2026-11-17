package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
)

// Get devuelve una incidencia a una de las partes o a un administrador
// (RF-07); cualquier otra persona recibe 404. Las partes ven solo las notas
// visibles; el administrador, todas, salvo que sea parte de la incidencia: en
// ese caso la ve como cualquier parte, sin las notas internas.
func (s *Service) Get(ctx context.Context, v Viewer, id string) (View, error) {
	inc, err := s.d.Repo.ByID(ctx, id)
	if err != nil {
		return View{}, err
	}
	if !v.Admin && !inc.IsParty(v.ID) {
		return View{}, domain.ErrNotFound
	}
	return s.one(ctx, inc, v.Admin && !inc.IsParty(v.ID))
}
