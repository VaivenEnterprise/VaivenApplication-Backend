// Package usecase contiene el caso de uso de la búsqueda de objetos (spec
// 006, T006). Depende solo de puertos (Art. III y XII.4).
package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
)

// Repository ejecuta la búsqueda (implementación: adapter/mongo.Repo).
type Repository interface {
	Search(ctx context.Context, q domain.Query) (domain.Page, error)
}

// CardView es una tarjeta lista para responder: la miniatura de la primera
// foto ya calculada con images.Service (plan P-03).
type CardView struct {
	Card     domain.Card
	ThumbURL *string
}

// Result es una página de tarjetas; Next es nil al final.
type Result struct {
	Cards []CardView
	Next  *domain.Cursor
}
