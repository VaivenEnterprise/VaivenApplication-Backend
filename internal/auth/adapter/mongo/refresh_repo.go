package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
)

// RefreshTokensCollection es el nombre de la colección de refresh tokens.
const RefreshTokensCollection = "refresh_tokens"

// refreshDoc es el documento BSON de refresh_tokens. Solo guarda el hash del
// token (plan P-03).
type refreshDoc struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	UserID    string        `bson:"userId"`
	FamilyID  string        `bson:"familyId"`
	TokenHash string        `bson:"tokenHash"`
	ExpiresAt time.Time     `bson:"expiresAt"`
	UsedAt    *time.Time    `bson:"usedAt"`
	RevokedAt *time.Time    `bson:"revokedAt"`
	CreatedAt time.Time     `bson:"createdAt"`
}

func (d refreshDoc) toDomain() domain.RefreshToken {
	return domain.RefreshToken{
		ID:        d.ID.Hex(),
		UserID:    d.UserID,
		FamilyID:  d.FamilyID,
		TokenHash: d.TokenHash,
		ExpiresAt: d.ExpiresAt.UTC(),
		UsedAt:    utcPtr(d.UsedAt),
		RevokedAt: utcPtr(d.RevokedAt),
		CreatedAt: d.CreatedAt.UTC(),
	}
}

// RefreshRepo guarda los refresh tokens. Los documentos vencidos los borra
// MongoDB con el índice TTL de expiresAt.
type RefreshRepo struct {
	col *drv.Collection
}

// NewRefreshRepo crea el repositorio sobre db.
func NewRefreshRepo(db *drv.Database) *RefreshRepo {
	return &RefreshRepo{col: db.Collection(RefreshTokensCollection)}
}

// RefreshTokenIndexes declara los índices de RefreshTokensCollection: es la fuente única
// de verdad que consumen EnsureIndexes y el catálogo de `admin indexes`
// (spec 012, T004). Solo datos: no abre conexiones.
func RefreshTokenIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(RefreshTokensCollection, []drv.IndexModel{
		{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetName("tokenHash_unique").SetUnique(true)},
		{Keys: bson.D{{Key: "familyId", Value: 1}}, Options: options.Index().SetName("familyId")},
		{Keys: bson.D{{Key: "userId", Value: 1}}, Options: options.Index().SetName("userId")},
		{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetName("expiresAt_ttl").SetExpireAfterSeconds(0)},
	}...)
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *RefreshRepo) Indexes() []adaptermongo.IndexSpec { return RefreshTokenIndexes() }

// EnsureIndexes crea los índices del plan §3 (hash único, familia y TTL) y uno
// por usuario para revocar todas sus sesiones en la baja. Es idempotente.
func (r *RefreshRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes()))
	if err != nil {
		return fmt.Errorf("refresh_tokens: crear índices: %w", err)
	}
	return nil
}

// Create guarda un refresh token nuevo.
func (r *RefreshRepo) Create(ctx context.Context, t domain.RefreshToken) error {
	d := refreshDoc{
		ID:        bson.NewObjectID(),
		UserID:    t.UserID,
		FamilyID:  t.FamilyID,
		TokenHash: t.TokenHash,
		ExpiresAt: t.ExpiresAt,
		CreatedAt: t.CreatedAt,
	}
	if _, err := r.col.InsertOne(ctx, d); err != nil {
		return fmt.Errorf("refresh_tokens: insertar: %w", err)
	}
	return nil
}

// ByHash busca por hash; si no existe devuelve domain.ErrInvalidSession.
func (r *RefreshRepo) ByHash(ctx context.Context, hash string) (domain.RefreshToken, error) {
	var d refreshDoc
	err := r.col.FindOne(ctx, bson.D{{Key: "tokenHash", Value: hash}}).Decode(&d)
	if errors.Is(err, drv.ErrNoDocuments) {
		return domain.RefreshToken{}, domain.ErrInvalidSession
	}
	if err != nil {
		return domain.RefreshToken{}, fmt.Errorf("refresh_tokens: buscar: %w", err)
	}
	return d.toDomain(), nil
}

// MarkUsed marca el token como usado solo si aún no se usó ni se revocó, de
// forma atómica: si dos peticiones lo canjean a la vez, solo una obtiene true.
func (r *RefreshRepo) MarkUsed(ctx context.Context, id string, at time.Time) (bool, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return false, fmt.Errorf("refresh_tokens: id inválido: %w", err)
	}
	res, err := r.col.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: oid}, {Key: "usedAt", Value: nil}, {Key: "revokedAt", Value: nil}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "usedAt", Value: at}}}},
	)
	if err != nil {
		return false, fmt.Errorf("refresh_tokens: marcar usado: %w", err)
	}
	return res.ModifiedCount == 1, nil
}

// RevokeFamily revoca todos los tokens aún vigentes de la familia.
func (r *RefreshRepo) RevokeFamily(ctx context.Context, familyID string, at time.Time) error {
	return r.revoke(ctx, bson.D{{Key: "familyId", Value: familyID}}, at)
}

// RevokeAllForUser revoca todas las sesiones de la persona (baja, RF-08).
func (r *RefreshRepo) RevokeAllForUser(ctx context.Context, userID string, at time.Time) error {
	return r.revoke(ctx, bson.D{{Key: "userId", Value: userID}}, at)
}

func (r *RefreshRepo) revoke(ctx context.Context, filter bson.D, at time.Time) error {
	filter = append(filter, bson.E{Key: "revokedAt", Value: nil})
	_, err := r.col.UpdateMany(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "revokedAt", Value: at}}}})
	if err != nil {
		return fmt.Errorf("refresh_tokens: revocar: %w", err)
	}
	return nil
}

// Comprobación en compilación del puerto que implementa RefreshRepo.
var _ usecase.RefreshTokens = (*RefreshRepo)(nil)
