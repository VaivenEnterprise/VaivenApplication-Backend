// Package mongo implementa el repositorio de sanciones con MongoDB (spec 010,
// T006; plan §3). Es el único paquete del módulo que importa el driver. Las
// sanciones son un registro: solo se insertan.
package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/domain"
)

// SanctionsCollection es la colección de sanciones.
const SanctionsCollection = "sanctions"

// Repo guarda las sanciones. Cumple usecase.Repository.
type Repo struct{ col *drv.Collection }

// NewRepo crea el repositorio sobre db.
func NewRepo(db *drv.Database) *Repo { return &Repo{col: db.Collection(SanctionsCollection)} }

// SanctionIndexes declara los índices de SanctionsCollection: es la fuente única
// de verdad que consumen EnsureIndexes y el catálogo de `admin indexes`
// (spec 012, T004). Solo datos: no abre conexiones.
func SanctionIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(SanctionsCollection,
		drv.IndexModel{
			Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("user_createdAt"),
		},
	)
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *Repo) Indexes() []adaptermongo.IndexSpec { return SanctionIndexes() }

// EnsureIndexes crea el índice de sanctions (plan §3): por persona, la más
// reciente primero.
func (r *Repo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes()))
	if err != nil {
		return fmt.Errorf("sanctions: crear índices: %w", err)
	}
	return nil
}

// sanctionDoc es el documento de sanctions (plan §3).
type sanctionDoc struct {
	ID               bson.ObjectID `bson:"_id"`
	UserID           string        `bson:"userId"`
	Type             string        `bson:"type"`
	Reason           string        `bson:"reason"`
	EndsAt           *time.Time    `bson:"endsAt"`
	IncidentID       *string       `bson:"incidentId"`
	CreatedByAdminID string        `bson:"createdByAdminId"`
	CreatedAt        time.Time     `bson:"createdAt"`
	UpdatedAt        time.Time     `bson:"updatedAt"`
}

func (d sanctionDoc) toDomain() domain.Sanction {
	s := domain.Sanction{
		ID: d.ID.Hex(), UserID: d.UserID, Type: domain.Type(d.Type), Reason: d.Reason,
		IncidentID: d.IncidentID, CreatedByAdminID: d.CreatedByAdminID, CreatedAt: d.CreatedAt.UTC(),
	}
	if d.EndsAt != nil {
		e := d.EndsAt.UTC()
		s.EndsAt = &e
	}
	return s
}

// NewID genera el identificador de una sanción.
func (r *Repo) NewID() string { return bson.NewObjectID().Hex() }

// Create inserta la sanción.
func (r *Repo) Create(ctx context.Context, s domain.Sanction) (domain.Sanction, error) {
	oid, err := bson.ObjectIDFromHex(s.ID)
	if err != nil {
		return domain.Sanction{}, fmt.Errorf("sanctions: id inválido: %w", err)
	}
	d := sanctionDoc{
		ID: oid, UserID: s.UserID, Type: string(s.Type), Reason: s.Reason, IncidentID: s.IncidentID,
		CreatedByAdminID: s.CreatedByAdminID, CreatedAt: s.CreatedAt.UTC(), UpdatedAt: s.CreatedAt.UTC(),
	}
	if s.EndsAt != nil {
		e := s.EndsAt.UTC()
		d.EndsAt = &e
	}
	if _, err := r.col.InsertOne(ctx, d); err != nil {
		return domain.Sanction{}, fmt.Errorf("sanctions: guardar sanción: %w", err)
	}
	return d.toDomain(), nil
}
