// Package http es el adaptador HTTP de las sanciones (spec 010, T009): la ruta
// de administración POST /admin/users/{id}/sanctions.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/usecase"
)

// Service es el caso de uso que usa el adaptador (usecase.Service).
type Service interface {
	Apply(ctx context.Context, adminID, userID string, in usecase.Input) (domain.Sanction, error)
}

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	// RequireAdmin deja pasar solo al rol admin (auth.RequireRole).
	RequireAdmin func(http.Handler) http.Handler
	Log          *slog.Logger
}

// Module registra la ruta de sanciones bajo /api/v1 (Art. XII.3).
type Module struct {
	svc   Service
	log   *slog.Logger
	auth  func(http.Handler) http.Handler
	admin func(http.Handler) http.Handler
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.RequireAdmin == nil || d.Log == nil {
		return nil, errors.New("sanctions: faltan dependencias del módulo HTTP")
	}
	return &Module{svc: d.Service, log: d.Log, auth: d.Authenticate, admin: d.RequireAdmin}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "sanctions" }

// Register monta la ruta, con sesión y rol admin (CA-05).
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.auth, m.admin)
		r.Post("/admin/users/{id}/sanctions", m.apply)
	})
}

func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !errors.As(err, &de) {
		m.log.ErrorContext(r.Context(), "sanctions: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

type sanctionRequest struct {
	Type       string     `json:"type"`
	Reason     string     `json:"reason"`
	EndsAt     *time.Time `json:"endsAt"`
	IncidentID *string    `json:"incidentId"`
}

// sanctionResponse es Sanction (api/openapi.yaml).
type sanctionResponse struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	Type       string     `json:"type"`
	Reason     string     `json:"reason"`
	EndsAt     *time.Time `json:"endsAt"`
	IncidentID *string    `json:"incidentId"`
	CreatedAt  time.Time  `json:"createdAt"`
}

func (m *Module) apply(w http.ResponseWriter, r *http.Request) {
	var req sanctionRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	if err := dec.Decode(&req); err != nil || dec.More() {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			m.fail(w, r, shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: typeErr.Field, Issue: "tipo de dato inválido"}))
			return
		}
		m.fail(w, r, shared.NewValidation("El cuerpo de la petición no es un JSON válido."))
		return
	}
	adminID, _ := auth.UserID(r.Context())
	s, err := m.svc.Apply(r.Context(), adminID, chi.URLParam(r, "id"), usecase.Input{
		Type: domain.Type(req.Type), Reason: req.Reason, EndsAt: req.EndsAt, IncidentID: req.IncidentID,
	})
	if err != nil {
		m.fail(w, r, err)
		return
	}
	out := sanctionResponse{
		ID: s.ID, UserID: s.UserID, Type: string(s.Type), Reason: s.Reason, IncidentID: s.IncidentID, CreatedAt: s.CreatedAt.UTC(),
	}
	if s.EndsAt != nil {
		u := s.EndsAt.UTC()
		out.EndsAt = &u
	}
	respond.JSON(w, http.StatusCreated, out)
}
