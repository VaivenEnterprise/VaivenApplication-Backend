// Package mongo implementa los repositorios de la cuenta con MongoDB (spec
// 003, plan §3; Art. II). Es el único paquete del módulo auth que importa el
// driver.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// UsersCollection es el nombre de la colección de cuentas.
const UsersCollection = "users"

// userDoc es el documento BSON de users; los campos coinciden con el JSON
// (camelCase, Art. 6.1 de la constitución).
type userDoc struct {
	ID              bson.ObjectID `bson:"_id,omitempty"`
	Email           string        `bson:"email"`
	PasswordHash    string        `bson:"passwordHash"`
	DisplayName     string        `bson:"displayName"`
	Phone           *string       `bson:"phone"`
	Role            string        `bson:"role"`
	Status          string        `bson:"status"`
	AcceptedTermsAt time.Time     `bson:"acceptedTermsAt"`
	RatingSum       int64         `bson:"ratingSum"`
	RatingCount     int64         `bson:"ratingCount"`
	SuspendedUntil  *time.Time    `bson:"suspendedUntil"`
	CreatedAt       time.Time     `bson:"createdAt"`
	UpdatedAt       time.Time     `bson:"updatedAt"`
	Version         int64         `bson:"version"`
	DeactivatedAt   *time.Time    `bson:"deactivatedAt"`
}

func toDoc(u domain.User) (userDoc, error) {
	d := userDoc{
		Email:           u.Email,
		PasswordHash:    u.PasswordHash,
		DisplayName:     u.DisplayName,
		Phone:           u.Phone,
		Role:            string(u.Role),
		Status:          string(u.Status),
		AcceptedTermsAt: u.AcceptedTermsAt,
		RatingSum:       u.RatingSum,
		RatingCount:     u.RatingCount,
		SuspendedUntil:  u.SuspendedUntil,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
		Version:         u.Version,
		DeactivatedAt:   u.DeactivatedAt,
	}
	if u.ID != "" {
		id, err := bson.ObjectIDFromHex(u.ID)
		if err != nil {
			return d, fmt.Errorf("users: id inválido: %w", err)
		}
		d.ID = id
	}
	return d, nil
}

func (d userDoc) toDomain() domain.User {
	return domain.User{
		ID:              d.ID.Hex(),
		Email:           d.Email,
		PasswordHash:    d.PasswordHash,
		DisplayName:     d.DisplayName,
		Phone:           d.Phone,
		Role:            domain.Role(d.Role),
		Status:          domain.Status(d.Status),
		AcceptedTermsAt: d.AcceptedTermsAt.UTC(),
		RatingSum:       d.RatingSum,
		RatingCount:     d.RatingCount,
		SuspendedUntil:  utcPtr(d.SuspendedUntil),
		CreatedAt:       d.CreatedAt.UTC(),
		UpdatedAt:       d.UpdatedAt.UTC(),
		Version:         d.Version,
		DeactivatedAt:   utcPtr(d.DeactivatedAt),
	}
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// UserRepo guarda las cuentas en la colección users.
type UserRepo struct {
	col *drv.Collection
}

// NewUserRepo crea el repositorio sobre db.
func NewUserRepo(db *drv.Database) *UserRepo {
	return &UserRepo{col: db.Collection(UsersCollection)}
}

// EnsureIndexes crea el índice único de correo (RF-02). Es idempotente.
func (r *UserRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateOne(ctx, drv.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetName("email_unique").SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("users: crear índices: %w", err)
	}
	return nil
}

// Create inserta la cuenta y devuelve la cuenta con su ID. Un correo repetido
// devuelve domain.ErrEmailTaken.
func (r *UserRepo) Create(ctx context.Context, u domain.User) (domain.User, error) {
	u.ID = ""
	d, err := toDoc(u)
	if err != nil {
		return domain.User{}, err
	}
	d.ID = bson.NewObjectID()
	if _, err := r.col.InsertOne(ctx, d); err != nil {
		if drv.IsDuplicateKeyError(err) {
			return domain.User{}, domain.ErrEmailTaken.Wrap(err)
		}
		return domain.User{}, fmt.Errorf("users: insertar: %w", err)
	}
	return d.toDomain(), nil
}

// ByEmail busca por correo normalizado; si no existe devuelve
// domain.ErrUserNotFound.
func (r *UserRepo) ByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.findOne(ctx, bson.D{{Key: "email", Value: email}})
}

// ByID busca por ID; un ID mal formado también es domain.ErrUserNotFound.
func (r *UserRepo) ByID(ctx context.Context, id string) (domain.User, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.User{}, domain.ErrUserNotFound
	}
	return r.findOne(ctx, bson.D{{Key: "_id", Value: oid}})
}

func (r *UserRepo) findOne(ctx context.Context, filter bson.D) (domain.User, error) {
	var d userDoc
	if err := r.col.FindOne(ctx, filter).Decode(&d); err != nil {
		if errors.Is(err, drv.ErrNoDocuments) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("users: buscar: %w", err)
	}
	return d.toDomain(), nil
}
