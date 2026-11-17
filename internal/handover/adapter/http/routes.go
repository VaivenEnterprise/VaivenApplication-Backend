package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
)

// Register monta las 8 rutas, todas con sesión.
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.authenticate)
		for _, k := range []domain.Kind{domain.KindDelivery, domain.KindReturn} {
			base := "/bookings/{id}/" + string(k)
			r.Put(base+"/evidence", m.evidence(k))
			r.Post(base+"/qr", m.issueQR(k))
			r.Post(base+"/confirm", m.confirm(k))
			r.Get(base, m.stage(k))
		}
	})
}
