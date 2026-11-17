package cloudinary

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/clock"
)

// Errores de construcción de URLs.
var (
	errMissingImageData = errors.New("falta publicId o formato")
	errUnknownDelivery  = errors.New("entrega desconocida")
)

// downloadTTL es la vigencia de una URL de descarga privada (plan P-03).
const downloadTTL = 10 * time.Minute

// Transformaciones de entrega pública (RF-07).
const (
	transformFull  = "c_limit,w_1200"
	transformThumb = "c_fill,w_300,h_300"
)

// URLBuilder arma las URLs para mostrar imágenes (RF-07): públicas con
// transformaciones o de descarga privada firmada con vencimiento.
type URLBuilder struct {
	cloudName string
	apiKey    string
	secret    string
	clock     clock.Clock
	ttl       time.Duration
}

// NewURLBuilder crea un URLBuilder. Devuelve error si falta cloudName, apiKey o
// secret (sin incluir el secreto). Si c es nil usa clock.System{}.
func NewURLBuilder(cloudName, apiKey, secret string, c clock.Clock) (*URLBuilder, error) {
	if err := checkConfig(cloudName, apiKey, secret); err != nil {
		return nil, err
	}
	if c == nil {
		c = clock.System{}
	}
	return &URLBuilder{cloudName: cloudName, apiKey: apiKey, secret: secret, clock: c, ttl: downloadTTL}, nil
}

// URLs devuelve la URL principal y la miniatura de img según su entrega.
// Devuelve error si faltan PublicID o Format o la entrega es desconocida.
func (b *URLBuilder) URLs(img domain.Image) (domain.URLs, error) {
	if img.PublicID == "" || img.Format == "" {
		return domain.URLs{}, fmt.Errorf("cloudinary: urls: %w", errMissingImageData)
	}
	switch img.Delivery {
	case domain.DeliveryPublic:
		return domain.URLs{
			URL:      b.publicURL(transformFull, img),
			ThumbURL: b.publicURL(transformThumb, img),
		}, nil
	case domain.DeliveryAuthenticated:
		u := b.downloadURL(img)
		// La descarga privada (image/download) no aplica transformaciones, así
		// que la miniatura es la misma URL firmada.
		return domain.URLs{URL: u, ThumbURL: u}, nil
	default:
		return domain.URLs{}, fmt.Errorf("cloudinary: urls: entrega %q: %w", img.Delivery, errUnknownDelivery)
	}
}

// publicURL arma https://res.cloudinary.com/<cloud>/image/upload/<t>/<publicId>.<format>.
func (b *URLBuilder) publicURL(transform string, img domain.Image) string {
	return "https://res.cloudinary.com/" + url.PathEscape(b.cloudName) + "/image/upload/" +
		transform + "/" + escapePublicID(img.PublicID) + "." + url.PathEscape(img.Format)
}

// downloadURL arma la URL de descarga privada firmada con vencimiento (P-03).
func (b *URLBuilder) downloadURL(img domain.Image) string {
	now := b.clock.Now()
	params := map[string]string{
		"expires_at": strconv.FormatInt(now.Add(b.ttl).Unix(), 10),
		"format":     img.Format,
		"public_id":  img.PublicID,
		"timestamp":  strconv.FormatInt(now.Unix(), 10),
		"type":       img.Delivery.CloudinaryType(),
	}
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	q.Set("signature", Sign(params, b.secret))
	q.Set("api_key", b.apiKey)
	return "https://api.cloudinary.com/v1_1/" + url.PathEscape(b.cloudName) + "/image/download?" + q.Encode()
}

// escapePublicID escapa cada segmento del publicId conservando las «/».
func escapePublicID(id string) string {
	segs := strings.Split(id, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
