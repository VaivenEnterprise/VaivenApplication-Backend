package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// Repo guarda las reservas y lee los objetos que necesita.
type Repo struct {
	client   *drv.Client
	bookings *drv.Collection
	items    *drv.Collection
}

// NewRepo crea el repositorio sobre db.
func NewRepo(db *drv.Database) *Repo {
	return &Repo{client: db.Client(), bookings: db.Collection(BookingsCollection), items: db.Collection(ItemsCollection)}
}

// blocking son los estados que ocupan el periodo (RF-03).
func blocking() bson.A { return bson.A{string(domain.StatusAccepted), string(domain.StatusInProgress)} }

// EnsureIndexes crea los 4 índices del plan §3. Es idempotente.
func (r *Repo) EnsureIndexes(ctx context.Context) error {
	_, err := r.bookings.Indexes().CreateMany(ctx, []drv.IndexModel{
		{Keys: bson.D{{Key: "itemId", Value: 1}, {Key: "status", Value: 1}, {Key: "startDate", Value: 1}, {Key: "endDate", Value: 1}},
			Options: options.Index().SetName("item_status_dates")},
		{Keys: bson.D{{Key: "requesterId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("requester_status_createdAt")},
		{Keys: bson.D{{Key: "ownerId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("owner_status_createdAt")},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "expiresAt", Value: 1}},
			Options: options.Index().SetName("status_expiresAt")},
	})
	if err != nil {
		return fmt.Errorf("bookings: crear índices: %w", err)
	}
	return nil
}

// NewID genera el id de una solicitud antes de guardarla (la conversación
// se abre con él, plan P-06).
func (r *Repo) NewID() string { return bson.NewObjectID().Hex() }

// Create inserta una solicitud.
func (r *Repo) Create(ctx context.Context, b domain.Booking) error {
	d, err := toDoc(b)
	if err != nil {
		return err
	}
	if _, err := r.bookings.InsertOne(ctx, d); err != nil {
		return fmt.Errorf("bookings: insertar: %w", err)
	}
	return nil
}

// ByID busca por id; si no existe o el id es inválido, domain.ErrNotFound.
func (r *Repo) ByID(ctx context.Context, id string) (domain.Booking, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Booking{}, domain.ErrNotFound
	}
	var d bookingDoc
	if err := r.bookings.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&d); err != nil {
		if errors.Is(err, drv.ErrNoDocuments) {
			return domain.Booking{}, domain.ErrNotFound
		}
		return domain.Booking{}, fmt.Errorf("bookings: buscar: %w", err)
	}
	return d.toDomain(), nil
}

// Update reemplaza la reserva si su versión guardada sigue siendo b.Version y
// devuelve la reserva con la versión incrementada; si no,
// domain.ErrVersionConflict.
func (r *Repo) Update(ctx context.Context, b domain.Booking) (domain.Booking, error) {
	return r.replace(ctx, r.bookings, b)
}

func (r *Repo) replace(ctx context.Context, col *drv.Collection, b domain.Booking) (domain.Booking, error) {
	d, err := toDoc(b)
	if err != nil {
		return domain.Booking{}, domain.ErrNotFound
	}
	expected := d.Version
	d.Version = expected + 1
	res, err := col.ReplaceOne(ctx, bson.D{{Key: "_id", Value: d.ID}, {Key: "version", Value: expected}}, d)
	if err != nil {
		return domain.Booking{}, fmt.Errorf("bookings: actualizar: %w", err)
	}
	if res.MatchedCount == 0 {
		return domain.Booking{}, domain.ErrVersionConflict
	}
	return d.toDomain(), nil
}

func overlapFilter(itemID string, start, end domain.Date) bson.D {
	return bson.D{
		{Key: "itemId", Value: itemID},
		{Key: "startDate", Value: bson.D{{Key: "$lte", Value: end.String()}}},
		{Key: "endDate", Value: bson.D{{Key: "$gte", Value: start.String()}}},
	}
}

// HasBlockingOverlap indica si el objeto tiene una reserva aceptada o en
// curso que se cruce con [start, end] (RF-03).
func (r *Repo) HasBlockingOverlap(ctx context.Context, itemID string, start, end domain.Date) (bool, error) {
	f := append(overlapFilter(itemID, start, end), bson.E{Key: "status", Value: bson.D{{Key: "$in", Value: blocking()}}})
	n, err := r.bookings.CountDocuments(ctx, f, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("bookings: buscar cruces: %w", err)
	}
	return n > 0, nil
}

// HasPendingOverlap indica si requesterID ya tiene una solicitud pendiente y
// vigente del objeto que se cruce (RF-03).
func (r *Repo) HasPendingOverlap(ctx context.Context, itemID, requesterID string, start, end domain.Date, now time.Time) (bool, error) {
	f := append(overlapFilter(itemID, start, end),
		bson.E{Key: "status", Value: string(domain.StatusPending)},
		bson.E{Key: "requesterId", Value: requesterID},
		bson.E{Key: "expiresAt", Value: bson.D{{Key: "$gt", Value: now.UTC()}}},
	)
	n, err := r.bookings.CountDocuments(ctx, f, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("bookings: buscar pendientes: %w", err)
	}
	return n > 0, nil
}

// ListByParty devuelve las reservas de userID como owner o requester, con
// un estado opcional, más recientes primero, desde after (exclusivo).
func (r *Repo) ListByParty(ctx context.Context, userID string, role domain.Role, status *domain.Status, after *domain.Cursor, limit int) ([]domain.Booking, error) {
	field := "requesterId"
	if role == domain.RoleOwner {
		field = "ownerId"
	}
	f := bson.D{{Key: field, Value: userID}}
	if status != nil {
		f = append(f, bson.E{Key: "status", Value: string(*status)})
	}
	if after != nil {
		oid, err := bson.ObjectIDFromHex(after.ID)
		if err != nil {
			return nil, fmt.Errorf("bookings: cursor inválido: %w", err)
		}
		at := after.CreatedAt.UTC()
		f = append(f, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "createdAt", Value: bson.D{{Key: "$lt", Value: at}}}},
			bson.D{{Key: "createdAt", Value: at}, {Key: "_id", Value: bson.D{{Key: "$lt", Value: oid}}}},
		}})
	}
	cur, err := r.bookings.Find(ctx, f, options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("bookings: listar: %w", err)
	}
	var docs []bookingDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("bookings: leer listado: %w", err)
	}
	out := make([]domain.Booking, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

// DuePending devuelve hasta limit solicitudes pendientes cuyo vencimiento ya
// pasó (plan P-04). El caso de uso las pasa a expired con Update.
func (r *Repo) DuePending(ctx context.Context, now time.Time, limit int) ([]domain.Booking, error) {
	cur, err := r.bookings.Find(ctx, bson.D{
		{Key: "status", Value: string(domain.StatusPending)},
		{Key: "expiresAt", Value: bson.D{{Key: "$lte", Value: now.UTC()}}},
	}, options.Find().SetSort(bson.D{{Key: "expiresAt", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("bookings: buscar vencidas: %w", err)
	}
	var docs []bookingDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("bookings: leer vencidas: %w", err)
	}
	out := make([]domain.Booking, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

// itemDoc son los campos de items que usa una solicitud.
type itemDoc struct {
	ID               bson.ObjectID `bson:"_id"`
	OwnerID          string        `bson:"ownerId"`
	Status           string        `bson:"status"`
	Title            string        `bson:"title"`
	UsageRules       *string       `bson:"usageRules"`
	Modality         string        `bson:"modality"`
	PricePerDayCents int64         `bson:"pricePerDayCents"`
	Photos           []imageDoc    `bson:"photos"`
}

// ItemForBooking lee el objeto (solo lectura). Un objeto removed o
// inexistente es domain.ErrItemNotFound.
func (r *Repo) ItemForBooking(ctx context.Context, itemID string) (domain.ItemForBooking, error) {
	oid, err := bson.ObjectIDFromHex(itemID)
	if err != nil {
		return domain.ItemForBooking{}, domain.ErrItemNotFound
	}
	var d itemDoc
	err = r.items.FindOne(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "status", Value: bson.D{{Key: "$ne", Value: "removed"}}}},
		options.FindOne().SetProjection(bson.D{
			{Key: "ownerId", Value: 1}, {Key: "status", Value: 1}, {Key: "title", Value: 1}, {Key: "usageRules", Value: 1},
			{Key: "modality", Value: 1}, {Key: "pricePerDayCents", Value: 1}, {Key: "photos", Value: bson.D{{Key: "$slice", Value: 1}}},
		})).Decode(&d)
	if err != nil {
		if errors.Is(err, drv.ErrNoDocuments) {
			return domain.ItemForBooking{}, domain.ErrItemNotFound
		}
		return domain.ItemForBooking{}, fmt.Errorf("bookings: leer objeto: %w", err)
	}
	it := domain.ItemForBooking{
		ID: d.ID.Hex(), OwnerID: d.OwnerID, Status: d.Status, Title: d.Title, UsageRules: d.UsageRules,
		Modality: domain.Modality(d.Modality), PricePerDayCents: d.PricePerDayCents,
	}
	if len(d.Photos) > 0 {
		p := d.Photos[0]
		it.FirstPhoto = &images.Image{PublicID: p.PublicID, Width: p.Width, Height: p.Height, Format: p.Format, Bytes: p.Bytes, Delivery: images.Delivery(p.Delivery)}
	}
	return it, nil
}
