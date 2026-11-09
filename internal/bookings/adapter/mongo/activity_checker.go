package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// UserActivity responde si una persona tiene reservas aceptadas o en curso,
// como propietaria o solicitante (RF-14). Cumple
// auth/usecase.ActiveBookingChecker: impide la baja de la cuenta (003).
type UserActivity struct{ col *drv.Collection }

// ItemActivity responde si un objeto tiene reservas aceptadas o en curso
// (RF-14). Cumple items/usecase.BookingChecker: impide eliminarlo (005).
type ItemActivity struct{ col *drv.Collection }

// NewUserActivity crea el verificador por persona.
func NewUserActivity(db *drv.Database) UserActivity {
	return UserActivity{col: db.Collection(BookingsCollection)}
}

// NewItemActivity crea el verificador por objeto.
func NewItemActivity(db *drv.Database) ItemActivity {
	return ItemActivity{col: db.Collection(BookingsCollection)}
}

// HasActiveBookings implementa el puerto de la spec 003.
func (u UserActivity) HasActiveBookings(ctx context.Context, userID string) (bool, error) {
	return exists(ctx, u.col, bson.D{
		{Key: "status", Value: bson.D{{Key: "$in", Value: blocking()}}},
		{Key: "$or", Value: bson.A{bson.D{{Key: "ownerId", Value: userID}}, bson.D{{Key: "requesterId", Value: userID}}}},
	})
}

// HasActiveBookings implementa el puerto de la spec 005.
func (i ItemActivity) HasActiveBookings(ctx context.Context, itemID string) (bool, error) {
	return exists(ctx, i.col, bson.D{
		{Key: "itemId", Value: itemID},
		{Key: "status", Value: bson.D{{Key: "$in", Value: blocking()}}},
	})
}

func exists(ctx context.Context, col *drv.Collection, f bson.D) (bool, error) {
	n, err := col.CountDocuments(ctx, f, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("bookings: consultar reservas activas: %w", err)
	}
	return n > 0, nil
}
