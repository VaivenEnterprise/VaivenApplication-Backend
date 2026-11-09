package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// Role es el papel de una persona en una reserva.
type Role string

// Papeles. RoleSystem es el vencimiento automático; RoleAdmin solo consulta.
const (
	RoleOwner     Role = "owner"
	RoleRequester Role = "requester"
	RoleAdmin     Role = "admin"
	RoleSystem    Role = "system"
)

// ItemSnapshot es la copia del objeto al solicitar (RF-04): un cambio
// posterior del objeto no la altera (CA-07).
type ItemSnapshot struct {
	Title      string
	UsageRules *string
	Modality   Modality
	FirstPhoto *images.Image
}

// PriceSnapshot es la tarifa y el total acordados.
type PriceSnapshot struct {
	Modality         Modality
	PricePerDayCents int64
	TotalCents       int64
	Currency         string
}

// PaymentStatus es el estado de un pago (lo avanza la spec 009).
type PaymentStatus string

// Estados de pago.
const PaymentPending PaymentStatus = "pending"

// BookingPayment es un pago esperado de la reserva.
type BookingPayment struct {
	Stage       Stage
	AmountCents int64
	Status      PaymentStatus
	ReportedAt  *time.Time
	ConfirmedAt *time.Time
}

// HistoryEntry es un cambio de estado.
type HistoryEntry struct {
	Status Status
	At     time.Time
	By     Role
}

// MaxHistory acota statusHistory (plan §3).
const MaxHistory = 10

// Límites de texto (RF-07, RF-08).
const (
	ReasonMin       = 3
	ReasonMax       = 200
	MeetingPointMin = 3
	MeetingPointMax = 200
	// RejectReasonUnavailable es el motivo de las pendientes que pierden el
	// periodo al aceptar otra (RF-07).
	RejectReasonUnavailable = "no_disponible"
)

// Booking es una solicitud y, si se acepta, la reserva.
type Booking struct {
	ID             string
	ItemID         string
	OwnerID        string
	RequesterID    string
	Start, End     Date
	Days           int
	Item           ItemSnapshot
	Price          PriceSnapshot
	PaymentMode    PaymentMode
	Payments       []BookingPayment
	Status         Status
	History        []HistoryEntry
	ExpiresAt      *time.Time
	MeetingPoint   *string
	CancelReason   *string
	CancelledBy    *Role
	ConversationID *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int64
}

// ItemForBooking son los datos del objeto que necesita una solicitud.
type ItemForBooking struct {
	ID               string
	OwnerID          string
	Status           string
	Title            string
	UsageRules       *string
	Modality         Modality
	PricePerDayCents int64
	FirstPhoto       *images.Image
}

// ExpiresAt calcula el vencimiento de una solicitud (POL-03, plan P-04): 48 h
// después de crearla o al empezar el día de inicio en México, lo primero.
func ExpiresAt(created time.Time, start Date, loc *time.Location) time.Time {
	byHours := created.Add(time.Duration(policy.POL03RequestExpiryHours) * time.Hour)
	if s := start.StartOf(loc); s.Before(byHours) {
		return s.UTC()
	}
	return byHours.UTC()
}

// NewRequest arma una solicitud pending a partir de la cotización y el
// objeto (RF-02 a RF-05). La regla de objeto propio y activo va primero.
func NewRequest(id, requesterID string, it ItemForBooking, q Quote, now time.Time, loc *time.Location) (Booking, error) {
	if it.OwnerID == requesterID {
		return Booking{}, ErrOwnItem
	}
	if it.Status != "active" {
		return Booking{}, ErrItemUnavailable
	}
	payments := make([]BookingPayment, 0, len(q.Payments))
	for _, p := range q.Payments {
		payments = append(payments, BookingPayment{Stage: p.Stage, AmountCents: p.AmountCents, Status: PaymentPending})
	}
	exp := ExpiresAt(now, q.Start, loc)
	return Booking{
		ID: id, ItemID: it.ID, OwnerID: it.OwnerID, RequesterID: requesterID,
		Start: q.Start, End: q.End, Days: q.Days,
		Item: ItemSnapshot{Title: it.Title, UsageRules: it.UsageRules, Modality: it.Modality, FirstPhoto: it.FirstPhoto},
		Price: PriceSnapshot{
			Modality: q.Modality, PricePerDayCents: q.PricePerDayCents, TotalCents: q.TotalCents, Currency: Currency,
		},
		PaymentMode: q.PaymentMode,
		Payments:    payments,
		Status:      StatusPending,
		History:     []HistoryEntry{{Status: StatusPending, At: now, By: RoleRequester}},
		ExpiresAt:   &exp,
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
	}, nil
}

// RoleOf devuelve el papel de userID, si es parte.
func (b Booking) RoleOf(userID string) (Role, bool) {
	switch {
	case userID == "":
		return "", false
	case userID == b.OwnerID:
		return RoleOwner, true
	case userID == b.RequesterID:
		return RoleRequester, true
	}
	return "", false
}

// EffectiveStatus presenta como expired una pendiente ya vencida aunque la
// tarea periódica aún no la haya cambiado (plan P-04).
func (b Booking) EffectiveStatus(now time.Time) Status {
	if b.Status == StatusPending && b.ExpiresAt != nil && !now.Before(*b.ExpiresAt) {
		return StatusExpired
	}
	return b.Status
}

// Overlaps indica si la reserva se cruza con el periodo [start, end].
func (b Booking) Overlaps(start, end Date) bool { return Overlaps(b.Start, b.End, start, end) }

// NormalizeText recorta un texto opcional y valida su longitud. Vacío = nil.
func NormalizeText(field string, s *string, minLen, maxLen int) (*string, error) {
	if s == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil, nil
	}
	if n := utf8.RuneCountInString(t); n < minLen || n > maxLen {
		return nil, shared.NewValidation(MessageInvalidData, shared.Detail{Field: field, Issue: lengthIssue(minLen, maxLen)})
	}
	return &t, nil
}

func lengthIssue(minLen, maxLen int) string {
	if minLen <= 1 {
		return "debe tener como máximo 200 caracteres"
	}
	return "debe tener entre 3 y 200 caracteres"
}
