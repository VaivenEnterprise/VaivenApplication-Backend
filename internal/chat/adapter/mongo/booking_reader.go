package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// BookingReader lee de bookings solo el estado, las partes y los datos del
// objeto (solo lectura, plan P-03). Cumple usecase.Bookings.
type BookingReader struct{ col *drv.Collection }

// NewBookingReader crea el lector sobre db.
func NewBookingReader(db *drv.Database) BookingReader {
	return BookingReader{col: db.Collection(BookingsCollection)}
}

type bookingInfoDoc struct {
	ID           bson.ObjectID `bson:"_id"`
	Status       string        `bson:"status"`
	OwnerID      string        `bson:"ownerId"`
	RequesterID  string        `bson:"requesterId"`
	ItemID       string        `bson:"itemId"`
	ItemSnapshot struct {
		Title      string `bson:"title"`
		FirstPhoto *struct {
			PublicID string `bson:"publicId"`
			Width    int    `bson:"width"`
			Height   int    `bson:"height"`
			Format   string `bson:"format"`
			Bytes    int64  `bson:"bytes"`
			Delivery string `bson:"delivery"`
		} `bson:"firstPhoto"`
	} `bson:"itemSnapshot"`
}

// ByIDs devuelve las reservas por id; las inexistentes no aparecen.
func (b BookingReader) ByIDs(ctx context.Context, ids []string) (map[string]domain.BookingInfo, error) {
	out := map[string]domain.BookingInfo{}
	oids := make(bson.A, 0, len(ids))
	for _, id := range ids {
		if oid, err := bson.ObjectIDFromHex(id); err == nil {
			oids = append(oids, oid)
		}
	}
	if len(oids) == 0 {
		return out, nil
	}
	cur, err := b.col.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: oids}}}},
		options.Find().SetProjection(bson.D{
			{Key: "status", Value: 1}, {Key: "ownerId", Value: 1}, {Key: "requesterId", Value: 1}, {Key: "itemId", Value: 1},
			{Key: "itemSnapshot.title", Value: 1}, {Key: "itemSnapshot.firstPhoto", Value: 1},
		}))
	if err != nil {
		return nil, fmt.Errorf("chat: leer reservas: %w", err)
	}
	var docs []bookingInfoDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("chat: leer reservas: %w", err)
	}
	for _, d := range docs {
		info := domain.BookingInfo{
			ID: d.ID.Hex(), Status: d.Status, OwnerID: d.OwnerID, RequesterID: d.RequesterID,
			ItemID: d.ItemID, ItemTitle: d.ItemSnapshot.Title,
		}
		if p := d.ItemSnapshot.FirstPhoto; p != nil {
			info.ItemPhoto = &images.Image{PublicID: p.PublicID, Width: p.Width, Height: p.Height, Format: p.Format, Bytes: p.Bytes, Delivery: images.Delivery(p.Delivery)}
		}
		out[info.ID] = info
	}
	return out, nil
}

// AccountReader indica qué cuentas siguen activas (RF-07; solo lectura de
// users). Cumple usecase.Accounts.
type AccountReader struct{ col *drv.Collection }

// NewAccountReader crea el lector sobre db.
func NewAccountReader(db *drv.Database) AccountReader {
	return AccountReader{col: db.Collection(UsersCollection)}
}

// Active devuelve, por id, si la cuenta existe y está activa.
func (a AccountReader) Active(ctx context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	oids := make(bson.A, 0, len(ids))
	for _, id := range ids {
		if oid, err := bson.ObjectIDFromHex(id); err == nil {
			oids = append(oids, oid)
		}
	}
	if len(oids) == 0 {
		return out, nil
	}
	cur, err := a.col.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: oids}}}, {Key: "status", Value: "active"}},
		options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("chat: leer cuentas: %w", err)
	}
	var docs []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("chat: leer cuentas: %w", err)
	}
	for _, d := range docs {
		out[d.ID.Hex()] = true
	}
	return out, nil
}
