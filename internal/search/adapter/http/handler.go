// Package http es el adaptador HTTP de la búsqueda (spec 006, T006):
// GET /api/v1/items (api/paths/search.yaml). Comparte la ruta con POST /items
// de la spec 005; chi las combina por método.
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

// Service es el caso de uso que usa el adaptador (search/usecase.Service).
type Service interface {
	Search(ctx context.Context, viewerID string, raw domain.RawQuery) (usecase.Result, error)
}

// ModuleDeps son las dependencias del módulo de búsqueda.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	Log          *slog.Logger
}

// Module registra GET /items bajo /api/v1 (Art. XII.3).
type Module struct {
	svc  Service
	auth func(http.Handler) http.Handler
	log  *slog.Logger
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.Log == nil {
		return nil, errors.New("search: faltan dependencias del módulo HTTP")
	}
	return &Module{svc: d.Service, auth: d.Authenticate, log: d.Log}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "search" }

// Register monta GET /items con sesión.
func (m *Module) Register(r chi.Router) {
	r.With(m.auth).Get("/items", m.search)
}

type pointResponse struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type zoneResponse struct {
	Municipality string  `json:"municipality"`
	Neighborhood *string `json:"neighborhood"`
}

type ownerResponse struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"displayName"`
	RatingAverage *float64 `json:"ratingAverage"`
	RatingCount   int64    `json:"ratingCount"`
}

// cardResponse es ItemCard (api/openapi.yaml): solo datos públicos.
type cardResponse struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Category         string         `json:"category"`
	Modality         string         `json:"modality"`
	PricePerDayCents int64          `json:"pricePerDayCents"`
	Zone             zoneResponse   `json:"zone"`
	ApproxLocation   *pointResponse `json:"approxLocation"`
	DistanceKm       *float64       `json:"distanceKm"`
	ThumbURL         *string        `json:"thumbUrl"`
	PhotoCount       int            `json:"photoCount"`
	Owner            ownerResponse  `json:"owner"`
	CreatedAt        time.Time      `json:"createdAt"`
}

type pageResponse struct {
	Items      []cardResponse `json:"items"`
	NextCursor *string        `json:"nextCursor"`
}

func toCard(v usecase.CardView) cardResponse {
	c := v.Card
	out := cardResponse{
		ID: c.ID, Title: c.Title, Category: c.Category, Modality: c.Modality,
		PricePerDayCents: c.PricePerDayCents,
		Zone:             zoneResponse{Municipality: c.Zone.Municipality, Neighborhood: c.Zone.Neighborhood},
		DistanceKm:       c.DistanceKm(),
		ThumbURL:         v.ThumbURL,
		PhotoCount:       c.PhotoCount,
		Owner: ownerResponse{
			ID: c.Owner.ID, DisplayName: c.Owner.DisplayName,
			RatingAverage: c.Owner.RatingAverage, RatingCount: c.Owner.RatingCount,
		},
		CreatedAt: c.CreatedAt.UTC(),
	}
	if l := c.Location; l != nil {
		out.ApproxLocation = &pointResponse{Lat: l.Lat, Lng: l.Lng}
	}
	return out
}

func (m *Module) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	raw := domain.RawQuery{
		Q: q.Get("q"), Category: q.Get("category"), Modality: q.Get("modality"),
		Municipality: q.Get("municipality"), From: q.Get("from"), To: q.Get("to"),
		Lat: q.Get("lat"), Lng: q.Get("lng"), RadiusKm: q.Get("radiusKm"),
		Limit: q.Get("limit"), Cursor: q.Get("cursor"),
	}
	viewer, _ := auth.UserID(r.Context())
	res, err := m.svc.Search(r.Context(), viewer, raw)
	if err != nil {
		var de *shared.Error
		if !errors.As(err, &de) {
			m.log.ErrorContext(r.Context(), "search: error interno", "error", err)
		}
		respond.Error(w, r, err)
		return
	}
	out := pageResponse{Items: make([]cardResponse, 0, len(res.Cards))}
	for _, v := range res.Cards {
		out.Items = append(out.Items, toCard(v))
	}
	if res.Next != nil {
		s := res.Next.Encode()
		out.NextCursor = &s
	}
	respond.JSON(w, http.StatusOK, out)
}
