// Package http es el adaptador HTTP de los pagos registrados (spec 009, T012):
// las 2 rutas de api/paths/payments.yaml.
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/usecase"
)

// Service son los casos de uso que usa el adaptador (usecase.Service).
type Service interface {
	Report(ctx context.Context, v usecase.Viewer, bookingID string, stage bookings.Stage, note *string) error
	Confirm(ctx context.Context, v usecase.Viewer, bookingID string, stage bookings.Stage) error
}

// WriteBooking responde con la reserva actual (el mismo Booking de la spec 007)
// para el id dado; lo implementa el módulo de reservas (Art. XII.4).
type WriteBooking func(w http.ResponseWriter, r *http.Request, bookingID string)

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	WriteBooking WriteBooking
	Log          *slog.Logger
}

// Module registra las rutas de pagos bajo /api/v1 (Art. XII.3).
type Module struct {
	svc          Service
	log          *slog.Logger
	authenticate func(http.Handler) http.Handler
	writeBooking WriteBooking
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.WriteBooking == nil || d.Log == nil {
		return nil, errors.New("payments: faltan dependencias del módulo HTTP")
	}
	return &Module{svc: d.Service, log: d.Log, authenticate: d.Authenticate, writeBooking: d.WriteBooking}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "payments" }

func viewer(r *http.Request) usecase.Viewer {
	id, _ := auth.UserID(r.Context())
	return usecase.Viewer{ID: id}
}

func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !errors.As(err, &de) {
		m.log.ErrorContext(r.Context(), "payments: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

func (m *Module) report(w http.ResponseWriter, r *http.Request) {
	var req reportRequest
	if err := decode(r, &req); err != nil {
		m.fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if err := m.svc.Report(r.Context(), viewer(r), id, bookings.Stage(chi.URLParam(r, "stage")), req.Note); err != nil {
		m.fail(w, r, err)
		return
	}
	m.writeBooking(w, r, id)
}

func (m *Module) confirm(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := m.svc.Confirm(r.Context(), viewer(r), id, bookings.Stage(chi.URLParam(r, "stage"))); err != nil {
		m.fail(w, r, err)
		return
	}
	m.writeBooking(w, r, id)
}
