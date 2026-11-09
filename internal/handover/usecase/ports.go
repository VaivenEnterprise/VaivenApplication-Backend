// Package usecase contiene los casos de uso de la entrega y la devolución
// (spec 009, T004–T005): registrar evidencia, generar el QR, confirmar y
// consultar la etapa. Depende solo de puertos (Art. III y XII.4).
package usecase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

// ErrNoMatch lo devuelve el repositorio cuando la operación atómica no aplicó
// porque la reserva ya no cumple el filtro (otra petición ganó, la evidencia
// cambió, el QR venció o se usó). El caso de uso vuelve a leer la reserva para
// explicar por qué.
var ErrNoMatch = errors.New("handover: la operación atómica no aplicó")

// Bookings lee reservas (lo implementa bookings/adapter/mongo.Repo).
type Bookings interface {
	ByID(ctx context.Context, id string) (bookings.Booking, error)
}

// Repository guarda las etapas dentro de la reserva (plan P-09: solo toca sus
// campos y sube la versión de la reserva). Cada método es una operación
// atómica sobre un documento y devuelve ErrNoMatch si no aplicó.
type Repository interface {
	// SaveEvidence reemplaza la evidencia de la etapa si la reserva sigue en el
	// estado de la etapa y la etapa no está confirmada. Borra el QR.
	SaveEvidence(ctx context.Context, bookingID string, k domain.Kind, h bookings.Handover) error
	// SaveQR guarda el QR si la etapa sigue en la versión de evidencia del QR y
	// sin confirmar.
	SaveQR(ctx context.Context, bookingID string, k domain.Kind, qr bookings.QR) error
	// Confirm consume el QR (tokenHash vigente, sin usar, de la versión actual)
	// y confirma la etapa y el cambio de estado en una sola operación (RNF-02).
	Confirm(ctx context.Context, bookingID string, c domain.Confirmation, tokenHash string) error
}

// Viewer es quien hace la petición.
type Viewer struct{ ID string }

// Deps son las dependencias del Service.
type Deps struct {
	Repo     Repository
	Bookings Bookings
	Images   imgport.Service
	Notifier notify.Notifier
	Log      *slog.Logger
	// Lateness es opcional; sin él la entrega no fija lateAt ni la devolución
	// registra ajustes (los inyecta payments).
	Lateness domain.Lateness
	Now      func() time.Time // nil = time.Now
	Location *time.Location   // nil = America/Mexico_City
	Rand     io.Reader        // nil = crypto/rand
}

// StageView es una etapa lista para responder (plan §2).
type StageView struct {
	Kind domain.Kind
	// Stage es la etapa guardada; nunca nil (una etapa vacía si no hay nada).
	Stage bookings.Handover
	// URLs corresponde a Stage.Photos, en el mismo orden: firmadas y de 10 min.
	URLs []images.URLs
	// QR es el estado del QR en el momento de responder; QRExpiresAt solo si hay
	// uno activo o vencido.
	QR          domain.QRState
	QRExpiresAt *time.Time
}

// QRIssued es el QR recién generado. Token solo existe aquí: no se guarda.
type QRIssued struct {
	Token     string
	Payload   string
	ExpiresAt time.Time
}
