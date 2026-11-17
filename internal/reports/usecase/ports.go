// Package usecase contiene los casos de uso de los reportes (spec 011, T011):
// reportar, listar la cola de revisión y resolver. Depende solo de puertos
// (Art. III y XII.4).
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"time"

	chat "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/port"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Límites de la página de reportes.
const (
	DefaultPage = 20
	MaxPage     = 50
	// UnavailableSummary es el resumen de lo reportado que ya no existe.
	UnavailableSummary = "Contenido no disponible"
	// summaryRunes acota la vista previa de un mensaje.
	summaryRunes = 80
)

// Repository guarda los reportes (adapter/mongo.Repo).
type Repository interface {
	NewID() string
	// Create guarda el reporte o devuelve el existente (created=false) si la
	// misma persona ya reportó lo mismo.
	Create(ctx context.Context, r domain.Report) (domain.Report, bool, error)
	ByID(ctx context.Context, id string) (domain.Report, error)
	// List devuelve los reportes con estado status, los más antiguos primero,
	// con id mayor que after.
	List(ctx context.Context, status domain.Status, after string, limit int) ([]domain.Report, error)
	// Resolve cierra el reporte si sigue open; si no, ErrAlreadyResolved.
	Resolve(ctx context.Context, id string, status domain.Status, res domain.Resolution) (domain.Report, error)
}

// Messages lee un mensaje y los participantes de su conversación
// (chat/adapter/mongo.MessageReader).
type Messages interface {
	Reference(ctx context.Context, id string) (chat.MessageRef, bool, error)
}

// Deps son las dependencias del Service.
type Deps struct {
	Repo     Repository
	Items    port.Moderation // items.Moderation (spec 005)
	Messages Messages
	Users    users.Directory
	Admin    users.Admin
	Notifier notify.Notifier
	Log      *slog.Logger
	Now      func() time.Time // nil = time.Now
}

// View es un reporte listo para responder: quien reportó y un resumen de lo
// reportado.
type View struct {
	Report        domain.Report
	Reporter      users.PublicUser
	TargetSummary string
}

// Page es una página de reportes; Next es el id para seguir (nil al final).
type Page struct {
	Items []View
	Next  *string
}

// Service implementa los casos de uso de los reportes.
type Service struct{ d Deps }

// NewService crea el servicio. Falla si falta una dependencia obligatoria.
func NewService(d Deps) (*Service, error) {
	if d.Repo == nil || d.Items == nil || d.Messages == nil || d.Users == nil || d.Admin == nil || d.Notifier == nil || d.Log == nil {
		return nil, errors.New("reports: faltan dependencias del servicio")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}, nil
}

// now devuelve la hora en UTC truncada a milisegundos (precisión de MongoDB).
func (s *Service) now() time.Time { return s.d.Now().UTC().Truncate(time.Millisecond) }
