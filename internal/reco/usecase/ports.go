// Package usecase contiene los casos de uso de las recomendaciones (spec 015,
// T006): registrar vistas, recomendar, borrar el historial, recalcular el
// modelo y evaluarlo. Depende solo de puertos que declara aquí (Art. III.3).
package usecase

import (
	"context"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

// Interactions guarda las vistas (adapter/mongo.InteractionRepo).
type Interactions interface {
	Record(ctx context.Context, e domain.Event) (bool, error)
	Recent(ctx context.Context, userID string, limit int) ([]domain.Event, error)
	Since(ctx context.Context, since time.Time, limit int) ([]domain.Event, error)
	DeleteUser(ctx context.Context, userID string) error
	DeleteUsers(ctx context.Context, userIDs []string) error
}

// Requests lee las solicitudes de reserva (adapter/mongo.RequestSource; spec
// 007, solo lectura).
type Requests interface {
	Since(ctx context.Context, since time.Time, limit int) ([]domain.Event, error)
	Recent(ctx context.Context, userID string, since time.Time, limit int) ([]domain.Event, error)
	ActiveItemIDs(ctx context.Context, userID string) ([]string, error)
}

// Items lee objetos y cuentas (adapter/mongo.ItemSource; solo lectura).
type Items interface {
	ItemState(ctx context.Context, itemID string) (ownerID, status string, found bool, err error)
	ActiveFeatures(ctx context.Context) ([]domain.ItemFeatures, error)
	Briefs(ctx context.Context, itemIDs []string) (map[string]domain.ItemBrief, error)
	Deactivated(ctx context.Context, userIDs []string) ([]string, error)
}

// Model guarda y lee el modelo calculado (adapter/mongo.ModelRepo).
type Model interface {
	Save(ctx context.Context, buildID string, neighbors map[string][]domain.Neighbor, popular map[string][]string, now time.Time) error
	DeleteOtherBuilds(ctx context.Context, keepBuildID string) error
	Neighbors(ctx context.Context, buildID string, itemIDs []string) (map[string][]domain.Neighbor, error)
	Popular(ctx context.Context, buildID string, keys []string) (map[string][]string, error)
}

// Meta es el modelo vigente, el candado y los borrados de historial
// (adapter/mongo.MetaRepo).
type Meta interface {
	TryLock(ctx context.Context, owner string, now time.Time) (bool, error)
	Publish(ctx context.Context, owner, buildID string, builtAt time.Time) (bool, error)
	Unlock(ctx context.Context, owner string) error
	Current(ctx context.Context) (buildID string, builtAt time.Time, ok bool, err error)
	SetReset(ctx context.Context, userID string, at time.Time) error
	Resets(ctx context.Context) (map[string]time.Time, error)
	ResetAt(ctx context.Context, userID string) (time.Time, bool, error)
}

// Cards da las tarjetas públicas visibles para viewerID, en el orden pedido
// (adapter/search.Cards; spec 006).
type Cards interface {
	Cards(ctx context.Context, viewerID string, ids []string) ([]searchusecase.CardView, error)
}

// Recommendation es un objeto recomendado: su tarjeta y el motivo.
type Recommendation struct {
	Card   searchusecase.CardView
	Reason domain.Reason
}
