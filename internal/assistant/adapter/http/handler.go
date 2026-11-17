// Package http es el adaptador HTTP del asistente de proyectos (spec 014,
// T012; api/paths/ai.yaml):
//
//	POST   /api/v1/ai/assistant/messages
//	DELETE /api/v1/ai/assistant/conversations/{id}
//
// Privacidad (RF-18): los registros nunca incluyen el texto de los mensajes,
// de las respuestas ni la ubicación; solo la causa interna de un fallo.
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
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	searchhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

// perMinute es el límite por persona y minuto (RF-15), además del diario.
const perMinute = 10

const messageDailyLimit = "Llegaste al límite diario de mensajes con el asistente. Intenta mañana."

type pointRequest struct {
	Lat *float64 `json:"lat" validate:"required,gte=-90,lte=90"`
	Lng *float64 `json:"lng" validate:"required,gte=-180,lte=180"`
}

// messageRequest se valida en la frontera con validator/v10 (Art. VI.2).
type messageRequest struct {
	ConversationID string        `json:"conversationId" validate:"omitempty,len=24,hexadecimal"`
	Message        string        `json:"message" validate:"required,min=2,max=500"`
	Location       *pointRequest `json:"location" validate:"omitempty"`
}

type needResponse struct {
	Name     string                    `json:"name"`
	Category string                    `json:"category"`
	Optional bool                      `json:"optional"`
	Matches  []searchhttp.CardResponse `json:"matches"`
}

type replyResponse struct {
	Summary     string         `json:"summary"`
	Needs       []needResponse `json:"needs"`
	Steps       []string       `json:"steps"`
	SafetyNotes []string       `json:"safetyNotes"`
}

type messageResponse struct {
	ConversationID string        `json:"conversationId"`
	Reply          replyResponse `json:"reply"`
}

// Assistant es lo que el handler necesita del caso de uso.
type Assistant interface {
	Send(ctx context.Context, userID string, in usecase.SendInput) (usecase.Reply, error)
	Delete(ctx context.Context, userID, conversationID string) error
}

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service         Assistant
	Authenticate    func(http.Handler) http.Handler
	Log             *slog.Logger
	Limiter         ratelimit.Limiter
	LimiterFallback ratelimit.Limiter
	Now             func() time.Time // nil = time.Now (Retry-After)
	Location        *time.Location   // nil = UTC (día del límite)
}

// Module registra las rutas del asistente bajo /api/v1 (Art. XII.3).
type Module struct {
	svc   Assistant
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
		return nil, errors.New("assistant: faltan dependencias del módulo HTTP")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Location == nil {
		d.Location = time.UTC
	}
	return &Module{
		svc: d.Service, log: d.Log, auth: d.Authenticate, now: d.Now, loc: d.Location,
		v: validator.New(validator.WithRequiredStructEnabled()),
		limit: middleware.RateLimit(d.Limiter, d.LimiterFallback, middleware.RateLimitConfig{
			Limit:     perMinute,
			Window:    time.Minute,
			KeyPrefix: "ai:assistant:",
			UserID:    func(r *http.Request) (string, bool) { return auth.UserID(r.Context()) },
		}, d.Log),
	}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "assistant" }

// Register monta las rutas con sesión y límite por persona.
func (m *Module) Register(r chi.Router) {
	r.With(m.auth, m.limit).Post("/ai/assistant/messages", m.message)
	r.With(m.auth, m.limit).Delete("/ai/assistant/conversations/{id}", m.deleteConversation)
}

func (m *Module) message(w http.ResponseWriter, r *http.Request) {
	var req messageRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // additionalProperties: false en el contrato
	if err := dec.Decode(&req); err != nil || dec.More() {
		respond.Error(w, r, shared.NewValidation("El cuerpo de la petición no es un JSON válido."))
		return
	}
	if err := m.v.Struct(req); err != nil {
		respond.Error(w, r, validationError(err))
		return
	}
	in := usecase.SendInput{ConversationID: req.ConversationID, Message: req.Message}
	if req.Location != nil {
		in.Location = &usecase.GeoPoint{Lat: *req.Location.Lat, Lng: *req.Location.Lng}
	}
	userID, _ := auth.UserID(r.Context())
	reply, err := m.svc.Send(r.Context(), userID, in)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toResponse(reply))
}

func (m *Module) deleteConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	if err := m.svc.Delete(r.Context(), userID, chi.URLParam(r, "id")); err != nil {
		m.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toResponse(rep usecase.Reply) messageResponse {
	out := messageResponse{
		ConversationID: rep.ConversationID,
		Reply: replyResponse{
			Summary: rep.Summary, Needs: make([]needResponse, 0, len(rep.Needs)),
			Steps: nonNil(rep.Steps), SafetyNotes: nonNil(rep.SafetyNotes),
		},
	}
	for _, n := range rep.Needs {
		nr := needResponse{Name: n.Need.Name, Category: n.Need.Category, Optional: n.Need.Optional,
			Matches: make([]searchhttp.CardResponse, 0, len(n.Matches))}
		for _, c := range n.Matches {
			nr.Matches = append(nr.Matches, searchhttp.ToCard(c))
		}
		out.Reply.Needs = append(out.Reply.Needs, nr)
	}
	return out
}

// validationError arma validation_failed con el campo que falló.
func validationError(err error) error {
	var ve validator.ValidationErrors
	field, issue := "message", "escribe de 2 a 500 caracteres"
	if errors.As(err, &ve) && len(ve) > 0 {
		switch ve[0].StructNamespace() {
		case "messageRequest.ConversationID":
			field, issue = "conversationId", "no es un identificador válido"
		case "messageRequest.Location.Lat", "messageRequest.Location.Lng":
			field, issue = "location", "lat entre -90 y 90 y lng entre -180 y 180"
		}
	}
	return shared.NewValidation("Revisa los datos marcados.", shared.Detail{Field: field, Issue: issue})
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
		if de.Err != nil {
			m.log.WarnContext(r.Context(), "assistant: no disponible", "error", de.Err)
		}
	case !errors.As(err, &de):
		m.log.ErrorContext(r.Context(), "assistant: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

func (m *Module) secondsToMidnight() int {
	now := m.now().In(m.loc)
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, m.loc)
	if s := int(next.Sub(now).Seconds()); s > 0 {
		return s
	}
	return 60
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
