package usecase

import (
	"context"
	"errors"
	"fmt"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

const messageInvalidImages = "Revisa las imágenes marcadas."

// Resolve valida las referencias que envió userID y devuelve las imágenes con
// metadatos verificados por Cloudinary (RF-05, POL-13): de 1 a 5, sin
// repetir, de su propia carpeta para el propósito, existentes, en formato
// permitido y de hasta MaxBytes. field es el nombre del campo en la petición
// de quien llama (p. ej. "photos"), para los detalles del error.
//
// Si alguna no cumple devuelve validation_failed con un detalle por imagen; si
// Cloudinary no responde, service_unavailable (RF-08). Nunca devuelve una
// lista parcial.
func (s *Service) Resolve(ctx context.Context, userID string, purpose domain.Purpose, field string, refs []domain.Ref) ([]domain.Image, error) {
	if !purpose.Valid() {
		return nil, domain.ErrInvalidPurpose
	}
	if field == "" {
		field = "images"
	}
	if len(refs) < domain.MinImages || len(refs) > domain.MaxImages {
		return nil, shared.NewValidation(messageInvalidImages,
			shared.Detail{Field: field, Issue: fmt.Sprintf("envía de %d a %d imágenes", domain.MinImages, domain.MaxImages)})
	}

	var details []shared.Detail
	seen := make(map[string]bool, len(refs))
	for i, ref := range refs {
		f := fmt.Sprintf("%s[%d].publicId", field, i)
		switch {
		case !domain.OwnedBy(ref.PublicID, s.d.BaseFolder, purpose, userID):
			details = append(details, shared.Detail{Field: f, Issue: "la imagen no es tuya o no corresponde a este uso"})
		case seen[ref.PublicID]:
			details = append(details, shared.Detail{Field: f, Issue: "imagen repetida"})
		}
		seen[ref.PublicID] = true
	}
	if len(details) > 0 {
		return nil, shared.NewValidation(messageInvalidImages, details...)
	}

	delivery := purpose.Delivery()
	images := make([]domain.Image, 0, len(refs))
	for i, ref := range refs {
		f := fmt.Sprintf("%s[%d].publicId", field, i)
		info, err := s.d.Resources.Lookup(ctx, delivery.CloudinaryType(), ref.PublicID)
		switch {
		case errors.Is(err, ErrResourceNotFound):
			details = append(details, shared.Detail{Field: f, Issue: "la imagen no se subió o ya no existe"})
			continue
		case err != nil:
			var de *shared.Error
			if errors.As(err, &de) && de.Code == shared.CodeUnavailable {
				return nil, domain.ErrUnavailable.Wrap(err)
			}
			return nil, fmt.Errorf("images: consultar %s: %w", f, err)
		case !domain.AllowedFormat(info.Format):
			details = append(details, shared.Detail{Field: f, Issue: "formato no permitido (jpg, jpeg, png o webp)"})
			continue
		case info.Bytes <= 0 || info.Bytes > s.d.MaxBytes:
			details = append(details, shared.Detail{Field: f, Issue: fmt.Sprintf("la imagen pesa más de %d MB", s.d.MaxBytes/(1024*1024))})
			continue
		}
		images = append(images, domain.Image{
			PublicID: ref.PublicID,
			Width:    info.Width,
			Height:   info.Height,
			Format:   info.Format,
			Bytes:    info.Bytes,
			Delivery: delivery,
		})
	}
	if len(details) > 0 {
		return nil, shared.NewValidation(messageInvalidImages, details...)
	}
	return images, nil
}
