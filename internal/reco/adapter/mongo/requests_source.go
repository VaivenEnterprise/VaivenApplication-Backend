package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// bookingsCollection es la colección de la spec 007 (dueño BE-A). Aquí solo
// se lee: requesterId, itemId, status y createdAt (plan P-05).
const bookingsCollection = "bookings"

// activeBookingStatuses son las reservas que excluyen un objeto de las
// recomendaciones de quien lo pidió (RF-05).
var activeBookingStatuses = bson.A{"pending", "accepted", "in_progress"}

type requestDoc struct {
	RequesterID string    `bson:"requesterId"`
	ItemID      string    `bson:"itemId"`
	CreatedAt   time.Time `bson:"createdAt"`
}

func (d requestDoc) toDomain() domain.Event {
	return domain.Event{UserID: d.RequesterID, ItemID: d.ItemID, Type: domain.EventRequest, Weight: domain.WeightRequest, At: d.CreatedAt.UTC()}
}

var requestProjection = bson.D{{Key: "_id", Value: 0}, {Key: "requesterId", Value: 1}, {Key: "itemId", Value: 1}, {Key: "createdAt", Value: 1}}

// RequestSource lee las solicitudes de reserva como interacciones de peso
// domain.WeightRequest. Nunca escribe en bookings.
type RequestSource struct {
	col *drv.Collection
}

// NewRequestSource crea la lectura sobre db.
func NewRequestSource(db *drv.Database) *RequestSource {
	return &RequestSource{col: db.Collection(bookingsCollection)}
}

// Since devuelve hasta limit solicitudes creadas desde since, las más
// recientes primero (recálculo, cada 6 h como máximo). No es una consulta
// frecuente y no se agrega un índice a una colección de BE-A (plan §2).
func (s *RequestSource) Since(ctx context.Context, since time.Time, limit int) ([]domain.Event, error) {
	opts := options.Find().SetProjection(requestProjection).SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(int64(limit))
	return s.find(ctx, bson.D{{Key: "createdAt", Value: bson.D{{Key: "$gte", Value: since.UTC()}}}}, opts)
}

// Recent devuelve las limit solicitudes más recientes de userID creadas desde
// since (since cero = sin límite inferior). Usa el índice de la 007
// requester_status_createdAt por su prefijo requesterId.
func (s *RequestSource) Recent(ctx context.Context, userID string, since time.Time, limit int) ([]domain.Event, error) {
	filter := bson.D{{Key: "requesterId", Value: userID}}
	if !since.IsZero() {
		filter = append(filter, bson.E{Key: "createdAt", Value: bson.D{{Key: "$gt", Value: since.UTC()}}})
	}
	opts := options.Find().SetProjection(requestProjection).SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(int64(limit))
	return s.find(ctx, filter, opts)
}

// ActiveItemIDs devuelve los objetos que userID tiene en una reserva activa
// (pendiente, aceptada o en curso).
func (s *RequestSource) ActiveItemIDs(ctx context.Context, userID string) ([]string, error) {
	cur, err := s.col.Find(ctx,
		bson.D{{Key: "requesterId", Value: userID}, {Key: "status", Value: bson.D{{Key: "$in", Value: activeBookingStatuses}}}},
		options.Find().SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "itemId", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("bookings: leer reservas activas: %w", err)
	}
	var docs []requestDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("bookings: leer reservas activas: %w", err)
	}
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.ItemID)
	}
	return out, nil
}

func (s *RequestSource) find(ctx context.Context, filter bson.D, opts *options.FindOptionsBuilder) ([]domain.Event, error) {
	cur, err := s.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("bookings: leer solicitudes: %w", err)
	}
	var docs []requestDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("bookings: leer solicitudes: %w", err)
	}
	out := make([]domain.Event, len(docs))
	for i, d := range docs {
		out[i] = d.toDomain()
	}
	return out, nil
}
