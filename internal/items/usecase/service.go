package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Deps son las dependencias del Service.
type Deps struct {
	Items      Repository
	Images     imgport.Service
	Moderation moderation.Checker
	Users      users.Directory
	Bookings   BookingChecker   // nil = NoActiveBookings
	Now        func() time.Time // nil = time.Now
}

// Service implementa los casos de uso de objetos.
type Service struct {
	d   Deps
	now clockFunc
}

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Items == nil || d.Images == nil || d.Moderation == nil || d.Users == nil {
		return nil, errors.New("items: faltan dependencias del servicio")
	}
	if d.Bookings == nil {
		d.Bookings = NoActiveBookings{}
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &Service{d: d, now: func() time.Time { return now().UTC().Truncate(time.Millisecond) }}, nil
}

// moderate revisa los textos con moderation.Checker (RF-05, plan P-08). Si
// alguno se rechaza devuelve validation_failed con el campo y el motivo.
func (s *Service) moderate(ctx context.Context, texts map[string]string) error {
	var details []shared.Detail
	for _, field := range []string{"title", "description", "usageRules"} {
		text, ok := texts[field]
		if !ok || text == "" {
			continue
		}
		res, err := s.d.Moderation.Check(ctx, text)
		if err != nil {
			return fmt.Errorf("items: moderación: %w", err)
		}
		if !res.Allowed {
			reason := res.Reason
			if reason == "" {
				reason = "contiene contenido no permitido"
			}
			details = append(details, shared.Detail{Field: field, Issue: reason})
		}
	}
	if len(details) > 0 {
		return shared.NewValidation("El texto no cumple las reglas de contenido.", details...)
	}
	return nil
}

// resolvePhotos verifica las fotos con Cloudinary (plan P-06).
func (s *Service) resolvePhotos(ctx context.Context, ownerID string, refs []images.Ref) ([]images.Image, error) {
	return s.d.Images.Resolve(ctx, ownerID, images.PurposeItemPhoto, "photos", refs)
}

// view arma la respuesta de un objeto para viewer.
func (s *Service) view(ctx context.Context, it domain.Item, viewer Viewer) (ItemView, error) {
	owners, err := s.d.Users.PublicUsers(ctx, []string{it.OwnerID})
	if err != nil {
		return ItemView{}, fmt.Errorf("items: perfil del dueño: %w", err)
	}
	return s.viewWith(it, viewer, owners)
}

func (s *Service) viewWith(it domain.Item, viewer Viewer, owners map[string]users.PublicUser) (ItemView, error) {
	owner, ok := owners[it.OwnerID]
	if !ok {
		owner = users.PublicUser{ID: it.OwnerID, DisplayName: DeletedOwnerName}
	}
	urls := make([]images.URLs, 0, len(it.Photos))
	for _, p := range it.Photos {
		u, err := s.d.Images.URLs(p)
		if err != nil {
			return ItemView{}, fmt.Errorf("items: url de foto: %w", err)
		}
		urls = append(urls, u)
	}
	return ItemView{
		Item:              it,
		Owner:             owner,
		PhotoURLs:         urls,
		ShowDeclaredValue: it.ShowsDeclaredValue(viewer.ID, viewer.Admin),
	}, nil
}

// placeholders devuelve n imágenes vacías para validar el número de fotos
// antes de llamar a Cloudinary.
func placeholders(n int) []images.Image { return make([]images.Image, n) }
