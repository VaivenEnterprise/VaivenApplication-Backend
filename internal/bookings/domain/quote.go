package domain

import (
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// Modality es la modalidad del objeto (préstamo o renta).
type Modality string

// Modalidades.
const (
	ModalityLoan Modality = "loan"
	ModalityRent Modality = "rent"
)

// PaymentMode es cómo se paga una renta (POL-05).
type PaymentMode string

// Modalidades de pago.
const (
	PaymentNone           PaymentMode = "none"
	PaymentFullOnDelivery PaymentMode = "full_on_delivery"
	PaymentSplit5050      PaymentMode = "split_50_50"
)

// Stage es la etapa de un pago.
type Stage string

// Etapas.
const (
	StageDelivery Stage = "delivery"
	StageReturn   Stage = "return"
)

// Payment es un importe por etapa (centavos).
type Payment struct {
	Stage       Stage
	AmountCents int64
}

// Currency es la única moneda (Art. V).
const Currency = "MXN"

// Quote es la cotización de un periodo (RF-01).
type Quote struct {
	ItemID           string
	Start, End       Date
	Days             int
	Modality         Modality
	PricePerDayCents int64
	TotalCents       int64
	PaymentMode      PaymentMode
	Payments         []Payment
}

// QuoteInput son los datos para cotizar.
type QuoteInput struct {
	ItemID           string
	Modality         Modality
	PricePerDayCents int64
	Start, End       string
	PaymentMode      PaymentMode
	// Today es hoy en America/Mexico_City.
	Today Date
}

// NewQuote valida el periodo y la modalidad de pago y calcula el total
// (plan P-01): days = (end − start) + 1; total = days × tarifa; en
// split_50_50 la entrega es ceil(total/2) y la devolución floor(total/2).
func NewQuote(in QuoteInput) (Quote, error) {
	var d []shared.Detail
	add := func(f, issue string) { d = append(d, shared.Detail{Field: f, Issue: issue}) }

	start, errS := ParseCalendarDate(in.Start)
	end, errE := ParseCalendarDate(in.End)
	if errS != nil {
		add("startDate", "debe tener el formato AAAA-MM-DD")
	}
	if errE != nil {
		add("endDate", "debe tener el formato AAAA-MM-DD")
	}
	days := 0
	if errS == nil && errE == nil {
		days = Days(start, end)
		switch {
		case start.Before(in.Today):
			add("startDate", "no puede ser anterior a hoy")
		case days < policy.POL02MinDays:
			add("endDate", "no puede ser anterior a startDate")
		case days > policy.POL02MaxDays:
			add("endDate", "el periodo dura como máximo 7 días")
		}
	}
	switch in.Modality {
	case ModalityLoan:
		if in.PaymentMode != PaymentNone {
			add("paymentMode", "un préstamo no tiene pagos: usa none")
		}
	default:
		if in.PaymentMode != PaymentFullOnDelivery && in.PaymentMode != PaymentSplit5050 {
			add("paymentMode", "en una renta debe ser full_on_delivery o split_50_50")
		}
	}
	if len(d) > 0 {
		return Quote{}, shared.NewValidation(MessageInvalidData, d...)
	}
	q := Quote{
		ItemID: in.ItemID, Start: start, End: end, Days: days, Modality: in.Modality,
		PricePerDayCents: in.PricePerDayCents, PaymentMode: in.PaymentMode,
	}
	if in.Modality == ModalityLoan {
		q.PricePerDayCents = policy.POL01LoanPriceCents
	}
	q.TotalCents = int64(days) * q.PricePerDayCents
	q.Payments = SplitPayments(q.TotalCents, q.PaymentMode)
	return q, nil
}

// SplitPayments reparte el total según POL-05. Un préstamo no tiene pagos.
func SplitPayments(total int64, mode PaymentMode) []Payment {
	switch mode {
	case PaymentFullOnDelivery:
		return []Payment{{StageDelivery, total}}
	case PaymentSplit5050:
		return []Payment{{StageDelivery, (total + 1) / 2}, {StageReturn, total / 2}}
	default:
		return []Payment{}
	}
}
