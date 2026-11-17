// Package domain contiene las reglas de las calificaciones (spec 010, T002):
// puntaje, comentario, quién puede calificar y hasta cuándo, y el promedio.
// Solo biblioteca estándar y los dominios compartidos (Art. III).
package domain

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// MessageInvalidData es el mensaje general de validation_failed.
const MessageInvalidData = bookings.MessageInvalidData

// Rating es la calificación que una parte da a la otra al concluir una
// reserva. No se edita ni se borra (spec 010 §5).
type Rating struct {
	ID        string
	BookingID string
	RaterID   string
	RateeID   string
	// RatedRole es el papel que tenía en la reserva la persona calificada.
	RatedRole bookings.Role
	Score     int
	Comment   *string
	CreatedAt time.Time
}

// Status es el resultado de RF-03: si se puede calificar y, si ya se hizo, la
// calificación propia. ExpiresAt es el fin de la ventana (nil sin completar).
type Status struct {
	CanRate   bool
	MyRating  *Rating
	ExpiresAt *time.Time
}

// ValidateScore exige un entero de 1 a 5 (POL-10).
func ValidateScore(score int) error {
	if score < policy.POL10MinScore || score > policy.POL10MaxScore {
		return shared.NewValidation(MessageInvalidData, shared.Detail{Field: "score", Issue: "debe ser un entero entre 1 y 5"})
	}
	return nil
}

// NormalizeComment recorta el comentario opcional (RF-01): vacío = nil y más
// de 300 caracteres es inválido.
func NormalizeComment(c *string) (*string, error) {
	if c == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*c)
	if t == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(t) > policy.POL10MaxCommentChars {
		return nil, shared.NewValidation(MessageInvalidData, shared.Detail{Field: "comment", Issue: "debe tener como máximo 300 caracteres"})
	}
	return &t, nil
}

// Average devuelve el promedio con un decimal, o nil sin calificaciones
// (RF-02). El redondeo es el de siempre: .05 sube.
func Average(sum, count int64) *float64 {
	if count <= 0 {
		return nil
	}
	avg := math.Round(float64(sum)/float64(count)*10) / 10
	return &avg
}
