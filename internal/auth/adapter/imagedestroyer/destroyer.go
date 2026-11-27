// Package imagedestroyer adapta el puerto compartido images/port.Destroyer al
// puerto usecase.ImageDestroyer de la baja de cuenta (spec 021, D-2): el job
// «image» de la bandeja de salida borra la foto en Cloudinary con reintento.
// Lo comparten cmd/api y cmd/admin.
package imagedestroyer

import (
	"context"
	"errors"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
)

// defaultDelivery es el tipo de Cloudinary cuando el job no lo trae.
const defaultDelivery = "upload"

// Destroyer implementa usecase.ImageDestroyer sobre imgport.Destroyer.
type Destroyer struct{ d imgport.Destroyer }

// New crea el adaptador. Falla si no hay borrador de imágenes.
func New(d imgport.Destroyer) (*Destroyer, error) {
	if d == nil {
		return nil, errors.New("imagedestroyer: falta el borrador de imágenes")
	}
	return &Destroyer{d: d}, nil
}

// Destroy borra la imagen; una que ya no existe cuenta como borrada. Un error se
// devuelve para que el job se reintente.
func (x *Destroyer) Destroy(ctx context.Context, ref usecase.ImageRef) error {
	delivery := ref.Delivery
	if delivery == "" {
		delivery = defaultDelivery
	}
	return x.d.Destroy(ctx, delivery, ref.PublicID)
}

// Comprobación en compilación del puerto de la baja.
var _ usecase.ImageDestroyer = (*Destroyer)(nil)
