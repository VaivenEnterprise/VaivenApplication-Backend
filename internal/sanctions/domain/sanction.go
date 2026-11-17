// Package domain contiene las reglas de las sanciones (spec 010, T005,
// POL-15): advertencia o suspensión con fecha fin, decididas manualmente por
// un administrador. No modifican las calificaciones. Solo biblioteca estándar
// y los errores compartidos (Art. III).
package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// MessageInvalidData es el mensaje general de validation_failed.
const MessageInvalidData = "Revisa los datos marcados."

// Type es el tipo de sanción (RF-09).
type Type string

// Tipos.
const (
	TypeWarning    Type = "warning"
	TypeSuspension Type = "suspension"
)

// Valid indica si el tipo existe.
func (t Type) Valid() bool { return t == TypeWarning || t == TypeSuspension }

// Límites del motivo.
const (
	ReasonMin = 5
	ReasonMax = 500
)

// Sanction es una sanción registrada. Es un registro: no se edita ni se borra.
type Sanction struct {
	ID               string
	UserID           string
	Type             Type
	Reason           string
	EndsAt           *time.Time // solo en suspensión
	IncidentID       *string    // incidencia que la motiva (opcional)
	CreatedByAdminID string
	CreatedAt        time.Time
}

// IsObjectID indica si s tiene la forma de un identificador (24 caracteres
// hexadecimales en minúscula).
func IsObjectID(s string) bool {
	if len(s) != 24 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// New valida y arma una sanción para userID (RF-09): el tipo debe existir, el
// motivo tener de 5 a 500 caracteres, la suspensión exige una fecha fin
// posterior a now y la advertencia no lleva fecha. incidentID, si viene, debe
// tener forma de identificador; que exista lo comprueba el caso de uso.
func New(id, userID string, typ Type, reason string, endsAt *time.Time, incidentID *string, adminID string, now time.Time) (Sanction, error) {
	var d []shared.Detail
	if !typ.Valid() {
		d = append(d, shared.Detail{Field: "type", Issue: "debe ser warning o suspension"})
	}
	text := strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(text); n < ReasonMin || n > ReasonMax {
		d = append(d, shared.Detail{Field: "reason", Issue: fmt.Sprintf("debe tener entre %d y %d caracteres", ReasonMin, ReasonMax)})
	}
	switch {
	case typ == TypeSuspension && endsAt == nil:
		d = append(d, shared.Detail{Field: "endsAt", Issue: "es obligatorio en una suspensión"})
	case typ == TypeSuspension && !endsAt.After(now):
		d = append(d, shared.Detail{Field: "endsAt", Issue: "debe ser una fecha futura"})
	case typ == TypeWarning && endsAt != nil:
		d = append(d, shared.Detail{Field: "endsAt", Issue: "una advertencia no lleva fecha fin"})
	}
	if incidentID != nil && !IsObjectID(*incidentID) {
		d = append(d, shared.Detail{Field: "incidentId", Issue: "identificador inválido"})
	}
	if len(d) > 0 {
		return Sanction{}, shared.NewValidation(MessageInvalidData, d...)
	}
	s := Sanction{
		ID: id, UserID: userID, Type: typ, Reason: text, IncidentID: incidentID,
		CreatedByAdminID: adminID, CreatedAt: now.UTC(),
	}
	if endsAt != nil {
		e := endsAt.UTC()
		s.EndsAt = &e
	}
	return s, nil
}
