package domain

import (
	"errors"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

func mustLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := Location()
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func detailFields(t *testing.T, err error) []string {
	t.Helper()
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeValidation {
		t.Fatalf("err = %v, want validation_failed", err)
	}
	var out []string
	for _, d := range de.Details {
		out = append(out, d.Field)
	}
	return out
}

func rentInput(start, end string, mode PaymentMode) QuoteInput {
	return QuoteInput{ItemID: "i1", Modality: ModalityRent, PricePerDayCents: 3000, Start: start, End: end, PaymentMode: mode, Today: MustDate("2026-11-08")}
}

func TestQuoteCA01(t *testing.T) {
	q, err := NewQuote(rentInput("2026-11-10", "2026-11-12", PaymentFullOnDelivery))
	if err != nil {
		t.Fatal(err)
	}
	if q.Days != 3 || q.TotalCents != 9000 || len(q.Payments) != 1 || q.Payments[0] != (Payment{StageDelivery, 9000}) {
		t.Errorf("q = %+v", q)
	}
}

func TestSplitCA02(t *testing.T) {
	p := SplitPayments(9001, PaymentSplit5050)
	if p[0] != (Payment{StageDelivery, 4501}) || p[1] != (Payment{StageReturn, 4500}) {
		t.Errorf("split = %+v", p)
	}
	if p := SplitPayments(9000, PaymentSplit5050); p[0].AmountCents != 4500 || p[1].AmountCents != 4500 {
		t.Errorf("par = %+v", p)
	}
	if p := SplitPayments(0, PaymentNone); p == nil || len(p) != 0 {
		t.Errorf("none = %+v", p)
	}
}

func TestQuoteBordersAndLoan(t *testing.T) {
	cases := map[string]struct {
		start, end string
		days       int
	}{
		"1 día":      {"2026-11-08", "2026-11-08", 1},
		"7 días":     {"2026-11-08", "2026-11-14", 7},
		"cambio mes": {"2026-11-29", "2026-12-02", 4},
		"cambio año": {"2026-12-30", "2027-01-02", 4},
	}
	for name, c := range cases {
		q, err := NewQuote(rentInput(c.start, c.end, PaymentSplit5050))
		if err != nil || q.Days != c.days || q.TotalCents != int64(c.days)*3000 {
			t.Errorf("%s: %+v %v", name, q, err)
		}
	}
	loan := rentInput("2026-11-10", "2026-11-11", PaymentNone)
	loan.Modality = ModalityLoan
	q, err := NewQuote(loan)
	if err != nil || q.TotalCents != 0 || q.PricePerDayCents != 0 || len(q.Payments) != 0 {
		t.Errorf("préstamo = %+v %v", q, err)
	}
}

func TestQuoteErrors(t *testing.T) {
	loan := rentInput("2026-11-10", "2026-11-11", PaymentSplit5050)
	loan.Modality = ModalityLoan
	cases := []struct {
		name string
		in   QuoteInput
		want []string
	}{
		{"8 días", rentInput("2026-11-08", "2026-11-15", PaymentSplit5050), []string{"endDate"}},
		{"fin antes", rentInput("2026-11-10", "2026-11-09", PaymentSplit5050), []string{"endDate"}},
		{"pasado", rentInput("2026-11-07", "2026-11-09", PaymentSplit5050), []string{"startDate"}},
		{"formato", rentInput("10/11/2026", "2026-11-31", PaymentSplit5050), []string{"startDate", "endDate"}},
		{"renta none", rentInput("2026-11-10", "2026-11-11", PaymentNone), []string{"paymentMode"}},
		{"préstamo split", loan, []string{"paymentMode"}},
	}
	for _, c := range cases {
		got := detailFields(t, func() error { _, err := NewQuote(c.in); return err }())
		if len(got) != len(c.want) || got[0] != c.want[0] {
			t.Errorf("%s: campos = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDates(t *testing.T) {
	loc := mustLoc(t)
	// 2026-11-09 03:00 UTC = 2026-11-08 21:00 en Ciudad de México.
	if got := Today(time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC), loc).String(); got != "2026-11-08" {
		t.Errorf("Today = %s", got)
	}
	if start := MustDate("2026-11-10").StartOf(loc).UTC(); !start.Equal(time.Date(2026, 11, 10, 6, 0, 0, 0, time.UTC)) {
		t.Errorf("StartOf = %v", start)
	}
	if !MustDate("x").IsZero() || MustDate("2026-11-10").IsZero() {
		t.Error("MustDate")
	}
	a, b := MustDate("2026-11-10"), MustDate("2026-11-12")
	if !a.Before(b) || !b.After(a) || a.After(b) {
		t.Error("orden")
	}
	ov := []struct {
		s1, e1, s2, e2 string
		want           bool
	}{
		{"2026-11-10", "2026-11-12", "2026-11-12", "2026-11-13", true}, // toca el borde
		{"2026-11-10", "2026-11-12", "2026-11-13", "2026-11-14", false},
		{"2026-11-10", "2026-11-12", "2026-11-08", "2026-11-10", true},
		{"2026-11-10", "2026-11-12", "2026-11-11", "2026-11-11", true},
		{"2026-11-10", "2026-11-12", "2026-11-01", "2026-11-09", false},
	}
	for _, c := range ov {
		if got := Overlaps(MustDate(c.s1), MustDate(c.e1), MustDate(c.s2), MustDate(c.e2)); got != c.want {
			t.Errorf("Overlaps(%v) = %v", c, got)
		}
	}
}
