package usecase

import (
	"context"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

// ItemInput son los datos para publicar un objeto (RF-01).
type ItemInput struct {
	Title              string
	Description        string
	Category           string
	Condition          domain.Condition
	Modality           domain.Modality
	DeclaredValueCents int64
	PricePerDayCents   int64
	PriceSource        domain.PriceSource
	UsageRules         *string
	Zone               domain.Zone
	Location           *domain.Location
	Photos             []images.Ref
}

// Create publica un objeto activo de viewer (RF-01 a RF-05). Orden: reglas
// del dominio, moderación y, al final, Cloudinary (la llamada de red más
// cara). La comprobación de cuenta suspendida (RF-12) la hace el middleware
// RequireActive antes de llegar aquí.
func (s *Service) Create(ctx context.Context, viewer Viewer, in ItemInput) (ItemView, error) {
	now := s.now()
	it := domain.Item{
		OwnerID:            viewer.ID,
		Title:              in.Title,
		Description:        in.Description,
		Category:           in.Category,
		Condition:          in.Condition,
		Modality:           in.Modality,
		DeclaredValueCents: in.DeclaredValueCents,
		PricePerDayCents:   in.PricePerDayCents,
		PriceSource:        in.PriceSource,
		UsageRules:         in.UsageRules,
		Zone:               in.Zone,
		Location:           in.Location,
		Photos:             placeholders(len(in.Photos)),
		Status:             domain.StatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
		Version:            1,
	}
	it.Normalize()
	if err := it.Validate(); err != nil {
		return ItemView{}, err
	}
	if err := s.moderate(ctx, textsOf(it)); err != nil {
		return ItemView{}, err
	}
	photos, err := s.resolvePhotos(ctx, viewer.ID, in.Photos)
	if err != nil {
		return ItemView{}, err
	}
	it.Photos = photos
	created, err := s.d.Items.Create(ctx, it)
	if err != nil {
		return ItemView{}, err
	}
	return s.view(ctx, created, viewer)
}

// textsOf devuelve los textos de it que pasan por moderación.
func textsOf(it domain.Item) map[string]string {
	t := map[string]string{"title": it.Title, "description": it.Description}
	if it.UsageRules != nil {
		t["usageRules"] = *it.UsageRules
	}
	return t
}
