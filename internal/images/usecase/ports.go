// Package usecase contiene los casos de uso de imágenes (spec 004): firmar una
// subida, resolver referencias verificándolas con Cloudinary y calcular URLs.
// Declara los puertos que necesita; el adaptador de Cloudinary los implementa
// (Art. III.3).
package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// UploadSigner firma los parámetros de una subida directa a Cloudinary.
type UploadSigner interface {
	UploadURL() string
	APIKey() string
	SignUpload(params map[string]string) string
}

// ResourceInfo son los metadatos que Cloudinary informa de una imagen subida.
type ResourceInfo struct {
	Format string
	Width  int
	Height int
	Bytes  int64
}

// ErrResourceNotFound lo devuelve Resources cuando la imagen no existe.
var ErrResourceNotFound = errors.New("images: la imagen no existe en Cloudinary")

// Resources consulta una imagen en la API de administración de Cloudinary.
// Si Cloudinary no responde devuelve un error que envuelve
// domain.ErrUnavailable.
type Resources interface {
	Lookup(ctx context.Context, cloudinaryType, publicID string) (ResourceInfo, error)
}

// URLBuilder calcula las URL de una imagen (RF-07).
type URLBuilder interface {
	URLs(img domain.Image) (domain.URLs, error)
}

// Deps son las dependencias del Service.
type Deps struct {
	Signer     UploadSigner
	Resources  Resources
	URLs       URLBuilder
	BaseFolder string // p. ej. vaiven/staging
	MaxBytes   int64  // 0 = domain.DefaultMaxBytes
	Now        func() time.Time
	NewID      func() (string, error) // nil = 16 bytes aleatorios en hex
}
