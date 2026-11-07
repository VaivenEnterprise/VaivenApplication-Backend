// Package images arma la implementación de imágenes con los adaptadores de
// Cloudinary (spec 004, plan P-04). Solo lo importa cmd/api: las demás specs
// dependen de la interfaz port.Service (internal/images/port), que no arrastra
// infraestructura (Art. III). Los tipos viven en internal/images/domain.
package images

import (
	"context"
	"errors"
	"fmt"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/adapter/cloudinary"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/clock"
)

// Service es un alias de port.Service, la interfaz que importan las demás specs.
type Service = port.Service

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

// String oculta el API secret al imprimir la configuración (Art. VI.5).
func (c Config) String() string {
	return fmt.Sprintf("{CloudName:%s APIKey:%s APISecret:[REDACTED] BaseFolder:%s MaxBytes:%d}",
		c.CloudName, c.APIKey, c.BaseFolder, c.MaxBytes)
}

// GoString cubre %#v.
func (c Config) GoString() string { return c.String() }

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
