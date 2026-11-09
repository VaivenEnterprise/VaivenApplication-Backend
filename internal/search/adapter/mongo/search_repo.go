package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
)

// Repo ejecuta las búsquedas sobre items (solo lectura).
type Repo struct {
	items *drv.Collection
	now   func() time.Time
}

// NewRepo crea el repositorio. now nil = time.Now.
func NewRepo(db *drv.Database, now func() time.Time) *Repo {
	if now == nil {
		now = time.Now
	}
	return &Repo{items: db.Collection(ItemsCollection), now: now}
}

// cardDoc es el resultado de projectStage.
type cardDoc struct {
	ID               bson.ObjectID `bson:"_id"`
	Title            string        `bson:"title"`
	Category         string        `bson:"category"`
	Modality         string        `bson:"modality"`
	PricePerDayCents int64         `bson:"pricePerDayCents"`
	Zone             struct {
		Municipality string  `bson:"municipality"`
		Neighborhood *string `bson:"neighborhood"`
	} `bson:"zone"`
	Location *struct {
		Coordinates []float64 `bson:"coordinates"`
	} `bson:"location"`
	FirstPhoto *struct {
		PublicID string `bson:"publicId"`
		Width    int    `bson:"width"`
		Height   int    `bson:"height"`
		Format   string `bson:"format"`
		Bytes    int64  `bson:"bytes"`
		Delivery string `bson:"delivery"`
	} `bson:"firstPhoto"`
	PhotoCount int `bson:"photoCount"`
	Owner      struct {
		ID          string `bson:"id"`
		DisplayName string `bson:"displayName"`
		RatingSum   int64  `bson:"ratingSum"`
		RatingCount int64  `bson:"ratingCount"`
	} `bson:"owner"`
	CreatedAt time.Time `bson:"createdAt"`
	SortKey   float64   `bson:"sortKey"`
	Dist      *float64  `bson:"dist"`
}

func (d cardDoc) toDomain() domain.Card {
	c := domain.Card{
		ID:               d.ID.Hex(),
		Title:            d.Title,
		Category:         d.Category,
		Modality:         d.Modality,
		PricePerDayCents: d.PricePerDayCents,
		Zone:             domain.Zone{Municipality: d.Zone.Municipality, Neighborhood: d.Zone.Neighborhood},
		PhotoCount:       d.PhotoCount,
		Owner: domain.Owner{
			ID: d.Owner.ID, DisplayName: d.Owner.DisplayName,
			RatingAverage: domain.RatingAverage(d.Owner.RatingSum, d.Owner.RatingCount),
			RatingCount:   d.Owner.RatingCount,
		},
		CreatedAt:      d.CreatedAt.UTC(),
		SortKey:        d.SortKey,
		DistanceMeters: d.Dist,
	}
	if l := d.Location; l != nil && len(l.Coordinates) == 2 {
		c.Location = &domain.Point{Lng: l.Coordinates[0], Lat: l.Coordinates[1]}
	}
	if p := d.FirstPhoto; p != nil {
		c.FirstPhoto = &images.Image{
			PublicID: p.PublicID, Width: p.Width, Height: p.Height,
			Format: p.Format, Bytes: p.Bytes, Delivery: images.Delivery(p.Delivery),
		}
	}
	return c
}

// Search ejecuta la consulta y devuelve una página de tarjetas (RF-01 a RF-07).
func (r *Repo) Search(ctx context.Context, q domain.Query) (domain.Page, error) {
	cur, err := r.items.Aggregate(ctx, BuildPipeline(q, r.now()))
	if err != nil {
		return domain.Page{}, fmt.Errorf("search: consultar: %w", err)
	}
	var docs []cardDoc
	if err := cur.All(ctx, &docs); err != nil {
		return domain.Page{}, fmt.Errorf("search: leer resultados: %w", err)
	}
	page := domain.Page{Cards: make([]domain.Card, 0, min(len(docs), q.Limit))}
	for i, d := range docs {
		if i == q.Limit {
			last := page.Cards[len(page.Cards)-1]
			page.Next = &domain.Cursor{Mode: q.Mode(), Key: last.SortKey, ID: last.ID}
			break
		}
		page.Cards = append(page.Cards, d.toDomain())
	}
	return page, nil
}
