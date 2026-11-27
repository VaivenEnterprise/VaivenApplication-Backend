// Package http es el adaptador HTTP de las sugerencias con IA (spec 014,
// T006): POST /api/v1/ai/listing-suggestions (api/paths/ai.yaml).
//
// Privacidad (RF-07): los registros nunca incluyen la foto, su publicId ni el
// texto sugerido; solo el error interno cuando no es de dominio.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/suggest/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

// perMinute es el límite por persona y minuto de la ruta, además del diario.
const perMinute = 10

const messageDailyLimit = "Llegaste al límite diario de sugerencias. Intenta mañana o escribe el título tú mismo."

// suggestionRequest se valida en la frontera con validator/v10 (Art. VI.2).
type suggestionRequest struct {
	ImagePublicID string `json:"imagePublicId" validate:"required,max=255"`
}

type suggestionResponse struct {
	Title       string `json:"title"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
}

// Suggester es lo que el handler necesita del caso de uso.
type Suggester interface {
	Suggest(ctx context.Context, userID, publicID string) (aidomain.Suggestion, error)
}

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service         Suggester
	Authenticate    func(http.Handler) http.Handler
	Log             *slog.Logger
	Limiter         ratelimit.Limiter
	LimiterFallback ratelimit.Limiter
	// Now sirve para calcular Retry-After del límite diario; nil = time.Now.
	Now func() time.Time
	// Location es la zona del «día» del límite; nil = UTC.
	Location *time.Location
}

// Module registra las rutas de sugerencias bajo /api/v1 (Art. XII.3).
type Module struct {
	svc   Suggester
	log   *slog.Logger
	auth  func(http.Handler) http.Handler
	limit func(http.Handler) http.Handler
	v     *validator.Validate
	now   func() time.Time
	loc   *time.Location
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.Log == nil || d.Limiter == nil {
		return nil, errors.New("suggest: faltan dependencias del módulo HTTP")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Location == nil {
		d.Location = time.UTC
	}
	return &Module{
		svc:  d.Service,
		log:  d.Log,
		auth: d.Authenticate,
		v:    validator.New(validator.WithRequiredStructEnabled()),
		now:  d.Now,
		loc:  d.Location,
		limit: middleware.RateLimit(d.Limiter, d.LimiterFallback, middleware.RateLimitConfig{
			Limit:     perMinute,
			Window:    time.Minute,
			KeyPrefix: "ai:suggest:",
			UserID:    func(r *http.Request) (string, bool) { return auth.UserID(r.Context()) },
		}, d.Log),
	}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "suggest" }

// Register monta POST /ai/listing-suggestions con sesión y límite por persona.
func (m *Module) Register(r chi.Router) {
	r.With(m.auth, m.limit).Post("/ai/listing-suggestions", m.suggest)
}

func (m *Module) suggest(w http.ResponseWriter, r *http.Request) {
	var req suggestionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // additionalProperties: false en el contrato
	if err := dec.Decode(&req); err != nil || dec.More() {
		respond.Error(w, r, respond.InvalidBody(err))
		return
	}
	if err := m.v.Struct(req); err != nil {
		respond.Error(w, r, shared.NewValidation("Revisa la foto.",
			shared.Detail{Field: "imagePublicId", Issue: "es obligatorio (máximo 255 caracteres)"}))
		return
	}
	userID, _ := auth.UserID(r.Context())
	sug, err := m.svc.Suggest(r.Context(), userID, req.ImagePublicID)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, suggestionResponse{Title: sug.Title, Category: sug.Category, Description: sug.Description})
}

func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, usecase.ErrDailyLimit) {
		w.Header().Set("Retry-After", strconv.Itoa(m.secondsToMidnight()))
		respond.Failure(w, r, http.StatusTooManyRequests, "rate_limited", messageDailyLimit)
		return
	}
	var de *shared.Error
	switch {
	case errors.As(err, &de) && de.Code == shared.CodeUnavailable:
		// Caída o bloqueo del proveedor: se registra la causa (sin contenido).
		if de.Err != nil {
			// cause y upstreamStatus son valores cerrados (spec 014, T018): sin
			// cuerpos, claves, URLs ni texto del modelo.
			attrs := []any{"error", de.Err}
			if cause, status, ok := aidomain.CauseOf(de.Err); ok {
				attrs = append(attrs, "cause", string(cause))
				if status > 0 {
					attrs = append(attrs, "upstreamStatus", status)
				}
			}
			m.log.WarnContext(r.Context(), "suggest: proveedor de IA no disponible", attrs...)
		}
	case !errors.As(err, &de):
		m.log.ErrorContext(r.Context(), "suggest: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

// secondsToMidnight es el Retry-After del límite diario: hasta la medianoche
// de la zona del contador.
func (m *Module) secondsToMidnight() int {
	now := m.now().In(m.loc)
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, m.loc)
	if s := int(next.Sub(now).Seconds()); s > 0 {
		return s
	}
	return 60
}
