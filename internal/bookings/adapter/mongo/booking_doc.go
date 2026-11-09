// Package mongo implementa el repositorio de reservas con MongoDB (spec 007,
// T004–T005, T009; plan §3, P-03). Es el único paquete del módulo bookings que
// importa el driver. Lee items en modo lectura salvo el contador
// reservationSeq de la transacción de aceptar.
package mongo

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// Colecciones.
const (
	BookingsCollection = "bookings"
	ItemsCollection    = "items"
)

type imageDoc struct {
	PublicID string `bson:"publicId"`
	Width    int    `bson:"width"`
	Height   int    `bson:"height"`
	Format   string `bson:"format"`
	Bytes    int64  `bson:"bytes"`
	Delivery string `bson:"delivery"`
}

func imageToDoc(i *images.Image) *imageDoc {
	if i == nil {
		return nil
	}
	return &imageDoc{PublicID: i.PublicID, Width: i.Width, Height: i.Height, Format: i.Format, Bytes: i.Bytes, Delivery: string(i.Delivery)}
}

func (d *imageDoc) toDomain() *images.Image {
	if d == nil {
		return nil
	}
	return &images.Image{PublicID: d.PublicID, Width: d.Width, Height: d.Height, Format: d.Format, Bytes: d.Bytes, Delivery: images.Delivery(d.Delivery)}
}

type itemSnapshotDoc struct {
	Title      string    `bson:"title"`
	UsageRules *string   `bson:"usageRules"`
	Modality   string    `bson:"modality"`
	FirstPhoto *imageDoc `bson:"firstPhoto"`
}

type priceSnapshotDoc struct {
	Modality         string `bson:"modality"`
	PricePerDayCents int64  `bson:"pricePerDayCents"`
	TotalCents       int64  `bson:"totalCents"`
	Currency         string `bson:"currency"`
}

type paymentDoc struct {
	Stage        string     `bson:"stage"`
	AmountCents  int64      `bson:"amountCents"`
	Status       string     `bson:"status"`
	ReportedAt   *time.Time `bson:"reportedAt"`
	ReportedNote *string    `bson:"reportedNote"`
	ConfirmedAt  *time.Time `bson:"confirmedAt"`
}

// qrDoc, handoverDoc y adjustmentDoc son los campos que escribe la spec 009
// (plan 009 §3). La 007 los lee y los vuelve a escribir tal cual cuando guarda
// la reserva completa, para no borrarlos.
type qrDoc struct {
	TokenHash       string     `bson:"tokenHash"`
	ExpiresAt       time.Time  `bson:"expiresAt"`
	UsedAt          *time.Time `bson:"usedAt"`
	EvidenceVersion int        `bson:"evidenceVersion"`
}

type handoverDoc struct {
	RecordedAt      *time.Time `bson:"recordedAt"`
	RecordedByID    *string    `bson:"recordedById"`
	ConditionNotes  string     `bson:"conditionNotes"`
	Photos          []imageDoc `bson:"photos"`
	EvidenceVersion int        `bson:"evidenceVersion"`
	QR              *qrDoc     `bson:"qr"`
	ConfirmedAt     *time.Time `bson:"confirmedAt"`
	ConfirmedByID   *string    `bson:"confirmedById"`
}

// EncodeHandover y EncodeQR dan a los repositorios de la spec 009 la misma
// representación BSON que usa la 007, para escribir sus campos con $set sin
// duplicar el modelo (plan 009, P-09).
func EncodeHandover(h domain.Handover) any { return handoverToDoc(&h) }

// EncodeQR es la representación BSON de un QR (sin el token, solo su hash).
func EncodeQR(q domain.QR) any {
	return &qrDoc{TokenHash: q.TokenHash, ExpiresAt: q.ExpiresAt.UTC(), UsedAt: utcPtr(q.UsedAt), EvidenceVersion: q.EvidenceVersion}
}

// EncodeAdjustment es la representación BSON de un ajuste por retraso.
func EncodeAdjustment(a domain.Adjustment) any {
	return &adjustmentDoc{Type: a.Type, LateDays: a.LateDays, AmountCents: a.AmountCents, Status: a.Status}
}

type adjustmentDoc struct {
	Type        string `bson:"type"`
	LateDays    int    `bson:"lateDays"`
	AmountCents int64  `bson:"amountCents"`
	Status      string `bson:"status"`
}

