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

// Colecciones del modelo calculado (plan §2 y P-03).
const (
	ModelCollection   = "reco_model"
	PopularCollection = "reco_popular"
)

// saveBatch es el tamaño de cada InsertMany al guardar un modelo.
const saveBatch = 500

// Cada documento del modelo lleva el buildId en su _id ("<buildId>:<clave>"):
// un modelo nuevo se escribe completo junto al anterior, MetaRepo.Publish lo
// vuelve el vigente en una sola escritura y después se borra el anterior. Así
// nunca se lee un modelo a medias (plan P-03). Toda lectura es por _id, por lo
// que estas colecciones no necesitan más índices.

type neighborDoc struct {
	ItemID  string  `bson:"itemId"`
	CF      float64 `bson:"cf"`
	Content float64 `bson:"content"`
}

type modelDoc struct {
	ID        string        `bson:"_id"`
	BuildID   string        `bson:"buildId"`
	ItemID    string        `bson:"itemId"`
	Neighbors []neighborDoc `bson:"neighbors"`
	CreatedAt time.Time     `bson:"createdAt"`
	UpdatedAt time.Time     `bson:"updatedAt"`
}

type popularDoc struct {
	ID        string    `bson:"_id"`
	BuildID   string    `bson:"buildId"`
	Key       string    `bson:"key"`
	Items     []string  `bson:"items"`
	CreatedAt time.Time `bson:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"`
}

func docID(buildID, key string) string { return buildID + ":" + key }

// ModelRepo guarda y lee el modelo calculado (vecinos y populares).
type ModelRepo struct {
	model   *drv.Collection
	popular *drv.Collection
}

// NewModelRepo crea el repositorio sobre db.
func NewModelRepo(db *drv.Database) *ModelRepo {
	return &ModelRepo{model: db.Collection(ModelCollection), popular: db.Collection(PopularCollection)}
}

// Save escribe el modelo buildID completo. Los arreglos se recortan a
// domain.Neighbors y domain.PopularSize (constitución §6.2). No lo vuelve
// vigente: eso lo hace MetaRepo.Publish.
func (r *ModelRepo) Save(ctx context.Context, buildID string, neighbors map[string][]domain.Neighbor, popular map[string][]string, now time.Time) error {
	now = now.UTC()
	models := make([]any, 0, len(neighbors))
	for itemID, ns := range neighbors {
		if len(ns) > domain.Neighbors {
			ns = ns[:domain.Neighbors]
		}
		d := modelDoc{ID: docID(buildID, itemID), BuildID: buildID, ItemID: itemID, Neighbors: make([]neighborDoc, len(ns)), CreatedAt: now, UpdatedAt: now}
		for i, n := range ns {
			d.Neighbors[i] = neighborDoc{ItemID: n.ItemID, CF: n.CF, Content: n.Content}
		}
		models = append(models, d)
	}
	if err := insertBatches(ctx, r.model, models); err != nil {
		return fmt.Errorf("reco_model: guardar vecinos: %w", err)
	}
	pops := make([]any, 0, len(popular))
	for key, ids := range popular {
		if len(ids) > domain.PopularSize {
			ids = ids[:domain.PopularSize]
		}
		pops = append(pops, popularDoc{ID: docID(buildID, key), BuildID: buildID, Key: key, Items: ids, CreatedAt: now, UpdatedAt: now})
	}
	if err := insertBatches(ctx, r.popular, pops); err != nil {
		return fmt.Errorf("reco_popular: guardar populares: %w", err)
	}
	return nil
}

func insertBatches(ctx context.Context, col *drv.Collection, docs []any) error {
	for start := 0; start < len(docs); start += saveBatch {
		end := min(start+saveBatch, len(docs))
		if _, err := col.InsertMany(ctx, docs[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// DeleteOtherBuilds borra los documentos de cualquier modelo distinto de
// keepBuildID (el anterior al vigente o uno que quedó a medias).
func (r *ModelRepo) DeleteOtherBuilds(ctx context.Context, keepBuildID string) error {
	filter := bson.D{{Key: "buildId", Value: bson.D{{Key: "$ne", Value: keepBuildID}}}}
	if _, err := r.model.DeleteMany(ctx, filter); err != nil {
		return fmt.Errorf("reco_model: borrar modelos anteriores: %w", err)
	}
	if _, err := r.popular.DeleteMany(ctx, filter); err != nil {
		return fmt.Errorf("reco_popular: borrar modelos anteriores: %w", err)
	}
	return nil
}

// Neighbors devuelve los vecinos de cada objeto del modelo buildID. Los
// objetos sin vecinos no aparecen en el mapa.
func (r *ModelRepo) Neighbors(ctx context.Context, buildID string, itemIDs []string) (map[string][]domain.Neighbor, error) {
	out := map[string][]domain.Neighbor{}
	if len(itemIDs) == 0 {
		return out, nil
	}
	ids := make([]string, len(itemIDs))
	for i, id := range itemIDs {
		ids[i] = docID(buildID, id)
	}
	cur, err := r.model.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}})
	if err != nil {
		return nil, fmt.Errorf("reco_model: leer vecinos: %w", err)
	}
	var docs []modelDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("reco_model: leer vecinos: %w", err)
	}
	for _, d := range docs {
		ns := make([]domain.Neighbor, len(d.Neighbors))
		for i, n := range d.Neighbors {
			ns[i] = domain.Neighbor{ItemID: n.ItemID, CF: n.CF, Content: n.Content}
		}
		out[d.ItemID] = ns
	}
	return out, nil
}

// Popular devuelve las listas de populares de las claves pedidas en el modelo
// buildID. Las claves sin lista no aparecen en el mapa.
func (r *ModelRepo) Popular(ctx context.Context, buildID string, keys []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(keys) == 0 {
		return out, nil
	}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = docID(buildID, k)
	}
	cur, err := r.popular.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}},
		options.Find().SetProjection(bson.D{{Key: "key", Value: 1}, {Key: "items", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("reco_popular: leer populares: %w", err)
	}
	var docs []popularDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("reco_popular: leer populares: %w", err)
	}
	for _, d := range docs {
		out[d.Key] = d.Items
	}
	return out, nil
}
