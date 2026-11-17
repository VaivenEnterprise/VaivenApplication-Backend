package domain

import (
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// Errores de calificaciones; el adaptador HTTP los traduce con el envoltorio
// del Art. IV.
var (
	// ErrNotCompleted: la reserva aún no está completada (CA-02).
	ErrNotCompleted = shared.NewConflict("Solo puedes calificar una reserva completada.")
	// ErrWindowClosed: pasaron más de 14 días desde que se completó (POL-10).
	ErrWindowClosed = shared.NewConflict("Ya pasó el plazo de 14 días para calificar esta reserva.")
	// ErrAlreadyRated: ya existe una calificación propia de esa reserva (CA-03).
	ErrAlreadyRated = shared.NewConflict("Ya calificaste esta reserva.")
	// ErrUserNotFound: la persona cuyas calificaciones se piden no existe.
	ErrUserNotFound = shared.NewNotFound("No encontramos a esa persona.")
)

// CompletedAt devuelve cuándo se completó la reserva: la última entrada
// `completed` del historial de estados que registra la spec 009 (plan P-02).
func CompletedAt(b bookings.Booking) (time.Time, bool) {
	for i := len(b.History) - 1; i >= 0; i-- {
		if b.History[i].Status == bookings.StatusCompleted {
			return b.History[i].At.UTC(), true
		}
	}
	return time.Time{}, false
}

// ExpiresAt es el fin de la ventana para calificar (POL-10).
func ExpiresAt(completedAt time.Time) time.Time {
	return completedAt.AddDate(0, 0, policy.POL10RatingWindowDays).UTC()
}

// CanRate indica si userID puede calificar la reserva b en el instante now: es
// parte (si no, ErrNotFound, para no revelar la reserva), está completada y
// está dentro de la ventana de 14 días. La unicidad (una calificación por
// persona y reserva) la garantiza el repositorio.
func CanRate(b bookings.Booking, userID string, now time.Time) error {
	if _, ok := b.RoleOf(userID); !ok {
		return bookings.ErrNotFound
	}
	if b.Status != bookings.StatusCompleted {
		return ErrNotCompleted
	}
	at, ok := CompletedAt(b)
	if !ok {
		return ErrNotCompleted
	}
	if !now.Before(ExpiresAt(at)) {
		return ErrWindowClosed
	}
	return nil
}

// New arma la calificación de userID a la otra parte de b. La calificada es la
// otra parte (RF-01). Valida puntaje y comentario; la regla CanRate la aplica
// quien llama antes.
func New(id string, b bookings.Booking, userID string, score int, comment *string, now time.Time) (Rating, error) {
	role, ok := b.RoleOf(userID)
	if !ok {
		return Rating{}, bookings.ErrNotFound
	}
	if err := ValidateScore(score); err != nil {
		return Rating{}, err
	}
	c, err := NormalizeComment(comment)
	if err != nil {
		return Rating{}, err
	}
	rateeID, ratedRole := b.RequesterID, bookings.RoleRequester
	if role == bookings.RoleRequester {
		rateeID, ratedRole = b.OwnerID, bookings.RoleOwner
	}
	return Rating{
		ID: id, BookingID: b.ID, RaterID: userID, RateeID: rateeID, RatedRole: ratedRole,
		Score: score, Comment: c, CreatedAt: now.UTC(),
	}, nil
}
