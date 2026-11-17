package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ModuleDeps son las dependencias del módulo de objetos.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	// RequireActive bloquea a cuentas suspendidas (spec 003, RF-13); se monta
	// solo en POST /items (RF-12).
	RequireActive func(http.Handler) http.Handler
	Log           *slog.Logger
}

// Module registra las rutas de objetos bajo /api/v1 (Art. XII.3).
type Module struct {
	h             handlers
	authenticate  func(http.Handler) http.Handler
	requireActive func(http.Handler) http.Handler
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.RequireActive == nil || d.Log == nil {
		return nil, errors.New("items: faltan dependencias del módulo HTTP")
	}
	return &Module{
		h:             handlers{svc: d.Service, dec: newDecoder(), log: d.Log},
		authenticate:  d.Authenticate,
		requireActive: d.RequireActive,
	}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "items" }

// Register monta las 8 rutas de api/paths/items.yaml, todas con sesión. La
// ruta estática price-suggestion va antes de {id}.
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.authenticate)
		r.With(m.requireActive).Post("/items", m.h.create)
		r.Get("/items/price-suggestion", m.h.priceSuggestion)
		r.Get("/items/{id}", m.h.get)
		r.Patch("/items/{id}", m.h.update)
		r.Delete("/items/{id}", m.h.remove)
		r.Post("/items/{id}/pause", m.h.pause)
		r.Post("/items/{id}/resume", m.h.resume)
		r.Get("/me/items", m.h.listMine)
		r.Get("/categories", m.h.categories)
	})
}
