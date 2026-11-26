package domain

import "strings"

// Neighbor es un objeto parecido a otro, con la similitud de cada modelo
// (RF-02 y RF-03). Se guardan como máximo Neighbors por objeto.
type Neighbor struct {
	ItemID  string
	CF      float64 // coseno colaborativo objeto-objeto
	Content float64 // coseno de contenido
}

// ItemFeatures son los datos de un objeto activo que usa el modelo de
// contenido y el arranque en frío (RF-03 y RF-04).
type ItemFeatures struct {
	ID               string
	OwnerID          string
	Title            string
	Category         string
	Modality         string
	PricePerDayCents int64
	Municipality     string
}

// ItemBrief es el título y la categoría de un objeto, para el motivo
// similar_to y para el arranque en frío (RF-04 y RF-05).
type ItemBrief struct {
	Title    string
	Category string
}

// PopularAll es la clave de la lista de populares de todos los objetos (RF-04).
const PopularAll = "all"

// PopularMunicipality es la clave de populares de un municipio; no distingue
// mayúsculas ni espacios repetidos.
func PopularMunicipality(m string) string {
	return "m:" + strings.ToLower(strings.Join(strings.Fields(m), " "))
}

// PopularCategory es la clave de populares de una categoría.
func PopularCategory(c string) string { return "c:" + c }
