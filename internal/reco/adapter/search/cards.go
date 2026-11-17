// Package search entrega a las recomendaciones (spec 015) las tarjetas
// públicas de la búsqueda (spec 006, también de BE-B) con sus mismas reglas
// de visibilidad (plan P-06; Art. XII.6: no se copia código).
package search

import (
	"context"
	"errors"

	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

// ByIDs es la parte de la búsqueda que se usa (searchusecase.Service la cumple).
type ByIDs interface {
	CardsByIDs(ctx context.Context, viewerID string, ids []string) ([]searchusecase.CardView, error)
}

// Cards obtiene tarjetas de objetos recomendados.
type Cards struct {
	search ByIDs
}

// New crea el adaptador.
func New(s ByIDs) (*Cards, error) {
	if s == nil {
		return nil, errors.New("reco: falta la búsqueda")
	}
	return &Cards{search: s}, nil
}

// Cards devuelve, en el orden pedido, las tarjetas de ids que viewerID puede
// ver; las que no son visibles se omiten. Nunca devuelve nil.
func (c *Cards) Cards(ctx context.Context, viewerID string, ids []string) ([]searchusecase.CardView, error) {
	cards, err := c.search.CardsByIDs(ctx, viewerID, ids)
	if err != nil {
		return nil, err
	}
	if cards == nil {
		return []searchusecase.CardView{}, nil
	}
	return cards, nil
}
