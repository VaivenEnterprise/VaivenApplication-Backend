package usecase

import (
	"context"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

// UpdateInput son los cambios de PATCH /items/{id} (RF-07). Un puntero nil no
// cambia el campo. UsageRules y Location se pueden borrar: Set* en true con
// valor nil los deja vacíos. Photos no nil reemplaza la lista completa.
type UpdateInput struct {
	Version            int64
	Title              *string
	Description        *string
	Category           *string
	Condition          *domain.Condition
	Modality           *domain.Modality
	DeclaredValueCents *int64
	PricePerDayCents   *int64
	PriceSource        *domain.PriceSource
	SetUsageRules      bool
	UsageRules         *string
	Zone               *domain.Zone
	SetLocation        bool
	Location           *domain.Location
	Photos             []images.Ref
}

// Update edita un objeto del que viewer es dueño (RF-07, CA-05). Exige la
// versión leída (plan P-05) y que el objeto resultante cumpla las mismas
// reglas que al publicarlo. Solo se moderan los textos que cambian y solo se
// consulta Cloudinary si cambian las fotos. Cambiar el precio no altera
// reservas existentes: viven con su propia instantánea (spec 007).
func (s *Service) Update(ctx context.Context, viewer Viewer, id string, in UpdateInput) (ItemView, error) {
	it, err := s.d.Items.ByID(ctx, id)
	if err != nil {
		return ItemView{}, err
	}
	if err := it.CheckOwnerAction(viewer.ID); err != nil {
		return ItemView{}, err
	}
	if in.Version != it.Version {
		return ItemView{}, domain.ErrVersionConflict
	}
	changedTexts := applyChanges(&it, in)
	if in.Photos != nil {
		it.Photos = placeholders(len(in.Photos))
	}
	it.Normalize()
	if err := it.Validate(); err != nil {
		return ItemView{}, err
	}
	toCheck := map[string]string{}
	all := textsOf(it)
	for _, f := range changedTexts {
		toCheck[f] = all[f]
	}
	if err := s.moderate(ctx, toCheck); err != nil {
		return ItemView{}, err
	}
	if in.Photos != nil {
		photos, err := s.resolvePhotos(ctx, viewer.ID, in.Photos)
		if err != nil {
			return ItemView{}, err
		}
		it.Photos = photos
	}
	it.UpdatedAt = s.now()
	updated, err := s.d.Items.Update(ctx, it)
	if err != nil {
		return ItemView{}, err
	}
	return s.view(ctx, updated, viewer)
}

// applyChanges copia en it los campos presentes y devuelve los textos
// moderables que cambiaron.
func applyChanges(it *domain.Item, in UpdateInput) []string {
	var texts []string
	if in.Title != nil {
		it.Title = *in.Title
		texts = append(texts, "title")
	}
	if in.Description != nil {
		it.Description = *in.Description
		texts = append(texts, "description")
	}
	if in.SetUsageRules {
		it.UsageRules = in.UsageRules
		texts = append(texts, "usageRules")
	}
	setIf(&it.Category, in.Category)
	setIf(&it.Condition, in.Condition)
	setIf(&it.Modality, in.Modality)
	setIf(&it.DeclaredValueCents, in.DeclaredValueCents)
	setIf(&it.PricePerDayCents, in.PricePerDayCents)
	setIf(&it.PriceSource, in.PriceSource)
	setIf(&it.Zone, in.Zone)
	if in.SetLocation {
		it.Location = in.Location
	}
	return texts
}

func setIf[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}
