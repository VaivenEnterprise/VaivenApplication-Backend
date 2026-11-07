// Package domain define las imágenes de VAIVÉN (spec 004): propósito, tipo de
// entrega, la referencia guardada en MongoDB y las reglas de POL-13. Solo usa
// la biblioteca estándar y los errores compartidos (Art. III).
package domain

import (
	"strings"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Purpose es para qué se sube una imagen (RF-01).
type Purpose string

// Propósitos válidos.
const (
	PurposeItemPhoto Purpose = "item_photo"
	PurposeEvidence  Purpose = "evidence"
)

// Valid indica si el propósito existe.
func (p Purpose) Valid() bool {
	return p == PurposeItemPhoto || p == PurposeEvidence
}

// Delivery indica cómo se entrega la imagen (Art. 6.10 de la constitución).
type Delivery string

// Tipos de entrega.
const (
	DeliveryPublic        Delivery = "public"
	DeliveryAuthenticated Delivery = "authenticated"
)

// Delivery devuelve la entrega de cada propósito (plan P-07): las fotos de
// objetos son públicas y las evidencias, privadas.
func (p Purpose) Delivery() Delivery {
	if p == PurposeEvidence {
		return DeliveryAuthenticated
	}
	return DeliveryPublic
}

// CloudinaryType es el «type» de Cloudinary de la entrega: "upload" (pública) o
// "authenticated" (privada, RF-02).
func (d Delivery) CloudinaryType() string {
	if d == DeliveryAuthenticated {
		return "authenticated"
	}
	return "upload"
}

// Límites de POL-13 y RF-05.
const (
	MinImages = 1
	MaxImages = 5
	// DefaultMaxBytes es el tamaño máximo por imagen (5 MB); configurable con
	// IMAGE_MAX_BYTES.
	DefaultMaxBytes int64 = 5 * 1024 * 1024
)

// AllowedFormats son los formatos permitidos, en el orden que firma Cloudinary
// (allowed_formats, RF-03).
const AllowedFormats = "jpg,jpeg,png,webp"

// AllowedFormat indica si format (de Cloudinary, en minúsculas) está permitido.
func AllowedFormat(format string) bool {
	for _, f := range strings.Split(AllowedFormats, ",") {
		if format == f {
			return true
		}
	}
	return false
}

// Image es la referencia verificada que se guarda en MongoDB (RF-06). Sus
// metadatos vienen de Cloudinary, nunca del cliente.
type Image struct {
	PublicID string
	Width    int
	Height   int
	Format   string
	Bytes    int64
	Delivery Delivery
}

// Ref es la referencia que envía el cliente: solo el publicId (RF-05).
type Ref struct {
	PublicID string
}

// URLs son las direcciones para mostrar una imagen (RF-07).
type URLs struct {
	URL      string
	ThumbURL string
}

// UploadSignature es lo que necesita el cliente para subir una imagen
// directamente a Cloudinary (RF-01). No incluye el API secret (RNF-01).
type UploadSignature struct {
	UploadURL      string
	APIKey         string
	Timestamp      int64
	Signature      string
	PublicID       string
	Type           string
	AllowedFormats string
	// Overwrite va firmado en "false": nadie puede reemplazar la imagen
	// después de verificarla (firma de un solo uso, Art. VI.7).
	Overwrite string
}

// FolderPrefix es la carpeta de una persona para un propósito:
// <base>/<purpose>/<userId>/ (RF-03).
func FolderPrefix(base string, p Purpose, userID string) string {
	return strings.TrimSuffix(base, "/") + "/" + string(p) + "/" + userID + "/"
}

// NewPublicID arma <base>/<purpose>/<userId>/<id> (RF-03).
func NewPublicID(base string, p Purpose, userID, id string) string {
	return FolderPrefix(base, p, userID) + id
}

// OwnedBy indica si publicID está en la carpeta de userID para el propósito
// (Art. 6.10): mismo prefijo y un solo segmento final, sin «..» ni barras.
func OwnedBy(publicID, base string, p Purpose, userID string) bool {
	if userID == "" || strings.Contains(userID, "/") {
		return false
	}
	prefix := FolderPrefix(base, p, userID)
	rest, ok := strings.CutPrefix(publicID, prefix)
	return ok && rest != "" && rest != "." && rest != ".." && !strings.ContainsAny(rest, "/\\")
}

// Errores de imágenes (mensajes en es-MX).
var (
	// ErrInvalidPurpose: propósito desconocido.
	ErrInvalidPurpose = shared.NewValidation("Revisa los datos marcados.",
		shared.Detail{Field: "purpose", Issue: "debe ser item_photo o evidence"})
	// ErrUnavailable: Cloudinary no responde; no se guardó nada (RF-08).
	ErrUnavailable = shared.NewUnavailable("No pudimos procesar las imágenes por ahora. Intenta de nuevo en unos minutos.")
)
