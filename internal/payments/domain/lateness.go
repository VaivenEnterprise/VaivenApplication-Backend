package domain

import (
	"fmt"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// Tolerance es la espera después del fin de la reserva antes de marcarla
// tardía (POL-07).
const Tolerance = time.Duration(policy.POL07LateToleranceHours) * time.Hour

// Lateness aplica las reglas de retraso con las fechas de México (plan P-05,
// P-06). Lo recibe el módulo de entrega por inyección (handover/domain.Lateness).
type Lateness struct{ loc *time.Location }

// NewLateness crea las reglas con la zona America/Mexico_City.
func NewLateness() (Lateness, error) {
	loc, err := bookings.Location()
	if err != nil {
		return Lateness{}, fmt.Errorf("payments: zona horaria: %w", err)
	}
	return Lateness{loc: loc}, nil
}

// NewLatenessIn crea las reglas con la zona dada (pruebas).
func NewLatenessIn(loc *time.Location) Lateness { return Lateness{loc: loc} }

// EndOfDay es el final del último día de la reserva en México: el inicio del
// día siguiente (el límite es exclusivo, así no hay segundos sueltos).
func (l Lateness) EndOfDay(b bookings.Booking) time.Time {
	return b.End.AddDays(1).StartOf(l.loc)
}

// LateAt es el instante desde el que la devolución se considera tardía: el
// final del último día más la tolerancia de 2 h (plan P-05).
func (l Lateness) LateAt(b bookings.Booking) time.Time {
	return l.EndOfDay(b).Add(Tolerance)
}

// IsLateAt indica si at ya pasó la tolerancia.
func (l Lateness) IsLateAt(b bookings.Booking, at time.Time) bool {
	return at.After(l.LateAt(b))
}

// LateDays son los días de retraso al devolver en at: techo de las horas desde
// el final del último día entre 24, con mínimo 1 una vez pasada la tolerancia;
// 0 si no hay retraso (plan P-06). 1 día 3 h tarde son 2 días.
func (l Lateness) LateDays(b bookings.Booking, at time.Time) int {
	if !l.IsLateAt(b, at) {
		return 0
	}
	over := at.Sub(l.EndOfDay(b))
	days := int((over + 24*time.Hour - 1) / (24 * time.Hour))
	return max(days, 1)
}

// Adjustment es el ajuste por devolver tarde en at: días de retraso × tarifa
// diaria, solo en renta (RF-10). Nil si no hay retraso o es un préstamo. Se
// registra, no se cobra.
func (l Lateness) Adjustment(b bookings.Booking, at time.Time) *bookings.Adjustment {
	if b.Price.Modality != bookings.ModalityRent {
		return nil
	}
	days := l.LateDays(b, at)
	if days == 0 {
		return nil
	}
	return &bookings.Adjustment{
		Type: bookings.AdjustmentLateReturn, LateDays: days,
		AmountCents: int64(days) * b.Price.PricePerDayCents, Status: bookings.AdjustmentRecorded,
	}
}

// Affects indica si other es una reserva afectada por la tardía late (plan
// P-07, CA-08): otra reserva aceptada del mismo objeto que debía empezar
// después de la devolución prevista y cuyo inicio ya llegó (startDate ≤ hoy).
func Affects(late, other bookings.Booking, today bookings.Date) bool {
	return other.ID != late.ID &&
		other.ItemID == late.ItemID &&
		other.Status == bookings.StatusAccepted &&
		other.AffectedBy == nil &&
		other.Start.After(late.End) &&
		!other.Start.After(today)
}
