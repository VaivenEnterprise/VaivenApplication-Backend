// Package usecase contiene los casos de uso de objetos (spec 005, T005–T006):
// publicar, editar, consultar, pausar, reanudar, eliminar, listar los propios
// y sugerir precio. Depende solo de puertos (Art. III y XII.4).
package usecase

import (
	"context"
	"time"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Repository guarda los objetos (implementación: adapter/mongo.ItemRepo).
type Repository interface {
	Create(ctx context.Context, it domain.Item) (domain.Item, error)
	// ByID devuelve el objeto aunque esté removed (la visibilidad la decide
	// el caso de uso) y domain.ErrNotFound si no existe o el id es inválido.
	ByID(ctx context.Context, id string) (domain.Item, error)
	// Update reemplaza el objeto si su versión guardada es it.Version y
	// devuelve el objeto con la versión incrementada; si no,
	// domain.ErrVersionConflict.
	Update(ctx context.Context, it domain.Item) (domain.Item, error)
	ListByOwner(ctx context.Context, ownerID string, statuses []domain.Status, after *domain.Cursor, limit int) ([]domain.Item, error)
}

// BookingChecker indica si un objeto tiene reservas aceptadas o en curso
// (plan P-07, RF-09). La spec 007 lo implementa; hasta entonces se usa
// NoActiveBookings.
type BookingChecker interface {
	HasActiveBookings(ctx context.Context, itemID string) (bool, error)
}

// NoActiveBookings es la implementación vacía: nunca hay reservas.
type NoActiveBookings struct{}

// HasActiveBookings siempre devuelve false.
func (NoActiveBookings) HasActiveBookings(context.Context, string) (bool, error) {
	return false, nil
}

// Viewer es quien hace la petición.
type Viewer struct {
	ID    string
	Admin bool
}

// ItemView es un objeto listo para responder: el perfil público del dueño,
// las URL de cada foto y si quien consulta puede ver el valor declarado
// (plan P-09).
type ItemView struct {
	Item              domain.Item
	Owner             users.PublicUser
	PhotoURLs         []images.URLs
	ShowDeclaredValue bool
}

// Page es una página de GET /me/items. Next es nil si no hay más.
type Page struct {
	Items []ItemView
	Next  *domain.Cursor
}

// Límites de la paginación de GET /me/items.
const (
	DefaultPageSize = 20
	MaxPageSize     = 50
)

// DeletedOwnerName es el nombre que se muestra si el dueño ya no existe.
const DeletedOwnerName = "Cuenta eliminada"

// clockFunc devuelve la hora actual (inyectable en pruebas).
type clockFunc func() time.Time
