// Package port declara images.Service, la interfaz que importan las demás
// specs para usar imágenes (spec 004, plan P-04; Art. XII.4). Solo depende de
// la biblioteca estándar y de internal/images/domain, así que los casos de uso
// de otros módulos lo pueden importar sin romper la regla de dependencia
// (Art. III). La implementación la arma internal/images.New en cmd/api.
//
// Uso típico en el caso de uso de otra spec:
//
//	imgs, err := svc.Resolve(ctx, userID, domain.PurposeItemPhoto, "photos", refs)
//	if err != nil { return err } // validation_failed o service_unavailable
//	item.Photos = imgs            // se guarda []domain.Image en MongoDB
//	u, err := svc.URLs(item.Photos[0]) // url y thumbUrl al responder
package port

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// Service es la fachada de imágenes.
type Service interface {
	// Signature firma la subida directa de una imagen a Cloudinary (RF-01).
	Signature(ctx context.Context, userID string, purpose domain.Purpose) (domain.UploadSignature, error)
	// Resolve valida referencias y devuelve imágenes verificadas (RF-05).
	// field es el nombre del campo de la petición de quien llama (p. ej.
	// "photos"), para los detalles de validation_failed.
	Resolve(ctx context.Context, userID string, purpose domain.Purpose, field string, refs []domain.Ref) ([]domain.Image, error)
	// URLs calcula url y thumbUrl al leer (RF-07).
	URLs(img domain.Image) (domain.URLs, error)
}
