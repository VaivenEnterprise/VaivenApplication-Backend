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
	Stage       string     `bson:"stage"`
	AmountCents int64      `bson:"amountCents"`
	Status      string     `bson:"status"`
	ReportedAt  *time.Time `bson:"reportedAt"`
	ConfirmedAt *time.Time `bson:"confirmedAt"`
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
			ReportedAt: utcPtr(p.ReportedAt), ConfirmedAt: utcPtr(p.ConfirmedAt),
		})
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
			ReportedAt: utcPtr(p.ReportedAt), ConfirmedAt: utcPtr(p.ConfirmedAt),
		})
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
