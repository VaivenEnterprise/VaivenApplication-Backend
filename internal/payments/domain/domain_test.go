package domain

import (
	"strings"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
)

var t0 = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

func ptr(s string) *string { return &s }

func rent(status bookings.Status, pays ...bookings.BookingPayment) bookings.Booking {
	return bookings.Booking{
		ID: "b1", ItemID: "i1", OwnerID: "owner", RequesterID: "req", Status: status,
		Start: bookings.MustDate("2026-11-20"), End: bookings.MustDate("2026-11-22"),
		Price:    bookings.PriceSnapshot{Modality: bookings.ModalityRent, PricePerDayCents: 3000},
		Payments: pays,
	}
}

func pay(stage bookings.Stage, st bookings.PaymentStatus) bookings.BookingPayment {
	return bookings.BookingPayment{Stage: stage, AmountCents: 4500, Status: st}
}

func TestPlanReport(t *testing.T) {
	ok := rent(bookings.StatusAccepted, pay(bookings.StageDelivery, bookings.PaymentPending), pay(bookings.StageReturn, bookings.PaymentPending))
	c, err := PlanReport(ok, "req", bookings.StageDelivery, ptr("  Transferencia  "), t0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Stage != bookings.StageDelivery || c.From != bookings.PaymentPending || c.To != bookings.PaymentReported ||
		!c.At.Equal(t0) || c.Note == nil || *c.Note != "Transferencia" {
		t.Errorf("Change = %+v", c)
	}
	if c, err := PlanReport(ok, "req", bookings.StageReturn, nil, t0); err != nil || c.Note != nil {
		t.Errorf("sin nota: %+v %v", c, err)
	}
	if c, err := PlanReport(ok, "req", bookings.StageReturn, ptr("   "), t0); err != nil || c.Note != nil {
		t.Errorf("nota en blanco = sin nota: %+v %v", c, err)
	}
	if _, err := PlanReport(ok, "req", bookings.StageDelivery, ptr(strings.Repeat("x", 201)), t0); err == nil {
		t.Error("nota de 201 caracteres debe fallar")
	}
	if _, err := PlanReport(ok, "req", bookings.StageDelivery, ptr(strings.Repeat("é", 200)), t0); err != nil {
		t.Errorf("200 caracteres deben valer: %v", err)
	}

	reported := rent(bookings.StatusInProgress, pay(bookings.StageDelivery, bookings.PaymentReported))
	cases := []struct {
		name  string
		b     bookings.Booking
		user  string
		stage bookings.Stage
		want  error
	}{
		{"el propietario no reporta", ok, "owner", bookings.StageDelivery, ErrForbidden},
		{"un tercero tampoco", ok, "otro", bookings.StageDelivery, ErrForbidden},
		{"etapa sin pago", rent(bookings.StatusAccepted, pay(bookings.StageDelivery, bookings.PaymentPending)), "req", bookings.StageReturn, ErrNoPayment},
		{"préstamo sin pagos (RF-08)", rent(bookings.StatusAccepted), "req", bookings.StageDelivery, ErrNoPayment},
		{"reserva pendiente", rent(bookings.StatusPending, pay(bookings.StageDelivery, bookings.PaymentPending)), "req", bookings.StageDelivery, ErrWrongBookingStatus},
		{"reserva cancelada", rent(bookings.StatusCancelled, pay(bookings.StageDelivery, bookings.PaymentPending)), "req", bookings.StageDelivery, ErrWrongBookingStatus},
		{"ya reportado", reported, "req", bookings.StageDelivery, ErrNotPending},
	}
	for _, c := range cases {
		if _, err := PlanReport(c.b, c.user, c.stage, nil, t0); err != c.want {
			t.Errorf("%s: err = %v, quiero %v", c.name, err, c.want)
		}
	}
	// El reporte también se permite con la reserva en curso o completada.
	for _, st := range []bookings.Status{bookings.StatusInProgress, bookings.StatusCompleted} {
		if _, err := PlanReport(rent(st, pay(bookings.StageReturn, bookings.PaymentPending)), "req", bookings.StageReturn, nil, t0); err != nil {
			t.Errorf("reserva %s: %v", st, err)
		}
	}
}

func TestPlanConfirm(t *testing.T) { // CA-06
	rep := rent(bookings.StatusAccepted, pay(bookings.StageDelivery, bookings.PaymentReported))
	c, err := PlanConfirm(rep, "owner", bookings.StageDelivery, t0)
	if err != nil || c.From != bookings.PaymentReported || c.To != bookings.PaymentConfirmed || !c.At.Equal(t0) || c.Note != nil {
		t.Fatalf("Change = %+v %v", c, err)
	}
	cases := []struct {
		name string
		b    bookings.Booking
		user string
		want error
	}{
		{"el solicitante no confirma", rep, "req", ErrForbidden},
		{"sin reporte previo → 409", rent(bookings.StatusAccepted, pay(bookings.StageDelivery, bookings.PaymentPending)), "owner", ErrNotReported},
		{"ya confirmado", rent(bookings.StatusAccepted, pay(bookings.StageDelivery, bookings.PaymentConfirmed)), "owner", ErrNotReported},
		{"etapa sin pago", rent(bookings.StatusAccepted), "owner", ErrNoPayment},
		{"reserva rechazada", rent(bookings.StatusRejected, pay(bookings.StageDelivery, bookings.PaymentReported)), "owner", ErrWrongBookingStatus},
	}
	for _, c := range cases {
		if _, err := PlanConfirm(c.b, c.user, bookings.StageDelivery, t0); err != c.want {
			t.Errorf("%s: err = %v, quiero %v", c.name, err, c.want)
		}
	}
}

func TestValidStage(t *testing.T) {
	if !ValidStage(bookings.StageDelivery) || !ValidStage(bookings.StageReturn) || ValidStage("x") {
		t.Error("ValidStage")
	}
}

func newLateness(t *testing.T) Lateness {
	t.Helper()
	l, err := NewLateness()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestLateAt(t *testing.T) {
	l := newLateness(t)
	b := rent(bookings.StatusInProgress)
	// Fin del 22 de noviembre en México = 06:00 UTC del 23; más 2 h de tolerancia.
	if got, want := l.EndOfDay(b), time.Date(2026, 11, 23, 6, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("EndOfDay = %v", got)
	}
	if got, want := l.LateAt(b), time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("LateAt = %v", got)
	}
	if Tolerance != 2*time.Hour {
		t.Errorf("Tolerance = %v", Tolerance)
	}
}

func TestLateDaysTable(t *testing.T) { // CA-07 y bordes de POL-07
	l := newLateness(t)
	b := rent(bookings.StatusInProgress)
	end := l.EndOfDay(b)
	cases := []struct {
		name string
		at   time.Time
		days int
	}{
		{"antes del fin", end.Add(-time.Hour), 0},
		{"justo al terminar el día", end, 0},
		{"1 h 59 min: dentro de la tolerancia", end.Add(2*time.Hour - time.Minute), 0},
		{"exactamente 2 h: aún no", end.Add(2 * time.Hour), 0},
		{"2 h 01 min: ya pasó, mínimo 1 día", end.Add(2*time.Hour + time.Minute), 1},
		{"24 h exactas: 1 día", end.Add(24 * time.Hour), 1},
		{"24 h 1 s: 2 días", end.Add(24*time.Hour + time.Second), 2},
		{"1 día 3 h tarde: 2 días (CA-07)", end.Add(27 * time.Hour), 2},
		{"48 h exactas: 2 días", end.Add(48 * time.Hour), 2},
		{"48 h 1 min: 3 días", end.Add(48*time.Hour + time.Minute), 3},
	}
	for _, c := range cases {
		if got := l.LateDays(b, c.at); got != c.days {
			t.Errorf("%s: LateDays = %d, quiero %d", c.name, got, c.days)
		}
		if got := l.IsLateAt(b, c.at); got != (c.days > 0) {
			t.Errorf("%s: IsLateAt = %v", c.name, got)
		}
	}
}

func TestAdjustment(t *testing.T) {
	l := newLateness(t)
	b := rent(bookings.StatusInProgress) // $30 por día
	a := l.Adjustment(b, l.EndOfDay(b).Add(27*time.Hour))
	if a == nil || a.Type != "late_return" || a.LateDays != 2 || a.AmountCents != 6000 || a.Status != "recorded" {
		t.Fatalf("CA-07: 2 días × 3000 = 6000, got %+v", a)
	}
	if l.Adjustment(b, l.EndOfDay(b).Add(time.Hour)) != nil {
		t.Error("a tiempo no hay ajuste")
	}
	loan := b
	loan.Price = bookings.PriceSnapshot{Modality: bookings.ModalityLoan}
	if l.Adjustment(loan, l.EndOfDay(loan).Add(72*time.Hour)) != nil {
		t.Error("un préstamo no genera ajuste (RF-10)")
	}
}

func TestAffects(t *testing.T) { // CA-08
	late := rent(bookings.StatusInProgress)
	today := bookings.MustDate("2026-11-23")
	mk := func(mut func(*bookings.Booking)) bookings.Booking {
		o := bookings.Booking{ID: "b2", ItemID: "i1", Status: bookings.StatusAccepted, Start: bookings.MustDate("2026-11-23"), End: bookings.MustDate("2026-11-24")}
		if mut != nil {
			mut(&o)
		}
		return o
	}
	if !Affects(late, mk(nil), today) {
		t.Fatal("la siguiente reserva que empieza hoy debe quedar afectada")
	}
	cases := map[string]bookings.Booking{
		"la misma reserva":      mk(func(o *bookings.Booking) { o.ID = "b1" }),
		"otro objeto":           mk(func(o *bookings.Booking) { o.ItemID = "i2" }),
		"no aceptada":           mk(func(o *bookings.Booking) { o.Status = bookings.StatusPending }),
		"ya marcada":            mk(func(o *bookings.Booking) { a := "x"; o.AffectedBy = &a }),
		"empieza antes del fin": mk(func(o *bookings.Booking) { o.Start = bookings.MustDate("2026-11-22") }),
		"todavía no le toca":    mk(func(o *bookings.Booking) { o.Start = bookings.MustDate("2026-11-24") }),
	}
	for name, o := range cases {
		if Affects(late, o, today) {
			t.Errorf("%s: no debe quedar afectada", name)
		}
	}
}
