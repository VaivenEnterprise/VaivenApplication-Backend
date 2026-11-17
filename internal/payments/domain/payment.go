// Package domain contiene las reglas de los pagos registrados y del retraso en
// la devolución (spec 009, T009): la máquina pending → reported → confirmed,
// el cálculo de lateAt y del ajuste, y la reserva afectada. VAIVÉN no cobra ni
// verifica pagos: solo lleva el control de lo acordado. Solo biblioteca
// estándar (Art. III).
package domain

import (
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Errores de dominio de pagos; el adaptador HTTP los traduce con el envoltorio
// del Art. IV.
var (
	// ErrForbidden: la persona es parte de la reserva pero la acción no le toca.
	ErrForbidden = shared.NewForbidden("No te corresponde esta acción en este pago.")
	// ErrNoPayment: la reserva no tiene un pago para esa etapa (p. ej. un
	// préstamo gratuito, RF-08, o la etapa de devolución en pago único).
	ErrNoPayment = shared.NewNotFound("Esta reserva no tiene un pago para esa etapa.")
	// ErrWrongBookingStatus: la reserva no admite pagos en su estado actual.
	ErrWrongBookingStatus = shared.NewConflict("La reserva no admite pagos en su estado actual.")
	// ErrNotPending: el pago ya fue reportado o confirmado.
	ErrNotPending = shared.NewConflict("Este pago ya fue reportado.")
	// ErrNotReported: confirmar exige que el solicitante haya reportado antes
	// (CA-06) y que no esté ya confirmado.
	ErrNotReported = shared.NewConflict("El solicitante aún no reporta este pago o ya está confirmado.")
)

// NoteMax es el largo máximo de la nota del reporte (RF-07).
const NoteMax = 200

// Change es el cambio de estado de un pago, listo para guardar.
type Change struct {
	Stage bookings.Stage
	// From y To son el estado esperado y el nuevo; el repositorio exige From.
	From, To bookings.PaymentStatus
	At       time.Time
	// Note solo en el reporte.
	Note *string
}

// acceptsPayments indica si la reserva puede tener pagos en ese estado: ya
// aceptada y no cancelada ni vencida.
func acceptsPayments(s bookings.Status) bool {
	return s == bookings.StatusAccepted || s == bookings.StatusInProgress || s == bookings.StatusCompleted
}

func find(b bookings.Booking, stage bookings.Stage) (bookings.BookingPayment, error) {
	p, ok := b.PaymentFor(stage)
	if !ok {
		return bookings.BookingPayment{}, ErrNoPayment
	}
	return p, nil
}

// PlanReport valida que userID pueda reportar el pago de la etapa y arma el
// cambio pending → reported (RF-07). Solo el solicitante; la nota es opcional.
// El orden de los errores es: papel (403), pago inexistente (404), estado de
// la reserva (409), estado del pago (409), nota (400).
func PlanReport(b bookings.Booking, userID string, stage bookings.Stage, note *string, at time.Time) (Change, error) {
	if r, _ := b.RoleOf(userID); r != bookings.RoleRequester {
		return Change{}, ErrForbidden
	}
	p, err := find(b, stage)
	if err != nil {
		return Change{}, err
	}
	if !acceptsPayments(b.Status) {
		return Change{}, ErrWrongBookingStatus
	}
	if p.Status != bookings.PaymentPending {
		return Change{}, ErrNotPending
	}
	n, err := bookings.NormalizeText("note", note, 1, NoteMax)
	if err != nil {
		return Change{}, err
	}
	return Change{Stage: stage, From: bookings.PaymentPending, To: bookings.PaymentReported, At: at, Note: n}, nil
}

// PlanConfirm valida que userID pueda confirmar el pago de la etapa y arma el
// cambio reported → confirmed. Solo el propietario, y solo si el solicitante
// ya lo reportó (CA-06).
func PlanConfirm(b bookings.Booking, userID string, stage bookings.Stage, at time.Time) (Change, error) {
	if r, _ := b.RoleOf(userID); r != bookings.RoleOwner {
		return Change{}, ErrForbidden
	}
	p, err := find(b, stage)
	if err != nil {
		return Change{}, err
	}
	if !acceptsPayments(b.Status) {
		return Change{}, ErrWrongBookingStatus
	}
	if p.Status != bookings.PaymentReported {
		return Change{}, ErrNotReported
	}
	return Change{Stage: stage, From: bookings.PaymentReported, To: bookings.PaymentConfirmed, At: at}, nil
}

// ValidStage indica si stage es una etapa de pago (delivery o return).
func ValidStage(stage bookings.Stage) bool {
	return stage == bookings.StageDelivery || stage == bookings.StageReturn
}
