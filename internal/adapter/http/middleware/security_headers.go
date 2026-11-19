package middleware

import (
	"net/http"
	"strconv"
	"time"
)

// SecurityHeadersConfig configura las cabeceras de seguridad (spec 012-T003,
// RF-04; spec 018, C-01).
type SecurityHeadersConfig struct {
	// HSTSMaxAge activa Strict-Transport-Security con esa vigencia. Cero no
	// la envía. Solo se activa en staging, que sirve HTTPS con certificado
	// válido; nunca por HTTP ni en producción sin autorización (018, B-8).
	// Sin includeSubDomains y sin preload (018, P-5).
	HSTSMaxAge time.Duration
}

// HSTSStagingMaxAge es la vigencia corta de HSTS en staging (un día).
const HSTSStagingMaxAge = 24 * time.Hour

// apiCSP no permite cargar nada ni enmarcar la respuesta: la API solo
// devuelve JSON. /docs (Swagger UI) pone su propia política.
const apiCSP = "default-src 'none'; frame-ancestors 'none'"

// SecurityHeaders pone en toda respuesta:
//   - X-Content-Type-Options: nosniff (no interpretar el contenido como otro tipo);
//   - X-Frame-Options: DENY y CSP frame-ancestors 'none' (no cargar en marcos);
//   - Referrer-Policy: no-referrer (no filtrar la página de origen);
//   - Cache-Control: no-store si la petición viene autenticada (datos
//     privados fuera de cachés intermedias);
//   - Strict-Transport-Security si está configurada y la conexión es TLS.
//
// Son valores por omisión: un handler puede reemplazarlos con Set (por
// ejemplo, /docs y su CSP), así ninguna cabecera queda duplicada.
func SecurityHeaders(cfg SecurityHeadersConfig) func(http.Handler) http.Handler {
	hsts := ""
	if cfg.HSTSMaxAge > 0 {
		hsts = "max-age=" + strconv.FormatInt(int64(cfg.HSTSMaxAge/time.Second), 10)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy", apiCSP)
			if r.Header.Get("Authorization") != "" {
				h.Set("Cache-Control", "no-store")
			}
			if hsts != "" && r.TLS != nil {
				h.Set("Strict-Transport-Security", hsts)
			}
			next.ServeHTTP(w, r)
		})
	}
}
