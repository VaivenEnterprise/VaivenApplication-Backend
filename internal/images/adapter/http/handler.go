// Package http es el adaptador HTTP del módulo de imágenes (spec 004, T008):
// POST /api/v1/uploads/signature (api/paths/uploads.yaml).
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

type signatureRequest struct {
	Purpose string `json:"purpose"`
}

type signatureResponse struct {
	UploadURL      string `json:"uploadUrl"`
	APIKey         string `json:"apiKey"`
	Timestamp      int64  `json:"timestamp"`
	Signature      string `json:"signature"`
	PublicID       string `json:"publicId"`
	Type           string `json:"type"`
	AllowedFormats string `json:"allowedFormats"`
}

// ModuleDeps son las dependencias del módulo de imágenes.
type ModuleDeps struct {
	Service images.Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	Log          *slog.Logger
	// Límite de firmas por persona (RF-04): Redis con respaldo local.
	Limiter           ratelimit.Limiter
	LimiterFallback   ratelimit.Limiter
	SignaturesPerHour int
}

// Module registra las rutas de imágenes bajo /api/v1 (Art. XII.3).
type Module struct {
	svc   images.Service
	log   *slog.Logger
	auth  func(http.Handler) http.Handler
	limit func(http.Handler) http.Handler
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.Log == nil || d.Limiter == nil || d.SignaturesPerHour <= 0 {
		return nil, errors.New("images: faltan dependencias del módulo HTTP")
	}
	return &Module{
		svc:  d.Service,
		log:  d.Log,
		auth: d.Authenticate,
		// Clave por persona (va después de Authenticate): 30 firmas por hora.
		limit: middleware.RateLimit(d.Limiter, d.LimiterFallback, middleware.RateLimitConfig{
			Limit:     d.SignaturesPerHour,
			Window:    time.Hour,
			KeyPrefix: "uploads:",
			UserID: func(r *http.Request) (string, bool) {
				return auth.UserID(r.Context())
			},
		}, d.Log),
	}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "images" }

// Register monta POST /uploads/signature: sesión obligatoria (CA-05) y límite
// por persona (RF-04).
func (m *Module) Register(r chi.Router) {
	r.With(m.auth, m.limit).Post("/uploads/signature", m.signature)
}

func (m *Module) signature(w http.ResponseWriter, r *http.Request) {
	var req signatureRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil || dec.More() {
		respond.Error(w, r, shared.NewValidation("El cuerpo de la petición no es un JSON válido."))
		return
	}
	userID, _ := auth.UserID(r.Context())
	sig, err := m.svc.Signature(r.Context(), userID, domain.Purpose(req.Purpose))
	if err != nil {
		var de *shared.Error
		if !errors.As(err, &de) {
			m.log.ErrorContext(r.Context(), "images: error interno", "error", err)
		}
		respond.Error(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, signatureResponse{
		UploadURL:      sig.UploadURL,
		APIKey:         sig.APIKey,
		Timestamp:      sig.Timestamp,
		Signature:      sig.Signature,
		PublicID:       sig.PublicID,
		Type:           sig.Type,
		AllowedFormats: sig.AllowedFormats,
	})
}