func handoverToDoc(h *domain.Handover) *handoverDoc {
	if h == nil {
		return nil
	}
	d := &handoverDoc{
		RecordedAt: utcPtr(h.RecordedAt), RecordedByID: h.RecordedByID, ConditionNotes: h.ConditionNotes,
		Photos: make([]imageDoc, 0, len(h.Photos)), EvidenceVersion: h.EvidenceVersion,
		ConfirmedAt: utcPtr(h.ConfirmedAt), ConfirmedByID: h.ConfirmedByID,
	}
	for i := range h.Photos {
		d.Photos = append(d.Photos, *imageToDoc(&h.Photos[i]))
	}
	if h.QR != nil {
		d.QR = &qrDoc{TokenHash: h.QR.TokenHash, ExpiresAt: h.QR.ExpiresAt.UTC(), UsedAt: utcPtr(h.QR.UsedAt), EvidenceVersion: h.QR.EvidenceVersion}
	}
	return d
}

func (d *handoverDoc) toDomain() *domain.Handover {
	if d == nil {
		return nil
	}
	h := &domain.Handover{
		RecordedAt: utcPtr(d.RecordedAt), RecordedByID: d.RecordedByID, ConditionNotes: d.ConditionNotes,
		Photos: make([]images.Image, 0, len(d.Photos)), EvidenceVersion: d.EvidenceVersion,
		ConfirmedAt: utcPtr(d.ConfirmedAt), ConfirmedByID: d.ConfirmedByID,
	}
	for i := range d.Photos {
		h.Photos = append(h.Photos, *d.Photos[i].toDomain())
	}
	if d.QR != nil {
		h.QR = &domain.QR{TokenHash: d.QR.TokenHash, ExpiresAt: d.QR.ExpiresAt.UTC(), UsedAt: utcPtr(d.QR.UsedAt), EvidenceVersion: d.QR.EvidenceVersion}
	}
	return h
}

type historyDoc struct {
	Status string    `bson:"status"`
	At     time.Time `bson:"at"`
	By     string    `bson:"by"`
}

// bookingDoc es el documento de bookings (plan §3). itemId, ownerId y
// requesterId son texto hexadecimal (como items.ownerId; la búsqueda de la
// 006 compara itemId así); startDate y endDate, cadenas YYYY-MM-DD.
type bookingDoc struct {
	ID             bson.ObjectID    `bson:"_id"`
	ItemID         string           `bson:"itemId"`
	OwnerID        string           `bson:"ownerId"`
	RequesterID    string           `bson:"requesterId"`
	StartDate      string           `bson:"startDate"`
	EndDate        string           `bson:"endDate"`
	Days           int              `bson:"days"`
	ItemSnapshot   itemSnapshotDoc  `bson:"itemSnapshot"`
	PriceSnapshot  priceSnapshotDoc `bson:"priceSnapshot"`
	PaymentMode    string           `bson:"paymentMode"`
	Payments       []paymentDoc     `bson:"payments"`
	Status         string           `bson:"status"`
	StatusHistory  []historyDoc     `bson:"statusHistory"`
	ExpiresAt      *time.Time       `bson:"expiresAt"`
	MeetingPoint   *string          `bson:"meetingPoint"`
	CancelReason   *string          `bson:"cancelReason"`
	CancelledBy    *string          `bson:"cancelledBy"`
	RejectReason   *string          `bson:"rejectReason"`
	ConversationID *string          `bson:"conversationId"`
	Delivery       *handoverDoc     `bson:"delivery"`
	Return         *handoverDoc     `bson:"return"`
	IsLate         bool             `bson:"isLate"`
	LateAt         *time.Time       `bson:"lateAt"`
	Adjustments    []adjustmentDoc  `bson:"adjustments"`
	AffectedBy     *string          `bson:"affectedBy"`
	CreatedAt      time.Time        `bson:"createdAt"`
	UpdatedAt      time.Time        `bson:"updatedAt"`
	Version        int64            `bson:"version"`
}

