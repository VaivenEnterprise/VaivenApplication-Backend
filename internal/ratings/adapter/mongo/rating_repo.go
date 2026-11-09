// Package mongo implementa el repositorio de calificaciones con MongoDB (spec
// 010, T003; plan §3, P-01). Es el único paquete del módulo que importa el
// driver. Suma el puntaje al perfil de la persona calificada mediante el puerto
// users.Admin (spec 003), nunca escribiendo en users directamente.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// RatingsCollection es la colección de calificaciones.
const RatingsCollection = "ratings"

// Repo guarda las calificaciones. Cumple usecase.Repository.
type Repo struct {
	client *drv.Client
	col    *drv.Collection
	admin  users.Admin
}

// NewRepo crea el repositorio sobre db (replica set: usa transacciones). admin
// es el puerto que suma la calificación al perfil; debe usar el mismo cliente
// de MongoDB para participar en la transacción.
func NewRepo(db *drv.Database, admin users.Admin) *Repo {
	return &Repo{client: db.Client(), col: db.Collection(RatingsCollection), admin: admin}
}

// EnsureIndexes crea los índices de ratings (plan §3): el único
// {bookingId, raterId}, que impide calificar dos veces la misma reserva, y
// {rateeId, _id desc} para listar las recibidas.
func (r *Repo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, []drv.IndexModel{
		{Keys: bson.D{{Key: "bookingId", Value: 1}, {Key: "raterId", Value: 1}}, Options: options.Index().SetName("booking_rater_unique").SetUnique(true)},
		{Keys: bson.D{{Key: "rateeId", Value: 1}, {Key: "_id", Value: -1}}, Options: options.Index().SetName("ratee_id")},
	})
	if err != nil {
		return fmt.Errorf("ratings: crear índices: %w", err)
	}
	return nil
}

// ratingDoc es el documento de ratings (plan §3 y Art. VI.4 de §6 de la
// constitución: createdAt y updatedAt).
type ratingDoc struct {
	ID        bson.ObjectID `bson:"_id"`
	BookingID string        `bson:"bookingId"`
	RaterID   string        `bson:"raterId"`
	RateeID   string        `bson:"rateeId"`
	RatedRole string        `bson:"ratedRole"`
	Score     int           `bson:"score"`
	Comment   *string       `bson:"comment"`
	CreatedAt time.Time     `bson:"createdAt"`
	UpdatedAt time.Time     `bson:"updatedAt"`
}

func (d ratingDoc) toDomain() domain.Rating {
	return domain.Rating{
		ID: d.ID.Hex(), BookingID: d.BookingID, RaterID: d.RaterID, RateeID: d.RateeID,
		RatedRole: bookings.Role(d.RatedRole), Score: d.Score, Comment: d.Comment, CreatedAt: d.CreatedAt.UTC(),
	}
}

// NewID genera el identificador de una calificación.
func (r *Repo) NewID() string { return bson.NewObjectID().Hex() }

// Create inserta la calificación y suma su puntaje al perfil de la persona
// calificada en una transacción (plan P-01): si falla una de las dos cosas no
// queda ninguna. Dos calificaciones iguales a la vez chocan con el índice
// único: una gana y la otra recibe domain.ErrAlreadyRated (RNF-01).
func (r *Repo) Create(ctx context.Context, rt domain.Rating) (domain.Rating, error) {
	oid, err := bson.ObjectIDFromHex(rt.ID)
	if err != nil {
		return domain.Rating{}, fmt.Errorf("ratings: id inválido: %w", err)
	}
	doc := ratingDoc{
		ID: oid, BookingID: rt.BookingID, RaterID: rt.RaterID, RateeID: rt.RateeID, RatedRole: string(rt.RatedRole),
		Score: rt.Score, Comment: rt.Comment, CreatedAt: rt.CreatedAt.UTC(), UpdatedAt: rt.CreatedAt.UTC(),
	}
	sess, err := r.client.StartSession()
	if err != nil {
		return domain.Rating{}, fmt.Errorf("ratings: iniciar sesión de MongoDB: %w", err)
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(tx context.Context) (any, error) {
		if _, err := r.col.InsertOne(tx, doc); err != nil {
			if drv.IsDuplicateKeyError(err) {
				return nil, domain.ErrAlreadyRated
			}
			return nil, err
		}
		return nil, r.admin.AddRating(tx, doc.RateeID, doc.Score)
	})
	if err != nil {
		if errors.Is(err, domain.ErrAlreadyRated) {
			return domain.Rating{}, domain.ErrAlreadyRated
		}
		return domain.Rating{}, fmt.Errorf("ratings: guardar calificación: %w", err)
	}
	return doc.toDomain(), nil
}

// ByBookingAndRater busca la calificación que raterID dio a la reserva.
func (r *Repo) ByBookingAndRater(ctx context.Context, bookingID, raterID string) (domain.Rating, bool, error) {
	var d ratingDoc
	err := r.col.FindOne(ctx, bson.D{{Key: "bookingId", Value: bookingID}, {Key: "raterId", Value: raterID}}).Decode(&d)
	if errors.Is(err, drv.ErrNoDocuments) {
		return domain.Rating{}, false, nil
	}
	if err != nil {
		return domain.Rating{}, false, fmt.Errorf("ratings: buscar calificación: %w", err)
	}
	return d.toDomain(), true, nil
}

// ListByRatee devuelve las calificaciones recibidas por rateeID, las más
// recientes primero (por _id), con _id menor que after ("" = desde el inicio).
func (r *Repo) ListByRatee(ctx context.Context, rateeID, after string, limit int) ([]domain.Rating, error) {
	f := bson.D{{Key: "rateeId", Value: rateeID}}
	if after != "" {
		oid, err := bson.ObjectIDFromHex(after)
		if err != nil {
			return nil, fmt.Errorf("ratings: cursor inválido: %w", err)
		}
		f = append(f, bson.E{Key: "_id", Value: bson.D{{Key: "$lt", Value: oid}}})
	}
	cur, err := r.col.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("ratings: listar calificaciones: %w", err)
	}
	var docs []ratingDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("ratings: leer calificaciones: %w", err)
	}
	out := make([]domain.Rating, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}
