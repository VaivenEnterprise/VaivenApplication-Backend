package mongo

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

// ItemRepo guarda los objetos en la colección items.
type ItemRepo struct {
	col *drv.Collection
}

// NewItemRepo crea el repositorio sobre db.
func NewItemRepo(db *drv.Database) *ItemRepo {
	return &ItemRepo{col: db.Collection(ItemsCollection)}
}

// ItemIndexes declara los índices de ItemsCollection: es la fuente única
// de verdad que consumen EnsureIndexes y el catálogo de `admin indexes`
// (spec 012, T004). Solo datos: no abre conexiones.
func ItemIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(ItemsCollection, []drv.IndexModel{
		{
			// GET /me/items: por dueño y estado, más recientes primero.
			Keys:    bson.D{{Key: "ownerId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("owner_status_createdAt"),
		},
		{
			Keys:    bson.D{{Key: "location", Value: "2dsphere"}},
			Options: options.Index().SetName("location_2dsphere"),
		},
		{
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "category", Value: 1}, {Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("status_category_createdAt"),
		},
	}...)
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *ItemRepo) Indexes() []adaptermongo.IndexSpec { return ItemIndexes() }

// EnsureIndexes crea los índices de este módulo (plan §3). Es idempotente. El
// índice de texto y los de búsqueda los agrega la spec 006 en su propio
// archivo.
func (r *ItemRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes()))
	if err != nil {
		return fmt.Errorf("items: crear índices: %w", err)
	}
	return nil
}

// Create inserta el objeto y lo devuelve con su ID.
func (r *ItemRepo) Create(ctx context.Context, it domain.Item) (domain.Item, error) {
	it.ID = ""
	d, err := toDoc(it)
	if err != nil {
		return domain.Item{}, err
	}
	res, err := r.col.InsertOne(ctx, d)
	if err != nil {
		return domain.Item{}, fmt.Errorf("items: insertar: %w", err)
	}
	oid, ok := res.InsertedID.(bson.ObjectID)
	if !ok {
		return domain.Item{}, errors.New("items: el id insertado no es un ObjectID")
	}
	d.ID = oid
	return d.toDomain(), nil
}

// ByID busca por ID; si no existe o el ID está mal formado devuelve
// domain.ErrNotFound. Devuelve también los removed: la visibilidad la decide
// el caso de uso.
func (r *ItemRepo) ByID(ctx context.Context, id string) (domain.Item, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Item{}, domain.ErrNotFound
	}
	var d itemDoc
	if err := r.col.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&d); err != nil {
		if errors.Is(err, drv.ErrNoDocuments) {
			return domain.Item{}, domain.ErrNotFound
		}
		return domain.Item{}, fmt.Errorf("items: buscar: %w", err)
	}
	return d.toDomain(), nil
}

// Update reemplaza el objeto si su versión guardada sigue siendo it.Version
// (control optimista, plan P-05) y devuelve el objeto con la versión
// incrementada. Si otra petición lo cambió antes devuelve
// domain.ErrVersionConflict. OwnerID y CreatedAt no cambian.
func (r *ItemRepo) Update(ctx context.Context, it domain.Item) (domain.Item, error) {
	d, err := toDoc(it)
	if err != nil {
		return domain.Item{}, domain.ErrNotFound
	}
	if d.ID.IsZero() {
		return domain.Item{}, domain.ErrNotFound
	}
	expected := d.Version
	d.Version = expected + 1
	filter := bson.D{{Key: "_id", Value: d.ID}, {Key: "version", Value: expected}, {Key: "ownerId", Value: d.OwnerID}}
	res, err := r.col.ReplaceOne(ctx, filter, d)
	if err != nil {
		return domain.Item{}, fmt.Errorf("items: actualizar: %w", err)
	}
	if res.MatchedCount == 0 {
		return domain.Item{}, domain.ErrVersionConflict
	}
	return d.toDomain(), nil
}

// ListByOwner devuelve hasta limit objetos de ownerID con alguno de los
// estados indicados, del más reciente al más antiguo, a partir de after
// (exclusivo; nil = desde el principio).
func (r *ItemRepo) ListByOwner(ctx context.Context, ownerID string, statuses []domain.Status, after *domain.Cursor, limit int) ([]domain.Item, error) {
	sts := make(bson.A, 0, len(statuses))
	for _, s := range statuses {
		sts = append(sts, string(s))
	}
	filter := bson.D{{Key: "ownerId", Value: ownerID}, {Key: "status", Value: bson.D{{Key: "$in", Value: sts}}}}
	if after != nil {
		oid, err := bson.ObjectIDFromHex(after.ID)
		if err != nil {
			return nil, fmt.Errorf("items: cursor inválido: %w", err)
		}
		at := after.CreatedAt.UTC()
		filter = append(filter, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "createdAt", Value: bson.D{{Key: "$lt", Value: at}}}},
			bson.D{{Key: "createdAt", Value: at}, {Key: "_id", Value: bson.D{{Key: "$lt", Value: oid}}}},
		}})
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(limit))
	cur, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("items: listar: %w", err)
	}
	var docs []itemDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("items: leer listado: %w", err)
	}
	out := make([]domain.Item, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}
