// Package mongo implementa el repositorio de objetos con MongoDB (spec 005,
// T004; plan §3; Art. II). Es el único paquete del módulo items que importa el
// driver.
package mongo

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

// ItemsCollection es el nombre de la colección de objetos.
const ItemsCollection = "items"

// itemDoc es el documento BSON de items; los campos coinciden con el JSON
// (camelCase, Art. 6.1 de la constitución).
type itemDoc struct {
	ID                 bson.ObjectID `bson:"_id,omitempty"`
	OwnerID            string        `bson:"ownerId"`
	Title              string        `bson:"title"`
	Description        string        `bson:"description"`
	Category           string        `bson:"category"`
	Condition          string        `bson:"condition"`
	Modality           string        `bson:"modality"`
	DeclaredValueCents int64         `bson:"declaredValueCents"`
	PricePerDayCents   int64         `bson:"pricePerDayCents"`
	PriceSource        string        `bson:"priceSource"`
	UsageRules         *string       `bson:"usageRules"`
	Zone               zoneDoc       `bson:"zone"`
	// Location es un GeoJSON Point [lng, lat] ya redondeado (RF-04). Se omite
	// si no hay ubicación: el índice 2dsphere ignora documentos sin el campo.
	Location  *pointDoc  `bson:"location,omitempty"`
	Photos    []imageDoc `bson:"photos"`
	Status    string     `bson:"status"`
	CreatedAt time.Time  `bson:"createdAt"`
	UpdatedAt time.Time  `bson:"updatedAt"`
	Version   int64      `bson:"version"`
}

type zoneDoc struct {
	Municipality string  `bson:"municipality"`
	Neighborhood *string `bson:"neighborhood"`
}

type pointDoc struct {
	Type        string    `bson:"type"`
	Coordinates []float64 `bson:"coordinates"`
}

// imageDoc es images.Image guardada (spec 004, RF-06).
type imageDoc struct {
	PublicID string `bson:"publicId"`
	Width    int    `bson:"width"`
	Height   int    `bson:"height"`
	Format   string `bson:"format"`
	Bytes    int64  `bson:"bytes"`
	Delivery string `bson:"delivery"`
}

func toDoc(it domain.Item) (itemDoc, error) {
	d := itemDoc{
		OwnerID:            it.OwnerID,
		Title:              it.Title,
		Description:        it.Description,
		Category:           it.Category,
		Condition:          string(it.Condition),
		Modality:           string(it.Modality),
		DeclaredValueCents: it.DeclaredValueCents,
		PricePerDayCents:   it.PricePerDayCents,
		PriceSource:        string(it.PriceSource),
		UsageRules:         it.UsageRules,
		Zone:               zoneDoc{Municipality: it.Zone.Municipality, Neighborhood: it.Zone.Neighborhood},
		Photos:             make([]imageDoc, 0, len(it.Photos)),
		Status:             string(it.Status),
		CreatedAt:          it.CreatedAt.UTC(),
		UpdatedAt:          it.UpdatedAt.UTC(),
		Version:            it.Version,
	}
	if it.ID != "" {
		oid, err := bson.ObjectIDFromHex(it.ID)
		if err != nil {
			return itemDoc{}, fmt.Errorf("items: id inválido: %w", err)
		}
		d.ID = oid
	}
	if l := it.Location; l != nil {
		// Se redondea otra vez aquí: ningún camino guarda la ubicación exacta.
		d.Location = &pointDoc{Type: "Point", Coordinates: []float64{domain.RoundCoord(l.Lng), domain.RoundCoord(l.Lat)}}
	}
	for _, p := range it.Photos {
		d.Photos = append(d.Photos, imageDoc{
			PublicID: p.PublicID, Width: p.Width, Height: p.Height,
			Format: p.Format, Bytes: p.Bytes, Delivery: string(p.Delivery),
		})
	}
	return d, nil
}

func (d itemDoc) toDomain() domain.Item {
	it := domain.Item{
		ID:                 d.ID.Hex(),
		OwnerID:            d.OwnerID,
		Title:              d.Title,
		Description:        d.Description,
		Category:           d.Category,
		Condition:          domain.Condition(d.Condition),
		Modality:           domain.Modality(d.Modality),
		DeclaredValueCents: d.DeclaredValueCents,
		PricePerDayCents:   d.PricePerDayCents,
		PriceSource:        domain.PriceSource(d.PriceSource),
		UsageRules:         d.UsageRules,
		Zone:               domain.Zone{Municipality: d.Zone.Municipality, Neighborhood: d.Zone.Neighborhood},
		Photos:             make([]images.Image, 0, len(d.Photos)),
		Status:             domain.Status(d.Status),
		CreatedAt:          d.CreatedAt.UTC(),
		UpdatedAt:          d.UpdatedAt.UTC(),
		Version:            d.Version,
	}
	if d.Location != nil && len(d.Location.Coordinates) == 2 {
		it.Location = &domain.Location{Lng: d.Location.Coordinates[0], Lat: d.Location.Coordinates[1]}
	}
	for _, p := range d.Photos {
		it.Photos = append(it.Photos, images.Image{
			PublicID: p.PublicID, Width: p.Width, Height: p.Height,
			Format: p.Format, Bytes: p.Bytes, Delivery: images.Delivery(p.Delivery),
		})
	}
	return it
}
