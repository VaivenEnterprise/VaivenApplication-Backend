package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Nombres de los índices de búsqueda sobre items (plan §4). El 2dsphere de
// location lo crea la spec 005.
const (
	TextIndexName    = "search_text"
	FiltersIndexName = "search_filters"
	RecentIndexName  = "search_recent"
)

// Indexes crea los índices de búsqueda en items. Es propio de esta spec: no
// toca el repositorio de items (Art. XII).
type Indexes struct {
	col *drv.Collection
}

// NewIndexes prepara los índices sobre db.
func NewIndexes(db *drv.Database) *Indexes {
	return &Indexes{col: db.Collection(ItemsCollection)}
}

// EnsureIndexes crea, de forma idempotente, el índice de texto en español
// (título peso 5, descripción peso 1; sin acentos ni mayúsculas, RF-04) y el
// compuesto de filtros. Cumple adapter/mongo.IndexEnsurer.
func (ix *Indexes) EnsureIndexes(ctx context.Context) error {
	_, err := ix.col.Indexes().CreateMany(ctx, []drv.IndexModel{
		{
			Keys: bson.D{{Key: "title", Value: "text"}, {Key: "description", Value: "text"}},
			Options: options.Index().SetName(TextIndexName).
				SetDefaultLanguage("spanish").
				SetWeights(bson.D{{Key: "title", Value: 5}, {Key: "description", Value: 1}}),
		},
		{
			Keys: bson.D{
				{Key: "status", Value: 1}, {Key: "category", Value: 1}, {Key: "modality", Value: 1},
				{Key: "zone.municipality", Value: 1}, {Key: "createdAt", Value: -1},
			},
			Options: options.Index().SetName(FiltersIndexName),
		},
		{
			// Orden del modo reciente (createdAt desc, _id desc) sin ordenar
			// en memoria cuando solo se filtra por status.
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}},
			Options: options.Index().SetName(RecentIndexName),
		},
	})
	if err != nil {
		return fmt.Errorf("search: crear índices: %w", err)
	}
	return nil
}
