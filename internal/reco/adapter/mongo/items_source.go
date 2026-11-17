package mongo

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
)

// Colecciones de otras specs que aquí solo se leen (plan P-05): items (005) y
// users (003).
const (
	itemsCollection = "items"
	usersCollection = "users"
)

type itemFeaturesDoc struct {
	ID               bson.ObjectID `bson:"_id"`
	OwnerID          string        `bson:"ownerId"`
	Title            string        `bson:"title"`
	Category         string        `bson:"category"`
	Modality         string        `bson:"modality"`
	PricePerDayCents int64         `bson:"pricePerDayCents"`
	Status           string        `bson:"status"`
	Zone             struct {
		Municipality string `bson:"municipality"`
	} `bson:"zone"`
}

var featuresProjection = bson.D{
	{Key: "ownerId", Value: 1}, {Key: "title", Value: 1}, {Key: "category", Value: 1}, {Key: "modality", Value: 1},
	{Key: "pricePerDayCents", Value: 1}, {Key: "status", Value: 1}, {Key: "zone.municipality", Value: 1},
}

func (d itemFeaturesDoc) toDomain() domain.ItemFeatures {
	return domain.ItemFeatures{
		ID: d.ID.Hex(), OwnerID: d.OwnerID, Title: d.Title, Category: d.Category, Modality: d.Modality,
		PricePerDayCents: d.PricePerDayCents, Municipality: d.Zone.Municipality,
	}
}

// ItemSource lee objetos y cuentas. Nunca escribe en items ni en users.
type ItemSource struct {
	items *drv.Collection
	users *drv.Collection
}

// NewItemSource crea la lectura sobre db.
func NewItemSource(db *drv.Database) *ItemSource {
	return &ItemSource{items: db.Collection(itemsCollection), users: db.Collection(usersCollection)}
}

// ItemState devuelve el dueño y el estado de un objeto (para validar un
// evento). found es false si no existe o el id no es válido.
func (s *ItemSource) ItemState(ctx context.Context, itemID string) (ownerID, status string, found bool, err error) {
	oid, err := bson.ObjectIDFromHex(itemID)
	if err != nil {
		return "", "", false, nil
	}
	var d itemFeaturesDoc
	err = s.items.FindOne(ctx, bson.D{{Key: "_id", Value: oid}},
		options.FindOne().SetProjection(bson.D{{Key: "ownerId", Value: 1}, {Key: "status", Value: 1}})).Decode(&d)
	switch {
	case errors.Is(err, drv.ErrNoDocuments):
		return "", "", false, nil
	case err != nil:
		return "", "", false, fmt.Errorf("items: leer estado: %w", err)
	}
	return d.OwnerID, d.Status, true, nil
}

// ActiveFeatures devuelve los datos de todos los objetos activos (modelo de
// contenido y populares). Usa el prefijo status de los índices de la 005.
func (s *ItemSource) ActiveFeatures(ctx context.Context) ([]domain.ItemFeatures, error) {
	cur, err := s.items.Find(ctx, bson.D{{Key: "status", Value: "active"}}, options.Find().SetProjection(featuresProjection))
	if err != nil {
		return nil, fmt.Errorf("items: leer objetos activos: %w", err)
	}
	var docs []itemFeaturesDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("items: leer objetos activos: %w", err)
	}
	out := make([]domain.ItemFeatures, len(docs))
	for i, d := range docs {
		out[i] = d.toDomain()
	}
	return out, nil
}

// Titles devuelve el título de cada objeto pedido (motivo similar_to). Los
// ids inválidos o inexistentes no aparecen.
func (s *ItemSource) Titles(ctx context.Context, itemIDs []string) (map[string]string, error) {
	out := map[string]string{}
	oids := toObjectIDs(itemIDs)
	if len(oids) == 0 {
		return out, nil
	}
	cur, err := s.items.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: oids}}}},
		options.Find().SetProjection(bson.D{{Key: "title", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("items: leer títulos: %w", err)
	}
	var docs []itemFeaturesDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("items: leer títulos: %w", err)
	}
	for _, d := range docs {
		out[d.ID.Hex()] = d.Title
	}
	return out, nil
}

// Deactivated devuelve, de userIDs, las cuentas dadas de baja (purga de
// eventos, RF-06). Los ids que no son ObjectID (personas sintéticas demo-NNN)
// se ignoran.
func (s *ItemSource) Deactivated(ctx context.Context, userIDs []string) ([]string, error) {
	oids := toObjectIDs(userIDs)
	if len(oids) == 0 {
		return nil, nil
	}
	cur, err := s.users.Find(ctx,
		bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: oids}}}, {Key: "status", Value: "deactivated"}},
		options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("users: leer cuentas dadas de baja: %w", err)
	}
	var docs []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("users: leer cuentas dadas de baja: %w", err)
	}
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.ID.Hex()
	}
	return out, nil
}

func toObjectIDs(ids []string) []bson.ObjectID {
	out := make([]bson.ObjectID, 0, len(ids))
	for _, id := range ids {
		if oid, err := bson.ObjectIDFromHex(id); err == nil {
			out = append(out, oid)
		}
	}
	return out
}
