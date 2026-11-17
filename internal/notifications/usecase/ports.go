// Package usecase contiene los casos de uso de los avisos (spec 011, T004):
// listar, contar no leídos, marcar leídos y registrar o quitar tokens de push.
// Depende solo de puertos (Art. III y XII.4).
package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
)

// Límites de la página de avisos (plan §2).
const (
	DefaultPage = 20
	MaxPage     = 50
)

// Repository guarda y lee los avisos (adapter/mongo.NotificationRepo). Todas
// las operaciones se acotan a userID: nadie ve ni toca avisos ajenos.
type Repository interface {
	// List devuelve los avisos de userID, los más recientes primero, con id
	// menor que after ("" = desde el inicio).
	List(ctx context.Context, userID, after string, limit int) ([]domain.Notification, error)
	// CountUnread cuenta los avisos sin leer de userID.
	CountUnread(ctx context.Context, userID string) (int64, error)
	// MarkRead marca como leídos los avisos ids de userID que sigan sin leer.
	MarkRead(ctx context.Context, userID string, ids []string, at time.Time) error
	// MarkAllRead marca como leídos todos los avisos sin leer de userID.
	MarkAllRead(ctx context.Context, userID string, at time.Time) error
}

// TokenRepository guarda los tokens de push (adapter/mongo.TokenRepo).
type TokenRepository interface {
	// Upsert registra el token o lo reasigna a la persona de t (RF-04).
	Upsert(ctx context.Context, t domain.PushToken) error
	// Delete quita el token si pertenece a userID; no es un error que no exista.
	Delete(ctx context.Context, userID, token string) error
}

// Deps son las dependencias del Service.
type Deps struct {
	Repo   Repository
	Tokens TokenRepository
	Now    func() time.Time // nil = time.Now
}

// Page es una página de avisos; Next es el id para seguir (nil al final).
type Page struct {
	Items []domain.Notification
	Next  *string
}

// Service implementa los casos de uso de los avisos.
type Service struct{ d Deps }

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Repo == nil || d.Tokens == nil {
		return nil, errors.New("notifications: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}, nil
}

// now devuelve la hora en UTC truncada a milisegundos (precisión de MongoDB).
func (s *Service) now() time.Time { return s.d.Now().UTC().Truncate(time.Millisecond) }
