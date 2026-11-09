// Package usecase contiene los casos de uso de pagos y retrasos (spec 009,
// T010–T011): reportar y confirmar el pago, y detectar devoluciones tardías y
// reservas afectadas. Depende solo de puertos (Art. III y XII.4).
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/domain"
)

// ErrNoMatch lo devuelve el repositorio cuando la operación atómica no aplicó
// porque la reserva ya no cumple el filtro (otra petición ganó).
var ErrNoMatch = errors.New("payments: la operación atómica no aplicó")

// Bookings lee reservas (lo implementa bookings/adapter/mongo.Repo).
type Bookings interface {
	ByID(ctx context.Context, id string) (bookings.Booking, error)
}

// LateBooking son los datos mínimos de una reserva tardía o afectada.
type LateBooking struct {
	ID          string
	ItemID      string
	OwnerID     string
	RequesterID string
	End         bookings.Date
}

// Repository guarda los pagos y las marcas de retraso dentro de la reserva
// (plan P-09: solo toca sus campos y sube la versión). Los métodos de escritura
// son atómicos sobre un documento y devuelven ErrNoMatch si no aplicaron.
type Repository interface {
	// ChangePayment pasa el pago de c.From a c.To si la reserva sigue aceptando
	// pagos y el pago sigue en c.From.
	ChangePayment(ctx context.Context, bookingID string, c domain.Change) error
	// DueLate devuelve hasta limit reservas in_progress sin marcar cuyo lateAt
	// ya pasó.
	DueLate(ctx context.Context, now time.Time, limit int) ([]LateBooking, error)
	// MarkLate marca isLate si la reserva sigue in_progress y sin marcar.
	MarkLate(ctx context.Context, bookingID string, at time.Time) error
	// AffectedCandidates devuelve las reservas aceptadas del mismo objeto que
	// empiezan después de late.End y a más tardar hoy, sin marcar aún.
	AffectedCandidates(ctx context.Context, late LateBooking, today bookings.Date) ([]LateBooking, error)
	// MarkAffected marca affectedBy si la reserva sigue aceptada y sin marcar.
	MarkAffected(ctx context.Context, bookingID, lateBookingID string, at time.Time) error
}

// Viewer es quien hace la petición.
type Viewer struct{ ID string }

// Deps son las dependencias del Service.
type Deps struct {
	Repo     Repository
	Bookings Bookings
	Notifier notify.Notifier
	Log      *slog.Logger
	Now      func() time.Time // nil = time.Now
	Location *time.Location   // nil = America/Mexico_City
}

// detectBatch acota cuántas reservas procesa cada pasada del detector.
const detectBatch = 100
