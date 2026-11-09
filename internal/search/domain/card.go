package domain

import (
	"math"
	"time"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// Zone es la zona aproximada del objeto.
type Zone struct {
	Municipality string
	Neighborhood *string
}

// Owner es el perfil público mínimo de quien ofrece (RF-06, H-04).
type Owner struct {
	ID            string
	DisplayName   string
	RatingAverage *float64
	RatingCount   int64
}

// Card es un resultado de búsqueda (RF-06): solo datos públicos. Nunca lleva
// valor declarado, reglas de uso ni datos de contacto.
type Card struct {
	ID               string
	Title            string
	Category         string
	Modality         string
	PricePerDayCents int64
	Zone             Zone
	// Location ya está redondeada a 2 decimales (spec 005, RF-04).
	Location *Point
	// DistanceMeters es la distancia a quien busca; nil sin lat/lng.
	DistanceMeters *float64
	// FirstPhoto es la primera foto (para la miniatura); nil si no hay.
	FirstPhoto *images.Image
	PhotoCount int
	Owner      Owner
	CreatedAt  time.Time
	// SortKey es el valor de orden del modo (para el cursor).
	SortKey float64
}

// DistanceKm devuelve la distancia en km con 1 decimal (RF-05), o nil.
func (c Card) DistanceKm() *float64 {
	if c.DistanceMeters == nil {
		return nil
	}
	km := math.Round(*c.DistanceMeters/100) / 10
	return &km
}

// RatingAverage calcula el promedio de calificaciones como la spec 003
// (suma / cantidad), o nil sin calificaciones.
func RatingAverage(sum, count int64) *float64 {
	if count <= 0 {
		return nil
	}
	avg := float64(sum) / float64(count)
	return &avg
}

// Page es una página de resultados; Next es nil al final (RF-07).
type Page struct {
	Cards []Card
	Next  *Cursor
}
