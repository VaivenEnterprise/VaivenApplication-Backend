// Package http ensambla el router de la API: monta el middleware en el orden
// del plan §4.3 y las rutas del plan §4.4 (spec 002, RF-09).
package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/handler"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

// maxBodyBytes es el límite global del cuerpo de una petición: 1 MiB (Art. VI.6).
const maxBodyBytes = 1 << 20

const (
	messageNotFound         = "No encontramos lo que buscas."
	messageMethodNotAllowed = "Este método no está permitido para esta ruta."
)

// Deps son las dependencias que el router recibe desde main.
type Deps struct {
	Log            *slog.Logger
	AllowedOrigins []string
	Pinger         handler.Pinger // MongoDB
	Redis          handler.Pinger // nil si no hay Redis configurado
	Meta           handler.MetaInfo

	// Límite de tasa (Art. XIV.2). Sin Limiter no se monta el middleware.
	Limiter         ratelimit.Limiter
	LimiterFallback ratelimit.Limiter
	RateLimit       middleware.RateLimitConfig
}

// NewRouter devuelve el handler raíz de la API.
//
// Orden de ejecución (plan §4.3): RequestID, Logging, Recover, CORS, límite de
// cuerpo y, por último, los handlers. Las rutas inexistentes (404) y los
// métodos no permitidos (405) también pasan por el middleware y usan el
// envoltorio único de error. Como el Art. IV no tiene un código propio para
// 405, usa not_found.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(
		middleware.RequestID,
		middleware.Logging(d.Log),
		middleware.Recover(d.Log),
		middleware.CORS(d.AllowedOrigins),
	)
	if d.Limiter != nil {
		r.Use(middleware.RateLimit(d.Limiter, d.LimiterFallback, d.RateLimit, d.Log))
	}
	r.Use(limitBody)

	r.Get("/healthz", handler.Healthz)
	r.Get("/readyz", handler.Readyz(d.Pinger, d.Redis))
	r.Get("/api/v1/meta", handler.Meta(d.Meta))

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		respond.Failure(w, req, http.StatusNotFound, string(domain.CodeNotFound), messageNotFound)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		respond.Failure(w, req, http.StatusMethodNotAllowed, string(domain.CodeNotFound), messageMethodNotAllowed)
	})

	return r
}

// limitBody limita el cuerpo de la petición a maxBodyBytes. Leer más allá del
// límite devuelve *http.MaxBytesError al handler.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}
