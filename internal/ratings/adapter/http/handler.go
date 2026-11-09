// Package http es el adaptador HTTP de las calificaciones (spec 010, T004): las
// 3 rutas de api/paths/ratings.yaml.
package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/usecase"
)

// Service son los casos de uso que usa el adaptador (usecase.Service).
type Service interface {
	Rate(ctx context.Context, userID, bookingID string, score int, comment *string) (usecase.RatingView, error)
	Status(ctx context.Context, userID, bookingID string) (usecase.StatusView, error)
	List(ctx context.Context, rateeID, after string, limit int) (usecase.Page, error)
}

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	Log          *slog.Logger
}

// Module registra las rutas de calificaciones bajo /api/v1 (Art. XII.3).
type Module struct {
	svc  Service
	log  *slog.Logger
	auth func(http.Handler) http.Handler
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.Log == nil {
		return nil, errors.New("ratings: faltan dependencias del módulo HTTP")
	}
	return &Module{svc: d.Service, log: d.Log, auth: d.Authenticate}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "ratings" }

// Register monta las 3 rutas, todas con sesión.
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.auth)
		r.Post("/bookings/{id}/ratings", m.rate)
		r.Get("/bookings/{id}/rating-status", m.status)
		r.Get("/users/{id}/ratings", m.list)
	})
}

func userID(r *http.Request) string {
	id, _ := auth.UserID(r.Context())
	return id
}

func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !errors.As(err, &de) {
		m.log.ErrorContext(r.Context(), "ratings: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

func invalid(field, issue string) error {
	return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: field, Issue: issue})
}

func isObjectIDHex(s string) bool {
	if len(s) != 24 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// --- respuestas ---

type raterResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// ratingResponse es Rating (api/openapi.yaml).
type ratingResponse struct {
	ID        string        `json:"id"`
	BookingID string        `json:"bookingId"`
	Score     int           `json:"score"`
	Comment   *string       `json:"comment"`
	RatedRole string        `json:"ratedRole"`
	Rater     raterResponse `json:"rater"`
	CreatedAt time.Time     `json:"createdAt"`
}

func toRating(r domain.Rating, raterName string) ratingResponse {
	return ratingResponse{
		ID: r.ID, BookingID: r.BookingID, Score: r.Score, Comment: r.Comment, RatedRole: string(r.RatedRole),
		Rater: raterResponse{ID: r.RaterID, DisplayName: raterName}, CreatedAt: r.CreatedAt.UTC(),
	}
}

// --- cursor: base64url(<id de la última calificación>) ---

func encodeCursor(id *string) *string {
	if id == nil {
		return nil
	}
	s := base64.RawURLEncoding.EncodeToString([]byte(*id))
	return &s
}

func decodeCursor(s string) (string, error) {
	if len(s) > 64 {
		return "", errors.New("cursor largo")
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || !isObjectIDHex(string(b)) {
		return "", errors.New("cursor inválido")
	}
	return string(b), nil
}

// --- handlers ---

type rateRequest struct {
	Score   *int    `json:"score"`
	Comment *string `json:"comment"`
}

func (m *Module) rate(w http.ResponseWriter, r *http.Request) {
	var req rateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	if err := dec.Decode(&req); err != nil || dec.More() {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			m.fail(w, r, invalid(typeErr.Field, "tipo de dato inválido"))
			return
		}
		m.fail(w, r, shared.NewValidation("El cuerpo de la petición no es un JSON válido."))
		return
	}
	if req.Score == nil {
		m.fail(w, r, invalid("score", "es obligatorio"))
		return
	}
	v, err := m.svc.Rate(r.Context(), userID(r), chi.URLParam(r, "id"), *req.Score, req.Comment)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusCreated, toRating(v.Rating, v.RaterName))
}

func (m *Module) status(w http.ResponseWriter, r *http.Request) {
	v, err := m.svc.Status(r.Context(), userID(r), chi.URLParam(r, "id"))
	if err != nil {
		m.fail(w, r, err)
		return
	}
	out := struct {
		CanRate   bool            `json:"canRate"`
		MyRating  *ratingResponse `json:"myRating"`
		ExpiresAt *time.Time      `json:"expiresAt"`
	}{CanRate: v.Status.CanRate}
	if mine := v.Status.MyRating; mine != nil {
		rr := toRating(*mine, v.MyRaterName)
		out.MyRating = &rr
	}
	if e := v.Status.ExpiresAt; e != nil {
		u := e.UTC()
		out.ExpiresAt = &u
	}
	respond.JSON(w, http.StatusOK, out)
}

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > usecase.MaxPage {
			m.fail(w, r, invalid("limit", "debe ser un entero entre 1 y "+strconv.Itoa(usecase.MaxPage)))
			return
		}
		limit = n
	}
	after := ""
	if s := r.URL.Query().Get("cursor"); s != "" {
		id, err := decodeCursor(s)
		if err != nil {
			m.fail(w, r, invalid("cursor", "no es válido"))
			return
		}
		after = id
	}
	page, err := m.svc.List(r.Context(), chi.URLParam(r, "id"), after, limit)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	out := struct {
		Items      []ratingResponse `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}{Items: make([]ratingResponse, 0, len(page.Items)), NextCursor: encodeCursor(page.Next)}
	for _, v := range page.Items {
		out.Items = append(out.Items, toRating(v.Rating, v.RaterName))
	}
	respond.JSON(w, http.StatusOK, out)
}
