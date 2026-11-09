// Package usecase contiene los casos de uso de las incidencias (spec 010, T007
// y T008): abrir, consultar y listar las propias (partes) y revisar, anotar y
// cerrar (administración). Depende solo de puertos (Art. III y XII.4).
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// ErrNoMatch lo devuelve el repositorio cuando la operación atómica no aplicó
// porque la incidencia ya no cumple el filtro (se cerró, llegó al máximo de
// notas o ya no existe). El caso de uso la vuelve a leer para explicar por qué.
var ErrNoMatch = errors.New("incidents: la operación atómica no aplicó")

// Bookings lee reservas (solo lectura, plan P-05); lo implementa
// bookings/adapter/mongo.Repo.
type Bookings interface {
	ByID(ctx context.Context, id string) (bookings.Booking, error)
}

// Repository guarda las incidencias (adapter/mongo.Repo).
type Repository interface {
	// NewID genera el identificador de una incidencia.
	NewID() string
	Create(ctx context.Context, i domain.Incident) (domain.Incident, error)
	// ByID busca una incidencia; si no existe, domain.ErrNotFound.
	ByID(ctx context.Context, id string) (domain.Incident, error)
	// Exists indica si existe la incidencia id (la usan las sanciones).
	Exists(ctx context.Context, id string) (bool, error)
	// CountOpen cuenta las incidencias abiertas de la reserva (RF-05).
	CountOpen(ctx context.Context, bookingID string) (int, error)
	// ListByParty devuelve las incidencias en que userID es quien reporta o la
	// parte reportada, las más recientes primero. status "" = todas.
	ListByParty(ctx context.Context, userID string, status domain.Status, after *domain.Cursor, limit int) ([]domain.Incident, error)
	// ListAll devuelve las incidencias para administración, las más antiguas
	// primero. status "" = todas.
	ListAll(ctx context.Context, status domain.Status, after *domain.Cursor, limit int) ([]domain.Incident, error)
	// AddNote agrega la nota solo si la incidencia está abierta y tiene menos
	// de 10 notas; si no, ErrNoMatch. Devuelve la incidencia actualizada.
	AddNote(ctx context.Context, id string, n domain.Note) (domain.Incident, error)
	// Close cierra la incidencia solo si está abierta; si no, ErrNoMatch.
	// Devuelve la incidencia actualizada.
	Close(ctx context.Context, id string, f domain.FinalResponse) (domain.Incident, error)
}

// Deps son las dependencias del Service.
type Deps struct {
	Repo     Repository
	Bookings Bookings
	Images   imgport.Service
	Users    users.Directory
	Notifier notify.Notifier
	Log      *slog.Logger
	Now      func() time.Time // nil = time.Now
	Location *time.Location   // nil = America/Mexico_City
}

// Viewer es quien hace la petición.
type Viewer struct {
	ID    string
	Admin bool
}

// OpenInput son los datos para abrir una incidencia.
type OpenInput struct {
	Reason      domain.Reason
	Description string
	Evidence    []images.Ref
}

// View es una incidencia lista para responder: con las dos partes, las URL
// firmadas de la evidencia (en el orden de Incident.Evidence) y las notas que
// corresponden a quien consulta (todas para el administrador, solo las
// visibles para las partes).
type View struct {
	Incident domain.Incident
	Reporter users.PublicUser
	Reported users.PublicUser
	URLs     []images.URLs
	Notes    []domain.Note
	// AdminView indica que Notes incluye las internas.
	AdminView bool
}

// Page es una página de incidencias; Next es nil al final.
type Page struct {
	Items []View
	Next  *domain.Cursor
}

// Límites de las páginas y tipos de aviso (RF-10).
const (
	DefaultPage = 20
	MaxPage     = 50
	// DeletedName es el nombre que se muestra si la persona ya no existe.
	DeletedName = "Cuenta eliminada"

	EventOpened = "incident.opened"
	EventNote   = "incident.note"
	EventClosed = "incident.closed"
)
