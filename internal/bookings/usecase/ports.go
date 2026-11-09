// Package usecase contiene los casos de uso de solicitudes y reservas (spec
// 007, T006–T008): cotizar, solicitar, aceptar, rechazar, cancelar, punto de
// encuentro, consultar, listar y vencer. Depende solo de puertos (Art. III y
// XII.4).
package usecase

import (
	"context"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Repository guarda las reservas (implementación: adapter/mongo.Repo).
type Repository interface {
	NewID() string
	Create(ctx context.Context, b domain.Booking) error
	ByID(ctx context.Context, id string) (domain.Booking, error)
	Update(ctx context.Context, b domain.Booking) (domain.Booking, error)
	// Accept guarda la aceptación de forma atómica y devuelve la aceptada y
	// las pendientes que rechazó por cruce (plan P-03).
	Accept(ctx context.Context, b domain.Booking, now time.Time) (domain.Booking, []domain.Booking, error)
	HasBlockingOverlap(ctx context.Context, itemID string, start, end domain.Date) (bool, error)
	HasPendingOverlap(ctx context.Context, itemID, requesterID string, start, end domain.Date, now time.Time) (bool, error)
	ListByParty(ctx context.Context, userID string, role domain.Role, status *domain.Status, after *domain.Cursor, limit int, now time.Time) ([]domain.Booking, error)
	DuePending(ctx context.Context, now time.Time, limit int) ([]domain.Booking, error)
	ItemForBooking(ctx context.Context, itemID string) (domain.ItemForBooking, error)
}

// Viewer es quien hace la petición.
type Viewer struct {
	ID    string
	Admin bool
}

// BookingView es una reserva lista para responder (plan §2).
type BookingView struct {
	// Booking ya trae el estado efectivo (una pendiente vencida es expired).
	Booking   domain.Booking
	Owner     users.PublicUser
	Requester users.PublicUser
	ThumbURL  *string
	// ViewerRole es owner, requester o admin.
	ViewerRole domain.Role
	// ShowMeetingPoint: el punto de encuentro solo lo ven las partes (RF-09).
	ShowMeetingPoint bool
	// Delivery y Return son las etapas de la spec 009 con sus URL firmadas. Solo
	// las ven las partes (RF-05): para admin y para etapas sin datos son nil.
	Delivery *HandoverView
	Return   *HandoverView
}

// HandoverView es una etapa de entrega o devolución lista para responder.
type HandoverView struct {
	Stage domain.Handover
	// URLs corresponde a Stage.Photos, en el mismo orden (firmadas, 10 min).
	URLs []images.URLs
	// QR es el estado del QR en este momento; QRExpiresAt solo si hay uno
	// activo o vencido.
	QR          domain.QRState
	QRExpiresAt *time.Time
}

// Page es una página de GET /me/bookings; Next es nil al final.
type Page struct {
	Items []BookingView
	Next  *domain.Cursor
}

// Límites de la paginación.
const (
	DefaultPageSize = 20
	MaxPageSize     = 50
	// expireBatch acota cuántas solicitudes vence cada pasada (P-04).
	expireBatch = 100
)

// DeletedName es el nombre que se muestra si una cuenta ya no existe.
const DeletedName = "Cuenta eliminada"
