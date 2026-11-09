// Package domain contiene las reglas de solicitudes y reservas (spec 007,
// T002–T003): fechas de calendario, cotización, máquina de estados y reglas de
// creación. Solo biblioteca estándar (Art. III).
package domain

import (
	"errors"
	"time"
	_ "time/tzdata" // la imagen distroless no trae zoneinfo (plan P-02)
)

// TimeZone es la zona de las fechas de calendario (plan §2, P-02).
const TimeZone = "America/Mexico_City"

// Location carga America/Mexico_City (con tzdata embebido).
func Location() (*time.Location, error) { return time.LoadLocation(TimeZone) }

const dateLayout = "2006-01-02"

// Date es una fecha de calendario sin hora (YYYY-MM-DD). Internamente es la
// medianoche UTC de ese día, solo para comparar y contar días.
type Date struct{ t time.Time }

// ErrInvalidDate indica una fecha que no es YYYY-MM-DD válida.
var ErrInvalidDate = errors.New("bookings: fecha inválida")

// ParseCalendarDate lee una fecha YYYY-MM-DD.
func ParseCalendarDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil || t.Format(dateLayout) != s {
		return Date{}, ErrInvalidDate
	}
	return Date{t: t}, nil
}

// MustDate es ParseCalendarDate para valores ya validados (pruebas y datos
// guardados); una fecha inválida devuelve el valor cero.
func MustDate(s string) Date {
	d, _ := ParseCalendarDate(s)
	return d
}

// Today devuelve la fecha de hoy en loc.
func Today(now time.Time, loc *time.Location) Date {
	y, m, d := now.In(loc).Date()
	return Date{t: time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

// String devuelve YYYY-MM-DD.
func (d Date) String() string { return d.t.Format(dateLayout) }

// IsZero indica si la fecha no se fijó.
func (d Date) IsZero() bool { return d.t.IsZero() }

// Before indica si d es anterior a o.
func (d Date) Before(o Date) bool { return d.t.Before(o.t) }

// After indica si d es posterior a o.
func (d Date) After(o Date) bool { return d.t.After(o.t) }

// StartOf devuelve el instante en que empieza el día d en loc.
func (d Date) StartOf(loc *time.Location) time.Time {
	y, m, day := d.t.Date()
	return time.Date(y, m, day, 0, 0, 0, 0, loc)
}

// Days cuenta los días de un periodo con fechas inclusivas (POL-01):
// (end − start) + 1. Si end es anterior a start devuelve ≤ 0.
func Days(start, end Date) int {
	return int(end.t.Sub(start.t).Hours()/24) + 1
}

// Overlaps indica si dos periodos inclusivos se cruzan.
func Overlaps(aStart, aEnd, bStart, bEnd Date) bool {
	return !aEnd.Before(bStart) && !bEnd.Before(aStart)
}
