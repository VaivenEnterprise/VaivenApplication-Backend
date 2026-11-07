// Package images es la única puerta a las imágenes para el resto de la
// plataforma (spec 004, plan P-04; Art. XII.4). Otras specs reciben
// referencias { "publicId": "…" } y las validan con Resolve antes de guardar,
// y calculan las URL al leer con URLs:
//
//	imgs, err := svc.Resolve(ctx, userID, domain.PurposeItemPhoto, "photos", refs)
//	if err != nil { return err } // validation_failed o service_unavailable
//	item.Photos = imgs            // se guarda []domain.Image en MongoDB
//	...
//	u, err := svc.URLs(item.Photos[0]) // url y thumbUrl para la respuesta
//
// Los tipos (Image, Ref, Purpose, URLs) viven en internal/images/domain.
package images

import (
	"context"
	"errors"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/adapter/cloudinary"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/clock"
)

// Service es la fachada de imágenes que importan las demás specs.
type Service interface {
	// Signature firma la subida directa de una imagen a Cloudinary (RF-01).
	Signature(ctx context.Context, userID string, purpose domain.Purpose) (domain.UploadSignature, error)
	// Resolve valida referencias y devuelve imágenes verificadas (RF-05).
	// field es el nombre del campo de la petición, para los detalles del error.
	Resolve(ctx context.Context, userID string, purpose domain.Purpose, field string, refs []domain.Ref) ([]domain.Image, error)
	// URLs calcula url y thumbUrl al leer (RF-07).
	URLs(img domain.Image) (domain.URLs, error)
}

// Config es la configuración de Cloudinary y de los límites (plan §5).
type Config struct {
	CloudName  string
	APIKey     string
	APISecret  string
	BaseFolder string
	MaxBytes   int64
	Clock      clock.Clock // nil = reloj del sistema
	// AdminOptions permite apuntar el cliente a un servidor de pruebas.
	AdminOptions []cloudinary.AdminOption
}

// New arma el servicio con los adaptadores de Cloudinary.
func New(cfg Config) (Service, error) {
	if cfg.Clock == nil {
		cfg.Clock = clock.System{}
	}
	signer, err := cloudinary.NewSigner(cfg.CloudName, cfg.APIKey, cfg.APISecret)
	if err != nil {
		return nil, err
	}
	admin, err := cloudinary.NewAdminClient(cfg.CloudName, cfg.APIKey, cfg.APISecret, cfg.AdminOptions...)
	if err != nil {
		return nil, err
	}
	urls, err := cloudinary.NewURLBuilder(cfg.CloudName, cfg.APIKey, cfg.APISecret, cfg.Clock)
	if err != nil {
		return nil, err
	}
	return usecase.NewService(usecase.Deps{
		Signer:     signer,
		Resources:  resources{admin: admin},
		URLs:       urls,
		BaseFolder: cfg.BaseFolder,
		MaxBytes:   cfg.MaxBytes,
		Now:        cfg.Clock.Now,
	})
}

// resources adapta el cliente de administración al puerto usecase.Resources.
type resources struct {
	admin *cloudinary.AdminClient
}

func (r resources) Lookup(ctx context.Context, cloudinaryType, publicID string) (usecase.ResourceInfo, error) {
	res, err := r.admin.GetResource(ctx, cloudinaryType, publicID)
	if errors.Is(err, cloudinary.ErrResourceNotFound) {
		return usecase.ResourceInfo{}, usecase.ErrResourceNotFound
	}
	if err != nil {
		return usecase.ResourceInfo{}, fmt.Errorf("images: %w", err)
	}
	return usecase.ResourceInfo{Format: res.Format, Width: res.Width, Height: res.Height, Bytes: res.Bytes}, nil
}
