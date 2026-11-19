// Package http es el adaptador HTTP de las recomendaciones (spec 015, T007;
// api/paths/recommendations.yaml):
//
//	POST   /api/v1/recommendations/events
//	GET    /api/v1/recommendations
//	DELETE /api/v1/recommendations/history
//
// Privacidad (RF-09): los registros nunca incluyen eventos, objetos vistos ni
// identificadores de objetos; solo la causa interna de un fallo.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/usecase"
	searchhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

// eventRequest se valida en la frontera con validator/v10 (Art. VI.2).
type eventRequest struct {
	ItemID string `json:"itemId" validate:"required,len=24,hexadecimal,lowercase"`
	Type   string `json:"type" validate:"required,oneof=view"`
}

type reasonResponse struct {
	Code      string `json:"code"`
	ItemID    string `json:"itemId,omitempty"`
	ItemTitle string `json:"itemTitle,omitempty"`
}

type recommendationResponse struct {
	Item   searchhttp.CardResponse `json:"item"`
	Reason reasonResponse          `json:"reason"`
}

type listResponse struct {
	Items      []recommendationResponse `json:"items"`
	NextCursor *string                  `json:"nextCursor"` // siempre null (Art. IV.4)
}

// Recommender es lo que el handler necesita del caso de uso.
type Recommender interface {
	Enabled() bool
	RecordView(ctx context.Context, userID, itemID, rawType string) error
	Recommend(ctx context.Context, userID, rawLimit, rawMunicipality string) ([]usecase.Recommendation, error)
	DeleteHistory(ctx context.Context, userID string) error
}

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service         Recommender
	Authenticate    func(http.Handler) http.Handler
	Log             *slog.Logger
	Limiter         ratelimit.Limiter
	LimiterFallback ratelimit.Limiter
}

// Module registra las rutas de recomendaciones bajo /api/v1 (Art. XII.3).
type Module struct {
	svc        Recommender
	log        *slog.Logger
	auth       func(http.Handler) http.Handler
	eventLimit func(http.Handler) http.Handler
	readLimit  func(http.Handler) http.Handler
	v          *validator.Validate
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.Log == nil || d.Limiter == nil {
		return nil, errors.New("reco: faltan dependencias del módulo HTTP")
	}
	perUser := func(prefix string) func(http.Handler) http.Handler {
		return middleware.RateLimit(d.Limiter, d.LimiterFallback, middleware.RateLimitConfig{
			Limit:     domain.EventsPerMinute,
			Window:    time.Minute,
			KeyPrefix: prefix,
			UserID:    func(r *http.Request) (string, bool) { return auth.UserID(r.Context()) },
		}, d.Log)
	}
	return &Module{
		svc: d.Service, log: d.Log, auth: d.Authenticate,
		eventLimit: perUser("reco:events:"), // RF-01: 60 vistas por minuto
		readLimit:  perUser("reco:read:"),
		v:          validator.New(validator.WithRequiredStructEnabled()),
	}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "recommendations" }

// Register monta las rutas con sesión y límite por persona.
func (m *Module) Register(r chi.Router) {
	r.With(m.auth, m.eventLimit).Post("/recommendations/events", m.event)
	r.With(m.auth, m.readLimit).Get("/recommendations", m.list)
	r.With(m.auth, m.readLimit).Delete("/recommendations/history", m.deleteHistory)
}

// disabled responde service_unavailable antes de validar nada (RF-10, CA-06).
func (m *Module) disabled(w http.ResponseWriter, r *http.Request) bool {
	if m.svc.Enabled() {
		return false
	}
	respond.Error(w, r, domain.ErrDisabled)
	return true
}

func (m *Module) event(w http.ResponseWriter, r *http.Request) {
	if m.disabled(w, r) {
		return
	}
	var req eventRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // additionalProperties: false en el contrato
	if err := dec.Decode(&req); err != nil || dec.More() {
		respond.Error(w, r, respond.InvalidBody(err))
		return
	}
	if err := m.v.Struct(req); err != nil {
		respond.Error(w, r, eventValidationError(err))
		return
	}
	userID, _ := auth.UserID(r.Context())
	if err := m.svc.RecordView(r.Context(), userID, req.ItemID, req.Type); err != nil {
		m.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	if m.disabled(w, r) {
		return
	}
	userID, _ := auth.UserID(r.Context())
	q := r.URL.Query()
	recs, err := m.svc.Recommend(r.Context(), userID, q.Get("limit"), q.Get("municipality"))
	if err != nil {
		m.fail(w, r, err)
		return
	}
	out := listResponse{Items: make([]recommendationResponse, 0, len(recs))}
	for _, rec := range recs {
		out.Items = append(out.Items, recommendationResponse{
			Item: searchhttp.ToCard(rec.Card),
			Reason: reasonResponse{
				Code: string(rec.Reason.Code), ItemID: rec.Reason.ItemID, ItemTitle: rec.Reason.ItemTitle,
			},
		})
	}
	respond.JSON(w, http.StatusOK, out)
}

func (m *Module) deleteHistory(w http.ResponseWriter, r *http.Request) {
	if m.disabled(w, r) {
		return
	}
	userID, _ := auth.UserID(r.Context())
	if err := m.svc.DeleteHistory(r.Context(), userID); err != nil {
		m.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// eventValidationError arma validation_failed con el campo que falló.
func eventValidationError(err error) error {
	var ve validator.ValidationErrors
	field, issue := "itemId", "debe ser un identificador válido"
	if errors.As(err, &ve) && len(ve) > 0 && ve[0].Field() == "Type" {
		field, issue = "type", "solo se acepta view"
	}
	return shared.NewValidation("Revisa los datos del evento.", shared.Detail{Field: field, Issue: issue})
}

// fail responde el error. Solo se registra la causa de los errores internos;
// nunca el evento ni el objeto (RF-09).
func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !errors.As(err, &de) {
		m.log.ErrorContext(r.Context(), "reco: error interno", "error", err)
	}
	respond.Error(w, r, err)
}
