package http

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

// ModuleDeps son las dependencias del módulo auth.
type ModuleDeps struct {
	Service Service
	Tokens  TokenParser
	Log     *slog.Logger
	// Límite de register, login y refresh (RF-07): Redis con respaldo local.
	Limiter         ratelimit.Limiter
	LimiterFallback ratelimit.Limiter
	LimitPerMinute  int
	// AccountLoginLimit son los intentos de login por cuenta en
	// AccountLoginWindow (15 min); 0 desactiva el límite por cuenta.
	AccountLoginLimit int
}

// Module registra las rutas de auth, cuenta y usuarios bajo /api/v1 (RF-11).
type Module struct {
	h            handlers
	authenticate func(http.Handler) http.Handler
	limit        func(http.Handler) http.Handler
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Tokens == nil || d.Log == nil || d.Limiter == nil || d.LimitPerMinute <= 0 {
		return nil, errors.New("auth: faltan dependencias del módulo HTTP")
	}
	return &Module{
		h: handlers{svc: d.Service, dec: newDecoder(), log: d.Log, account: accountLimiter{
			primary: d.Limiter, fallback: d.LimiterFallback, limit: d.AccountLoginLimit,
		}},
		authenticate: Authenticate(d.Tokens),
		limit: middleware.RateLimit(d.Limiter, d.LimiterFallback, middleware.RateLimitConfig{
			Limit: d.LimitPerMinute, Window: time.Minute, KeyPrefix: "auth:",
		}, d.Log),
	}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "auth" }

// Authenticate es el middleware que los demás módulos montan en sus rutas
// protegidas (llega por cmd/api/modules.go).
func (m *Module) Authenticate() func(http.Handler) http.Handler { return m.authenticate }

// Register monta las 8 rutas de api/paths/auth.yaml y users.yaml.
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.limit)
		r.Post("/auth/register", m.h.register)
		r.Post("/auth/login", m.h.login)
		r.Post("/auth/refresh", m.h.refresh)
	})
	r.Post("/auth/logout", m.h.logout)

	r.Group(func(r chi.Router) {
		r.Use(m.authenticate)
		r.Get("/me", m.h.getMe)
		r.Patch("/me", m.h.updateMe)
		r.Delete("/me", m.h.deleteMe)
		r.Get("/users/{id}", m.h.getUser)
	})
}

// asDomain indica si err es (o envuelve) un error de dominio.
func asDomain(err error, target **shared.Error) bool {
	return errors.As(err, target)
}
