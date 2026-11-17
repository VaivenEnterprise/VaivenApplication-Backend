// Package usecase es el caso de uso del asistente de proyectos (spec 014,
// parte B). Declara sus puertos (Art. III.3); los adaptadores los implementan.
package usecase

import (
	"context"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

// GeoPoint es la ubicación de quien busca. Solo se usa para la búsqueda: nunca
// se envía al proveedor de IA ni se guarda (RF-18).
type GeoPoint struct {
	Lat, Lng float64
}

// FindRequest pide, para cada necesidad, quién la tiene disponible.
type FindRequest struct {
	ViewerID string
	Needs    []aidomain.Need
	// Near es opcional; sin él se usa Municipality o solo el texto (RF-11).
	Near         *GeoPoint
	Municipality string
}

// NeedResult son las coincidencias de una necesidad, en el mismo orden que
// FindRequest.Needs. Las tarjetas son las de la búsqueda de la spec 006 (misma
// visibilidad y ubicación aproximada); no se copian ni se redefinen.
type NeedResult struct {
	Need    aidomain.Need
	Matches []searchusecase.CardView
}

// Finder busca quién tiene cada necesidad (lo implementa
// internal/assistant/adapter/search sobre la búsqueda de la spec 006).
type Finder interface {
	Find(ctx context.Context, req FindRequest) ([]NeedResult, error)
}
