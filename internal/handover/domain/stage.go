// Package domain contiene las reglas de la entrega y la devolución (spec 009,
// T002): roles por etapa, evidencia, QR de un solo uso y ventana de entrega.
// Solo biblioteca estándar (Art. III).
package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// Kind es la etapa presencial: la entrega o la devolución.
type Kind string

// Etapas.
const (
	KindDelivery Kind = "delivery"
	KindReturn   Kind = "return"
)

// Valid indica si la etapa existe.
func (k Kind) Valid() bool { return k == KindDelivery || k == KindReturn }

// RecorderRole es quien registra la evidencia y genera el QR (plan §4).
func (k Kind) RecorderRole() bookings.Role {
	if k == KindReturn {
		return bookings.RoleRequester
	}
	return bookings.RoleOwner
}

// ConfirmerRole es quien escanea y confirma el QR.
func (k Kind) ConfirmerRole() bookings.Role {
	if k == KindReturn {
		return bookings.RoleOwner
	}
	return bookings.RoleRequester
}

// RequiredStatus es el estado que debe tener la reserva para trabajar la etapa.
func (k Kind) RequiredStatus() bookings.Status {
	if k == KindReturn {
		return bookings.StatusInProgress
	}
	return bookings.StatusAccepted
}

// ResultStatus es el estado al confirmar la etapa.
func (k Kind) ResultStatus() bookings.Status {
	if k == KindReturn {
		return bookings.StatusCompleted
	}
	return bookings.StatusInProgress
}

// Límites de la evidencia (RF-01).
const (
	NotesMin = 3
	NotesMax = 500
)

// Of devuelve la etapa guardada en la reserva (nil si no hay nada).
func Of(b bookings.Booking, k Kind) *bookings.Handover {
	if k == KindReturn {
		return b.Return
	}
	return b.Delivery
}

// ValidateNotes recorta y valida el estado del objeto (RF-01).
func ValidateNotes(s string) (string, error) {
	t := strings.TrimSpace(s)
	if n := utf8.RuneCountInString(t); n < NotesMin || n > NotesMax {
		return "", ErrConditionNotes
	}
	return t, nil
}

// ValidatePhotoCount valida cuántas fotos se envían antes de resolverlas (POL-13).
func ValidatePhotoCount(n int) error {
	if n < images.MinImages || n > images.MaxImages {
		return ErrPhotoCount
	}
	return nil
}

// role devuelve el papel de userID; quien no es parte no llega aquí (el caso
// de uso responde 404 antes).
func role(b bookings.Booking, userID string) bookings.Role {
	r, _ := b.RoleOf(userID)
	return r
}

// CanRecord valida que userID pueda registrar evidencia de la etapa (RF-01,
// RF-04, RF-13): el papel correcto, la reserva en el estado correcto y la
// etapa sin confirmar.
func CanRecord(b bookings.Booking, k Kind, userID string) error {
	return canWork(b, k, userID)
}

// CanIssueQR valida que userID pueda generar el QR: lo mismo que registrar
// evidencia y, además, que ya haya evidencia (RF-02).
func CanIssueQR(b bookings.Booking, k Kind, userID string) error {
	if err := canWork(b, k, userID); err != nil {
		return err
	}
	if h := Of(b, k); h == nil || h.RecordedAt == nil || h.EvidenceVersion < 1 {
		return ErrNoEvidence
	}
	return nil
}

func canWork(b bookings.Booking, k Kind, userID string) error {
	if role(b, userID) != k.RecorderRole() {
		return ErrForbidden
	}
	if h := Of(b, k); h != nil && h.ConfirmedAt != nil {
		return ErrAlreadyConfirmed
	}
	if b.Status != k.RequiredStatus() {
		return ErrWrongStatus
	}
	return nil
}

// Record arma la etapa con la evidencia nueva. Reemplazar la evidencia sube
// evidenceVersion e invalida el QR vigente (RF-01).
func Record(prev *bookings.Handover, by, notes string, photos []images.Image, at time.Time) bookings.Handover {
	version := 1
	if prev != nil {
		version = prev.EvidenceVersion + 1
	}
	by2 := by
	at2 := at
	return bookings.Handover{
		RecordedAt:      &at2,
		RecordedByID:    &by2,
		ConditionNotes:  notes,
		Photos:          photos,
		EvidenceVersion: version,
	}
}

// DeliveryWindow es el periodo en que se puede confirmar la entrega (plan
// P-04, provisional [NEEDS CLARIFICATION]): desde el inicio del día anterior a
// startDate hasta el final del día endDate, hora de México. El final es
// exclusivo.
func DeliveryWindow(b bookings.Booking, loc *time.Location) (from, until time.Time) {
	return b.Start.AddDays(-1).StartOf(loc), b.End.AddDays(1).StartOf(loc)
}

// Confirmation es lo que se guarda al confirmar una etapa, de forma atómica
// (plan P-02).
type Confirmation struct {
	Kind        Kind
	NewStatus   bookings.Status
	ConfirmedBy string
	At          time.Time
	History     bookings.HistoryEntry
	// LateAt (solo entrega): desde cuándo la devolución se considera tardía.
	LateAt *time.Time
	// Adjustment (solo devolución): ajuste por retraso, si lo hay.
	Adjustment *bookings.Adjustment
}

// Lateness son las reglas de retraso que necesita la confirmación; las
// implementa payments (T009) y llegan por inyección, así handover no depende
// de ese módulo (Art. XII.4).
type Lateness interface {
	// LateAt es el instante desde el que la reserva se considera tardía.
	LateAt(b bookings.Booking) time.Time
	// Adjustment es el ajuste por devolver tarde en el instante at; nil si no
	// corresponde (a tiempo, o préstamo).
	Adjustment(b bookings.Booking, at time.Time) *bookings.Adjustment
}

// PlanConfirm valida todo lo que puede validarse sin tocar la base y arma lo
// que se guardará. La atomicidad real (QR sin usar, vigente y de la versión
// vigente) la vuelve a exigir el repositorio con el mismo token (RNF-02).
func PlanConfirm(b bookings.Booking, k Kind, userID, token string, at time.Time, loc *time.Location, late Lateness) (Confirmation, error) {
	if strings.TrimSpace(token) == "" {
		return Confirmation{}, ErrTokenRequired
	}
	if role(b, userID) != k.ConfirmerRole() {
		return Confirmation{}, ErrForbidden
	}
	h := Of(b, k)
	if h != nil && h.ConfirmedAt != nil {
		return Confirmation{}, ErrAlreadyConfirmed
	}
	if b.Status != k.RequiredStatus() {
		return Confirmation{}, ErrWrongStatus
	}
	if err := VerifyQR(h, token, at); err != nil {
		return Confirmation{}, err
	}
	if k == KindDelivery {
		from, until := DeliveryWindow(b, loc)
		if at.Before(from) || !at.Before(until) {
			return Confirmation{}, ErrOutsideWindow
		}
	}
	c := Confirmation{
		Kind: k, NewStatus: k.ResultStatus(), ConfirmedBy: userID, At: at,
		History: bookings.HistoryEntry{Status: k.ResultStatus(), At: at, By: bookings.RoleSystem},
	}
	if !bookings.CanTransition(b.Status, c.NewStatus, bookings.RoleSystem) {
		return Confirmation{}, ErrWrongStatus
	}
	if late != nil {
		switch k {
		case KindDelivery:
			t := late.LateAt(b).UTC()
			c.LateAt = &t
		case KindReturn:
			c.Adjustment = late.Adjustment(b, at)
		}
	}
	return c, nil
}
