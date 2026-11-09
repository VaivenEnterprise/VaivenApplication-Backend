package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
)

// PushTokensCollection es la colección de tokens de push.
const PushTokensCollection = "push_tokens"

// TokenRepo guarda los tokens de Expo. Cumple usecase.TokenRepository.
type TokenRepo struct{ col *drv.Collection }

// NewTokenRepo crea el repositorio sobre db.
func NewTokenRepo(db *drv.Database) *TokenRepo {
	return &TokenRepo{col: db.Collection(PushTokensCollection)}
}

// EnsureIndexes crea los índices del plan §3: token único (un dispositivo
// pertenece a una sola persona) y {userId} para buscar los de una persona.
func (r *TokenRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, []drv.IndexModel{
		{Keys: bson.D{{Key: "token", Value: 1}}, Options: options.Index().SetName("token_unique").SetUnique(true)},
		{Keys: bson.D{{Key: "userId", Value: 1}}, Options: options.Index().SetName("user")},
	})
	if err != nil {
		return fmt.Errorf("push_tokens: crear índices: %w", err)
	}
	return nil
}

// Upsert registra el token o, si ya existía, lo reasigna a la persona y la
// plataforma de t y renueva lastSeenAt (RF-04). Es atómico: dos registros
// simultáneos del mismo token no chocan con el índice único (se reintenta una
// vez ante la carrera de dos upserts).
func (r *TokenRepo) Upsert(ctx context.Context, t domain.PushToken) error {
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "userId", Value: t.UserID}, {Key: "platform", Value: t.Platform}, {Key: "lastSeenAt", Value: t.LastSeenAt.UTC()},
		}},
		{Key: "$setOnInsert", Value: bson.D{{Key: "_id", Value: bson.NewObjectID()}, {Key: "createdAt", Value: t.CreatedAt.UTC()}}},
	}
	var err error
	for i := 0; i < 2; i++ {
		_, err = r.col.UpdateOne(ctx, bson.D{{Key: "token", Value: t.Token}}, update, options.UpdateOne().SetUpsert(true))
		if err == nil || !drv.IsDuplicateKeyError(err) {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("push_tokens: registrar token: %w", err)
	}
	return nil
}

// Delete quita el token solo si pertenece a userID.
func (r *TokenRepo) Delete(ctx context.Context, userID, token string) error {
	if _, err := r.col.DeleteOne(ctx, bson.D{{Key: "token", Value: token}, {Key: "userId", Value: userID}}); err != nil {
		return fmt.Errorf("push_tokens: eliminar token: %w", err)
	}
	return nil
}

// TokensOf devuelve los tokens de userID (para el worker de push).
func (r *TokenRepo) TokensOf(ctx context.Context, userID string) ([]string, error) {
	cur, err := r.col.Find(ctx, bson.D{{Key: "userId", Value: userID}}, options.Find().SetProjection(bson.D{{Key: "token", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("push_tokens: buscar tokens: %w", err)
	}
	var docs []struct {
		Token string `bson:"token"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("push_tokens: leer tokens: %w", err)
	}
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.Token)
	}
	return out, nil
}

// DeleteToken quita el token sin importar de quién sea: lo usa el worker
// cuando Expo responde DeviceNotRegistered (CA-02).
func (r *TokenRepo) DeleteToken(ctx context.Context, token string) error {
	if _, err := r.col.DeleteOne(ctx, bson.D{{Key: "token", Value: token}}); err != nil {
		return fmt.Errorf("push_tokens: eliminar token inválido: %w", err)
	}
	return nil
}
