// Package cloudinary habla con la API de Cloudinary con net/http, sin SDK
// (Art. VIII.4; plan P-S3). El API secret solo vive en el backend (Art. VI.7).
package cloudinary

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

// apiBaseURL es la URL de la Admin API de Cloudinary.
const apiBaseURL = "https://api.cloudinary.com"

// PingTimeout es el tiempo máximo de Ping (plan P-S3).
const PingTimeout = 5 * time.Second

// Ping comprueba que Cloudinary responde y que las credenciales son válidas
// con GET /v1_1/<cloud>/ping. Los errores nunca incluyen el secreto.
func Ping(ctx context.Context, cfg config.Cloudinary) error {
	return ping(ctx, http.DefaultClient, apiBaseURL, cfg)
}

func ping(ctx context.Context, client *http.Client, baseURL string, cfg config.Cloudinary) error {
	if cfg.CloudName == "" || cfg.APIKey == "" || cfg.APISecret.Reveal() == "" {
		return fmt.Errorf("cloudinary: faltan credenciales (cloud name, API key o API secret)")
	}

	ctx, cancel := context.WithTimeout(ctx, PingTimeout)
	defer cancel()

	endpoint := baseURL + "/v1_1/" + url.PathEscape(cfg.CloudName) + "/ping"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("cloudinary: crear la petición: %w", err)
	}
	req.SetBasicAuth(cfg.APIKey, cfg.APISecret.Reveal())

	res, err := client.Do(req)
	if err != nil {
		// El error de transporte trae la URL, que no lleva credenciales
		// (van en el encabezado Authorization).
		return fmt.Errorf("cloudinary: ping: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))

	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("cloudinary: credenciales rechazadas (HTTP %d)", res.StatusCode)
	default:
		return fmt.Errorf("cloudinary: respuesta inesperada (HTTP %d)", res.StatusCode)
	}
}
