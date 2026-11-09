// Package usecase contiene los casos de uso de las calificaciones (spec 010,
// T004): calificar, consultar si se puede calificar y listar las recibidas.
// Depende solo de puertos (Art. III y XII.4).
package usecase

import (
	"context"
	"log/slog"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Bookings lee reservas (solo lectura, plan P-05); lo implementa
// bookings/adapter/mongo.Repo.
type Bookings interface {
	ByID(ctx context.Context, id string) (bookings.Booking, error)
}

// Repository guarda las calificaciones (adapter/mongo.Repo).
type Repository interface {
	// NewID genera el identificador de una calificación.
	NewID() string
	// Create guarda la calificación y suma el puntaje al perfil de la persona
	// calificada de forma atómica (plan P-01). Si ya existe una calificación de
	// la misma persona a la misma reserva devuelve domain.ErrAlreadyRated.
	Create(ctx context.Context, r domain.Rating) (domain.Rating, error)
	// ByBookingAndRater busca la calificación que raterID dio a la reserva.
	ByBookingAndRater(ctx context.Context, bookingID, raterID string) (domain.Rating, bool, error)
	// ListByRatee devuelve las calificaciones recibidas por rateeID, las más
	// recientes primero, con id menor que after ("" = desde el inicio).
	ListByRatee(ctx context.Context, rateeID, after string, limit int) ([]domain.Rating, error)
}

// Deps son las dependencias del Service.
type Deps struct {
	Repo     Repository
	Bookings Bookings
	Users    users.Directory
	Notifier notify.Notifier
	Log      *slog.Logger
	Now      func() time.Time // nil = time.Now
}

// Límites de la página de calificaciones (plan §2).
const (
	DefaultPage = 20
	MaxPage     = 50
	// EventRatingReceived es el tipo del aviso de calificación recibida (RF-10).
	EventRatingReceived = "rating.received"
	// DeletedName es el nombre que se muestra si la persona ya no existe.
	DeletedName = "Cuenta eliminada"
)

// RatingView es una calificación lista para responder: lleva el nombre visible
// de quien calificó (RF-04).
type RatingView struct {
	Rating    domain.Rating
	RaterName string
}

// Page es una página de calificaciones; Next es el id para seguir (nil al final).
type Page struct {
	Items []RatingView
	Next  *string
}
