// Package usecase contiene el caso de uso de las sanciones (spec 010, T008,
// POL-15): registrar una advertencia o una suspensión. Depende solo de puertos
// (Art. III y XII.4).
package usecase

import (
	"context"
	"log/slog"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// Repository guarda las sanciones (adapter/mongo.Repo).
type Repository interface {
	// NewID genera el identificador de una sanción.
	NewID() string
	Create(ctx context.Context, s domain.Sanction) (domain.Sanction, error)
}

// Incidents indica si existe una incidencia; lo implementa el repositorio del
// módulo de incidencias.
type Incidents interface {
	Exists(ctx context.Context, id string) (bool, error)
}

// Deps son las dependencias del Service.
type Deps struct {
	Repo      Repository
	Incidents Incidents
	// Directory comprueba que la persona sancionada existe.
	Directory users.Directory
	// Admin suspende la cuenta (users.Admin, spec 003).
	Admin    users.Admin
	Notifier notify.Notifier
	Log      *slog.Logger
	Now      func() time.Time // nil = time.Now
}

// EventApplied es el tipo del aviso de sanción aplicada (RF-10).
const EventApplied = "sanction.applied"

// Input son los datos de una sanción (RF-09).
type Input struct {
	Type       domain.Type
	Reason     string
	EndsAt     *time.Time
	IncidentID *string
}
