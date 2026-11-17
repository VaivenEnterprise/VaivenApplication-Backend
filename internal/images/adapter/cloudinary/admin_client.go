package cloudinary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// adminDefaultBaseURL es la URL de la Admin API de Cloudinary.
const adminDefaultBaseURL = "https://api.cloudinary.com"

// AdminTimeout es el tiempo máximo de cada consulta a la Admin API (T004).
const AdminTimeout = 5 * time.Second

// adminMaxBody limita lo que se lee de una respuesta de Cloudinary (1 MiB).
const adminMaxBody = 1 << 20

// ErrResourceNotFound indica que Cloudinary no tiene la imagen (HTTP 404).
var ErrResourceNotFound = errors.New("cloudinary: la imagen no existe")

// Resource son los metadatos de una imagen según Cloudinary (RF-05, P-02).
type Resource struct {
	PublicID string
	Type     string
	Format   string
	Width    int
	Height   int
	Bytes    int64
}

// AdminClient consulta la Admin API de Cloudinary con net/http y autenticación
// básica. El API secret nunca aparece en errores ni al imprimir el cliente.
type AdminClient struct {
	baseURL    string
	cloudName  string
	apiKey     string
	secret     string
	httpClient *http.Client
}

// AdminOption ajusta un AdminClient al crearlo.
type AdminOption func(*AdminClient)

// WithAdminBaseURL cambia la URL base de la Admin API (para pruebas con
// httptest). Por defecto es https://api.cloudinary.com.
func WithAdminBaseURL(u string) AdminOption {
	return func(c *AdminClient) {
		c.baseURL = strings.TrimRight(u, "/")
	}
}

// WithAdminHTTPClient cambia el cliente HTTP. Por defecto se usa uno con
// Timeout de 5 segundos.
func WithAdminHTTPClient(h *http.Client) AdminOption {
	return func(c *AdminClient) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// NewAdminClient crea el cliente de la Admin API. Devuelve error si falta el
// cloud name, la API key o el API secret; el error nunca incluye el secreto.
func NewAdminClient(cloudName, apiKey, secret string, opts ...AdminOption) (*AdminClient, error) {
	if cloudName == "" || apiKey == "" || secret == "" {
		return nil, errors.New("cloudinary: faltan credenciales (cloud name, API key o API secret)")
	}
	c := &AdminClient{
		baseURL:    adminDefaultBaseURL,
		cloudName:  cloudName,
		apiKey:     apiKey,
		secret:     secret,
		httpClient: &http.Client{Timeout: AdminTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// String describe el cliente sin revelar el API secret.
func (c *AdminClient) String() string {
	return fmt.Sprintf("cloudinary.AdminClient{cloud: %q, apiKey: %q, secret: [REDACTED]}", c.cloudName, c.apiKey)
}

// GoString describe el cliente con %#v sin revelar el API secret.
func (c *AdminClient) GoString() string {
	return c.String()
}

// adminResourceJSON es la parte de la respuesta de Cloudinary que se usa.
type adminResourceJSON struct {
	PublicID string `json:"public_id"`
	Type     string `json:"type"`
	Format   string `json:"format"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    int64  `json:"bytes"`
}

// GetResource consulta GET /v1_1/<cloud>/resources/image/<deliveryType>/<publicID>
// (P-02). deliveryType debe ser "upload" o "authenticated". Devuelve
// ErrResourceNotFound si la imagen no existe y un error que envuelve
// domain.ErrUnavailable (service_unavailable, RF-08) si Cloudinary falla, no
// responde a tiempo o rechaza las credenciales.
func (c *AdminClient) GetResource(ctx context.Context, deliveryType, publicID string) (Resource, error) {
	if deliveryType != "upload" && deliveryType != "authenticated" {
		return Resource{}, fmt.Errorf("cloudinary: tipo de entrega inválido %q", deliveryType)
	}
	escaped, err := adminEscapePublicID(publicID)
	if err != nil {
		return Resource{}, err
	}

	endpoint := c.baseURL + "/v1_1/" + url.PathEscape(c.cloudName) +
		"/resources/image/" + deliveryType + "/" + escaped
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Resource{}, fmt.Errorf("cloudinary: crear la petición: %w", err)
	}
	req.SetBasicAuth(c.apiKey, c.secret)
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		// El error de transporte trae la URL, que no lleva credenciales (van
		// en el encabezado Authorization).
		return Resource{}, fmt.Errorf("cloudinary: consultar la imagen: %w: %w", domain.ErrUnavailable, err)
	}
	defer res.Body.Close()
	body := io.LimitReader(res.Body, adminMaxBody)

	switch {
	case res.StatusCode == http.StatusOK:
	case res.StatusCode == http.StatusNotFound:
		return Resource{}, ErrResourceNotFound
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return Resource{}, fmt.Errorf("cloudinary: credenciales rechazadas (HTTP %d): %w", res.StatusCode, domain.ErrUnavailable)
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
		return Resource{}, fmt.Errorf("cloudinary: servicio no disponible (HTTP %d): %w", res.StatusCode, domain.ErrUnavailable)
	default:
		return Resource{}, fmt.Errorf("cloudinary: respuesta inesperada (HTTP %d)", res.StatusCode)
	}

	var out adminResourceJSON
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		return Resource{}, fmt.Errorf("cloudinary: respuesta inválida: %w: %w", domain.ErrUnavailable, err)
	}
	return Resource(out), nil
}

// adminEscapePublicID escapa cada segmento del publicID con url.PathEscape y
// conserva las barras. Rechaza segmentos vacíos, «.» y «..» para que la ruta
// no cambie de recurso.
func adminEscapePublicID(publicID string) (string, error) {
	if publicID == "" {
		return "", errors.New("cloudinary: falta el public_id")
	}
	segments := strings.Split(publicID, "/")
	for i, s := range segments {
		if s == "" || s == "." || s == ".." {
			return "", fmt.Errorf("cloudinary: public_id inválido %q", publicID)
		}
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/"), nil
}
