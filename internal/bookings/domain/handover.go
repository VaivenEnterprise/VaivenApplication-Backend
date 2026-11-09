package domain

import (
	"time"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// Datos de entrega, devolución, pagos y retraso que llenan las specs 009
// (plan 009 §3). Viven aquí, junto a Booking, porque se guardan dentro del
// mismo documento: así la 007 los conserva cuando guarda la reserva completa
// y no hay ciclos de importación. Las reglas están en handover/domain y
// payments/domain.

// QR es el código de un solo uso de una etapa. Solo se guarda el hash del
// token (RNF-01).
type QR struct {
	TokenHash       string
	ExpiresAt       time.Time
	UsedAt          *time.Time
	EvidenceVersion int
}

// Handover es una etapa presencial: la entrega o la devolución.
type Handover struct {
	RecordedAt      *time.Time
	RecordedByID    *string
	ConditionNotes  string
	Photos          []images.Image
	EvidenceVersion int
	QR              *QR
	ConfirmedAt     *time.Time
	ConfirmedByID   *string
}

// Adjustment es un ajuste registrado, nunca cobrado (RF-10).
type Adjustment struct {
	Type        string
	LateDays    int
	AmountCents int64
	Status      string
}

// Valores de Adjustment.
const (
	AdjustmentLateReturn = "late_return"
	AdjustmentRecorded   = "recorded"
)

// Estados de pago que avanza la spec 009 (PaymentPending lo fija la 007).
const (
	PaymentReported  PaymentStatus = "reported"
	PaymentConfirmed PaymentStatus = "confirmed"
)

// PaymentFor devuelve el pago de la etapa, si la reserva lo tiene.
func (b Booking) PaymentFor(stage Stage) (BookingPayment, bool) {
	for _, p := range b.Payments {
		if p.Stage == stage {
			return p, true
		}
	}
	return BookingPayment{}, false
}

// QRState es el estado del QR de una etapa tal como se muestra al cliente.
type QRState string

// Estados del QR.
const (
	QRNone    QRState = "none"
	QRActive  QRState = "active"
	QRExpired QRState = "expired"
	QRUsed    QRState = "used"
)

// QRStateAt devuelve el estado del QR de la etapa en el instante at. Una etapa
// nil, sin QR o con un QR de otra versión de evidencia no tiene QR vigente.
func (h *Handover) QRStateAt(at time.Time) QRState {
	if h == nil || h.QR == nil {
		return QRNone
	}
	switch {
	case h.QR.UsedAt != nil:
		return QRUsed
	case h.QR.EvidenceVersion != h.EvidenceVersion:
		return QRNone // la evidencia cambió: el QR ya no vale
	case !at.Before(h.QR.ExpiresAt):
		return QRExpired
	}
	return QRActive
}
