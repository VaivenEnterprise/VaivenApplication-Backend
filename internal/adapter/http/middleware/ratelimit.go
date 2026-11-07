package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

const (
	codeRateLimited    = "rate_limited"
	messageRateLimited = "Hiciste demasiadas solicitudes. Espera un momento e intenta de nuevo."
)

// RateLimitConfig fija cuántas peticiones se permiten por ventana.
type RateLimitConfig struct {
	Limit  int
	Window time.Duration
	// UserID devuelve el usuario autenticado de la petición; mientras no haya
	// autenticación (spec 003) puede ser nil y el límite es por IP.
	UserID func(r *http.Request) (string, bool)
}

// RateLimit aplica el límite con primary (Redis) y, si falla, con fallback
// (en memoria) para no tumbar la API (Art. XIV.2). El paso a modo degradado y
// la recuperación se registran una sola vez cada uno. Responde 429
// rate_limited con el envoltorio del Art. IV y Retry-After.
func RateLimit(primary, fallback ratelimit.Limiter, cfg RateLimitConfig, log *slog.Logger) func(http.Handler) http.Handler {
	d := &degradation{log: log}
	retryAfter := strconv.Itoa(int((cfg.Window + time.Second - 1) / time.Second))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := limitKey(r, cfg.UserID)

			allowed, err := primary.Allow(r.Context(), key, cfg.Limit, cfg.Window)
			if err != nil {
				d.fail(err)
				allowed = true
				if fallback != nil {
					allowed, err = fallback.Allow(r.Context(), key, cfg.Limit, cfg.Window)
				}
				if err != nil {
					// Sin ningún limitador se deja pasar: el límite protege, no
					// debe cortar el servicio.
					allowed = true
				}
			} else {
				d.recover()
			}

			if !allowed {
				w.Header().Set("Retry-After", retryAfter)
				respond.Failure(w, r, http.StatusTooManyRequests, codeRateLimited, messageRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// limitKey usa el usuario autenticado o, si no hay, la IP de la conexión.
// No se confía en X-Forwarded-For: la API recibe el tráfico directamente (sin
// proxy), así que ese encabezado lo controla el cliente.
func limitKey(r *http.Request, userID func(*http.Request) (string, bool)) string {
	if userID != nil {
		if id, ok := userID(r); ok && id != "" {
			return "user:" + id
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return "ip:" + host
}

// degradation registra una vez la caída de Redis y una vez su recuperación.
type degradation struct {
	mu       sync.Mutex
	degraded bool
	log      *slog.Logger
}

func (d *degradation) fail(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.degraded {
		d.degraded = true
		d.log.Warn("límite de tasa degradado: Redis no responde, se usa el limitador local", "error", err)
	}
}

func (d *degradation) recover() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.degraded {
		d.degraded = false
		d.log.Info("límite de tasa restablecido en Redis")
	}
}
