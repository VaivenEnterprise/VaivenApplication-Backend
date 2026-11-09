// Package http es el adaptador HTTP de solicitudes y reservas (spec 007,
// T010): las 8 rutas de api/paths/bookings.yaml.
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Service son los casos de uso que usa el adaptador (usecase.Service).
type Service interface {
	Quote(ctx context.Context, viewer usecase.Viewer, in usecase.QuoteInput) (domain.Quote, error)
	Create(ctx context.Context, viewer usecase.Viewer, in usecase.CreateInput) (usecase.BookingView, error)
	Get(ctx context.Context, viewer usecase.Viewer, id string) (usecase.BookingView, error)
	List(ctx context.Context, viewer usecase.Viewer, q usecase.ListQuery) (usecase.Page, error)
	Accept(ctx context.Context, viewer usecase.Viewer, id string, meetingPoint *string) (usecase.BookingView, error)
	Reject(ctx context.Context, viewer usecase.Viewer, id string, reason *string) (usecase.BookingView, error)
	Cancel(ctx context.Context, viewer usecase.Viewer, id string, reason *string) (usecase.BookingView, error)
	SetMeetingPoint(ctx context.Context, viewer usecase.Viewer, id, description string) (usecase.BookingView, error)
}

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	// RequireActive bloquea a cuentas suspendidas; solo en POST /bookings.
	RequireActive func(http.Handler) http.Handler
	Log           *slog.Logger
}

// Module registra las rutas de reservas bajo /api/v1 (Art. XII.3).
type Module struct {
	svc           Service
	log           *slog.Logger
	authenticate  func(http.Handler) http.Handler
	requireActive func(http.Handler) http.Handler
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.RequireActive == nil || d.Log == nil {
		return nil, errors.New("bookings: faltan dependencias del módulo HTTP")
	}
	return &Module{svc: d.Service, log: d.Log, authenticate: d.Authenticate, requireActive: d.RequireActive}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "bookings" }

// Register monta las 8 rutas, todas con sesión.
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.authenticate)
		r.Get("/items/{id}/quote", m.quote)
		r.With(m.requireActive).Post("/bookings", m.create)
		r.Get("/bookings/{id}", m.get)
		r.Get("/me/bookings", m.list)
		r.Post("/bookings/{id}/accept", m.accept)
		r.Post("/bookings/{id}/reject", m.reject)
		r.Post("/bookings/{id}/cancel", m.cancel)
		r.Post("/bookings/{id}/meeting-point", m.meetingPoint)
	})
}

func viewer(r *http.Request) usecase.Viewer {
	id, _ := auth.UserID(r.Context())
	return usecase.Viewer{ID: id, Admin: auth.Role(r.Context()) == "admin"}
}

func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !errors.As(err, &de) {
		m.log.ErrorContext(r.Context(), "bookings: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

func (m *Module) reply(w http.ResponseWriter, r *http.Request, status int, v usecase.BookingView, err error) {
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, status, toBooking(v))
}

func (m *Module) quote(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var d []shared.Detail
	for _, f := range []string{"startDate", "endDate", "paymentMode"} {
		if q.Get(f) == "" {
			d = append(d, shared.Detail{Field: f, Issue: "es obligatorio"})
		}
	}
	if len(d) > 0 {
		m.fail(w, r, shared.NewValidation(domain.MessageInvalidData, d...))
		return
	}
	res, err := m.svc.Quote(r.Context(), viewer(r), usecase.QuoteInput{
		ItemID: chi.URLParam(r, "id"), Start: q.Get("startDate"), End: q.Get("endDate"),
		PaymentMode: domain.PaymentMode(q.Get("paymentMode")),
	})
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toQuote(res))
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decode(r, &req, false); err != nil {
		m.fail(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		m.fail(w, r, err)
		return
	}
	v, err := m.svc.Create(r.Context(), viewer(r), in)
	m.reply(w, r, http.StatusCreated, v, err)
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	v, err := m.svc.Get(r.Context(), viewer(r), chi.URLParam(r, "id"))
	m.reply(w, r, http.StatusOK, v, err)
}

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	q, err := parseListQuery(r)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	page, err := m.svc.List(r.Context(), viewer(r), q)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	out := pageResponse{Items: make([]bookingResponse, 0, len(page.Items)), NextCursor: encodeCursor(page.Next)}
	for _, v := range page.Items {
		out.Items = append(out.Items, toBooking(v))
	}
	respond.JSON(w, http.StatusOK, out)
}

func (m *Module) accept(w http.ResponseWriter, r *http.Request) {
	var req acceptRequest
	if err := decode(r, &req, true); err != nil {
		m.fail(w, r, err)
		return
	}
	var mp *string
	if req.MeetingPoint != nil {
		if req.MeetingPoint.Description == nil {
			m.fail(w, r, invalid("meetingPoint.description", "es obligatorio"))
			return
		}
		mp = req.MeetingPoint.Description
	}
	v, err := m.svc.Accept(r.Context(), viewer(r), chi.URLParam(r, "id"), mp)
	m.reply(w, r, http.StatusOK, v, err)
}

func (m *Module) reject(w http.ResponseWriter, r *http.Request) {
	var req reasonRequest
	if err := decode(r, &req, true); err != nil {
		m.fail(w, r, err)
		return
	}
	v, err := m.svc.Reject(r.Context(), viewer(r), chi.URLParam(r, "id"), req.Reason)
	m.reply(w, r, http.StatusOK, v, err)
}

func (m *Module) cancel(w http.ResponseWriter, r *http.Request) {
	var req reasonRequest
	if err := decode(r, &req, true); err != nil {
		m.fail(w, r, err)
		return
	}
	v, err := m.svc.Cancel(r.Context(), viewer(r), chi.URLParam(r, "id"), req.Reason)
	m.reply(w, r, http.StatusOK, v, err)
}

func (m *Module) meetingPoint(w http.ResponseWriter, r *http.Request) {
	var req meetingPointJSON
	if err := decode(r, &req, false); err != nil {
		m.fail(w, r, err)
		return
	}
	if req.Description == nil {
		m.fail(w, r, invalid("description", "es obligatorio"))
		return
	}
	v, err := m.svc.SetMeetingPoint(r.Context(), viewer(r), chi.URLParam(r, "id"), *req.Description)
	m.reply(w, r, http.StatusOK, v, err)
}
