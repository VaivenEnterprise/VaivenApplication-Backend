package http

import "github.com/go-chi/chi/v5"

// Register monta las 2 rutas, ambas con sesión.
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.authenticate)
		r.Post("/bookings/{id}/payments/{stage}/report", m.report)
		r.Post("/bookings/{id}/payments/{stage}/confirm", m.confirm)
	})
}