func toDoc(b domain.Booking) (bookingDoc, error) {
	oid, err := bson.ObjectIDFromHex(b.ID)
	if err != nil {
		return bookingDoc{}, fmt.Errorf("bookings: id inválido: %w", err)
	}
	d := bookingDoc{
		ID: oid, ItemID: b.ItemID, OwnerID: b.OwnerID, RequesterID: b.RequesterID,
		StartDate: b.Start.String(), EndDate: b.End.String(), Days: b.Days,
		ItemSnapshot: itemSnapshotDoc{
			Title: b.Item.Title, UsageRules: b.Item.UsageRules, Modality: string(b.Item.Modality),
			FirstPhoto: imageToDoc(b.Item.FirstPhoto),
		},
		PriceSnapshot: priceSnapshotDoc{
			Modality: string(b.Price.Modality), PricePerDayCents: b.Price.PricePerDayCents,
			TotalCents: b.Price.TotalCents, Currency: b.Price.Currency,
		},
		PaymentMode:    string(b.PaymentMode),
		Payments:       make([]paymentDoc, 0, len(b.Payments)),
		Status:         string(b.Status),
		StatusHistory:  make([]historyDoc, 0, len(b.History)),
		ExpiresAt:      utcPtr(b.ExpiresAt),
		MeetingPoint:   b.MeetingPoint,
		CancelReason:   b.CancelReason,
		RejectReason:   b.RejectReason,
		ConversationID: b.ConversationID,
		Delivery:       handoverToDoc(b.Delivery),
		Return:         handoverToDoc(b.Return),
		IsLate:         b.IsLate,
		LateAt:         utcPtr(b.LateAt),
		Adjustments:    make([]adjustmentDoc, 0, len(b.Adjustments)),
		AffectedBy:     b.AffectedBy,
		CreatedAt:      b.CreatedAt.UTC(),
		UpdatedAt:      b.UpdatedAt.UTC(),
		Version:        b.Version,
	}
	if b.CancelledBy != nil {
		s := string(*b.CancelledBy)
		d.CancelledBy = &s
	}
	for _, p := range b.Payments {
		d.Payments = append(d.Payments, paymentDoc{
			Stage: string(p.Stage), AmountCents: p.AmountCents, Status: string(p.Status),
			ReportedAt: utcPtr(p.ReportedAt), ReportedNote: p.ReportedNote, ConfirmedAt: utcPtr(p.ConfirmedAt),
		})
	}
	for _, a := range b.Adjustments {
		d.Adjustments = append(d.Adjustments, adjustmentDoc{Type: a.Type, LateDays: a.LateDays, AmountCents: a.AmountCents, Status: a.Status})
	}
	for _, h := range b.History {
		d.StatusHistory = append(d.StatusHistory, historyDoc{Status: string(h.Status), At: h.At.UTC(), By: string(h.By)})
	}
	return d, nil
}

func (d bookingDoc) toDomain() domain.Booking {
	b := domain.Booking{
		ID: d.ID.Hex(), ItemID: d.ItemID, OwnerID: d.OwnerID, RequesterID: d.RequesterID,
		Start: domain.MustDate(d.StartDate), End: domain.MustDate(d.EndDate), Days: d.Days,
		Item: domain.ItemSnapshot{
			Title: d.ItemSnapshot.Title, UsageRules: d.ItemSnapshot.UsageRules,
			Modality: domain.Modality(d.ItemSnapshot.Modality), FirstPhoto: d.ItemSnapshot.FirstPhoto.toDomain(),
		},
		Price: domain.PriceSnapshot{
			Modality: domain.Modality(d.PriceSnapshot.Modality), PricePerDayCents: d.PriceSnapshot.PricePerDayCents,
			TotalCents: d.PriceSnapshot.TotalCents, Currency: d.PriceSnapshot.Currency,
		},
		PaymentMode:    domain.PaymentMode(d.PaymentMode),
		Payments:       make([]domain.BookingPayment, 0, len(d.Payments)),
		Status:         domain.Status(d.Status),
		History:        make([]domain.HistoryEntry, 0, len(d.StatusHistory)),
		ExpiresAt:      utcPtr(d.ExpiresAt),
		MeetingPoint:   d.MeetingPoint,
		CancelReason:   d.CancelReason,
		RejectReason:   d.RejectReason,
		ConversationID: d.ConversationID,
		Delivery:       d.Delivery.toDomain(),
		Return:         d.Return.toDomain(),
		IsLate:         d.IsLate,
		LateAt:         utcPtr(d.LateAt),
		Adjustments:    make([]domain.Adjustment, 0, len(d.Adjustments)),
		AffectedBy:     d.AffectedBy,
		CreatedAt:      d.CreatedAt.UTC(),
		UpdatedAt:      d.UpdatedAt.UTC(),
		Version:        d.Version,
	}
	if d.CancelledBy != nil {
		r := domain.Role(*d.CancelledBy)
		b.CancelledBy = &r
	}
	for _, p := range d.Payments {
		b.Payments = append(b.Payments, domain.BookingPayment{
			Stage: domain.Stage(p.Stage), AmountCents: p.AmountCents, Status: domain.PaymentStatus(p.Status),
			ReportedAt: utcPtr(p.ReportedAt), ReportedNote: p.ReportedNote, ConfirmedAt: utcPtr(p.ConfirmedAt),
		})
	}
	for _, a := range d.Adjustments {
		b.Adjustments = append(b.Adjustments, domain.Adjustment{Type: a.Type, LateDays: a.LateDays, AmountCents: a.AmountCents, Status: a.Status})
	}
	for _, h := range d.StatusHistory {
		b.History = append(b.History, domain.HistoryEntry{Status: domain.Status(h.Status), At: h.At.UTC(), By: domain.Role(h.By)})
	}
	return b
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
