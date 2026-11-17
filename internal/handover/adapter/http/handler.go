// Package http es el adaptador HTTP de la entrega y la devolución (spec 009,
// T006): las 8 rutas de api/paths/handover.yaml.
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/usecase"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// Service son los casos de uso que usa el adaptador (usecase.Service).
type Service interface {
	RecordEvidence(ctx context.Context, v usecase.Viewer, bookingID string, k domain.Kind, notes string, refs []images.Ref) (usecase.StageView, error)
	IssueQR(ctx context.Context, v usecase.Viewer, bookingID string, k domain.Kind) (usecase.QRIssued, error)
	Confirm(ctx context.Context, v usecase.Viewer, bookingID string, k domain.Kind, token string) error
	GetStage(ctx context.Context, v usecase.Viewer, bookingID string, k domain.Kind) (usecase.StageView, error)
}

// WriteBooking responde con la reserva actual (el mismo Booking de la spec 007)
// para el id dado. Lo implementa el módulo de reservas; así este módulo no
// repite su mapeo (Art. XII.4).
type WriteBooking func(w http.ResponseWriter, r *http.Request, bookingID string)

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	WriteBooking WriteBooking
	Log          *slog.Logger
}

// Module registra las rutas de entrega y devolución bajo /api/v1 (Art. XII.3).
type Module struct {
	svc          Service
	log          *slog.Logger
	authenticate func(http.Handler) http.Handler
	writeBooking WriteBooking
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.WriteBooking == nil || d.Log == nil {
		return nil, errors.New("handover: faltan dependencias del módulo HTTP")
	}
	return &Module{svc: d.Service, log: d.Log, authenticate: d.Authenticate, writeBooking: d.WriteBooking}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "handover" }

func viewer(r *http.Request) usecase.Viewer {
	id, _ := auth.UserID(r.Context())
	return usecase.Viewer{ID: id}
}

func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !errors.As(err, &de) {
		m.log.ErrorContext(r.Context(), "handover: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

func (m *Module) evidence(k domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req evidenceRequest
		if err := decode(r, &req); err != nil {
			m.fail(w, r, err)
			return
		}
		notes, refs, err := req.input()
		if err != nil {
			m.fail(w, r, err)
			return
		}
		v, err := m.svc.RecordEvidence(r.Context(), viewer(r), chi.URLParam(r, "id"), k, notes, refs)
		if err != nil {
			m.fail(w, r, err)
			return
		}
		respond.JSON(w, http.StatusOK, toStage(v))
	}
}

func (m *Module) issueQR(k domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, err := m.svc.IssueQR(r.Context(), viewer(r), chi.URLParam(r, "id"), k)
		if err != nil {
			m.fail(w, r, err)
			return
		}
		// El token solo viaja en esta respuesta; no se registra (RNF-01).
		w.Header().Set("Cache-Control", "no-store")
		respond.JSON(w, http.StatusCreated, toQR(q))
	}
}

func (m *Module) confirm(k domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req confirmRequest
		if err := decode(r, &req); err != nil {
			m.fail(w, r, err)
			return
		}
		if req.Token == nil {
			m.fail(w, r, domain.ErrTokenRequired)
			return
		}
		id := chi.URLParam(r, "id")
		if err := m.svc.Confirm(r.Context(), viewer(r), id, k, *req.Token); err != nil {
			m.fail(w, r, err)
			return
		}
		m.writeBooking(w, r, id)
	}
}

func (m *Module) stage(k domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := m.svc.GetStage(r.Context(), viewer(r), chi.URLParam(r, "id"), k)
		if err != nil {
			m.fail(w, r, err)
			return
		}
		respond.JSON(w, http.StatusOK, toStage(v))
	}
}
