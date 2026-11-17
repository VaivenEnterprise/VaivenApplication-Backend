// Package domain contiene las reglas de las incidencias (spec 010, T005): los
// motivos, los dos estados, quién puede abrirlas y cuándo, las notas y el
// cierre. Solo biblioteca estándar y los dominios compartidos (Art. III).
package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// MessageInvalidData es el mensaje general de validation_failed.
const MessageInvalidData = "Revisa los datos marcados."

// Reason es el motivo de una incidencia (RF-05).
type Reason string

// Motivos.
const (
	ReasonNoShow         Reason = "no_show"
	ReasonLateReturn     Reason = "late_return"
	ReasonDamaged        Reason = "damaged"
	ReasonNotReturned    Reason = "not_returned"
	ReasonPaymentDispute Reason = "payment_dispute"
	ReasonOther          Reason = "other"
)

// Valid indica si el motivo existe.
func (r Reason) Valid() bool {
	switch r {
	case ReasonNoShow, ReasonLateReturn, ReasonDamaged, ReasonNotReturned, ReasonPaymentDispute, ReasonOther:
		return true
	}
	return false
}

// Status es el estado de una incidencia: solo hay dos (RF-06).
type Status string

// Estados.
const (
	StatusOpen   Status = "open"
	StatusClosed Status = "closed"
)

// Valid indica si el estado existe.
func (s Status) Valid() bool { return s == StatusOpen || s == StatusClosed }

// Outcome es la decisión registrada al cerrar (RF-08). Solo la registra: no
// cobra ni restringe nada por sí misma.
type Outcome string

// Resultados.
const (
	OutcomeNone               Outcome = "none"
	OutcomeMonetaryAdjustment Outcome = "monetary_adjustment"
	OutcomeRestriction        Outcome = "restriction"
)

// Valid indica si el resultado existe.
func (o Outcome) Valid() bool {
	return o == OutcomeNone || o == OutcomeMonetaryAdjustment || o == OutcomeRestriction
}

// Límites de texto y de cantidad (RF-05, RF-08).
const (
	DescriptionMin = 10
	DescriptionMax = 1000
	MaxEvidence    = 5
	MaxNotes       = 10
	NoteMin        = 3
	NoteMax        = 1000
	FinalMin       = 5
	FinalMax       = 1000
)

// Note es una nota del administrador. Las partes solo ven las que tienen
// VisibleToParties.
type Note struct {
	Text             string
	At               time.Time
	ByAdminID        string
	VisibleToParties bool
}

// FinalResponse es la respuesta con la que se cerró la incidencia; la ven
// ambas partes (CA-07).
type FinalResponse struct {
	Text      string
	Outcome   Outcome
	ByAdminID string
	At        time.Time
}

// Incident es el reporte de un incumplimiento vinculado a una reserva. Abrirlo
// no cambia el estado de la reserva (RF-06).
type Incident struct {
	ID          string
	BookingID   string
	ReporterID  string
	ReportedID  string
	Reason      Reason
	Description string
	Evidence    []images.Image
	Status      Status
	Notes       []Note
	Final       *FinalResponse
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    *time.Time
	Version     int64
}

// IsParty indica si userID es quien reporta o la parte reportada.
func (i Incident) IsParty(userID string) bool {
	return userID != "" && (userID == i.ReporterID || userID == i.ReportedID)
}

// VisibleNotes devuelve las notas que ven las partes.
func (i Incident) VisibleNotes() []Note {
	out := make([]Note, 0, len(i.Notes))
	for _, n := range i.Notes {
		if n.VisibleToParties {
			out = append(out, n)
		}
	}
	return out
}

// Cursor marca la posición de una página de incidencias: por fecha de creación
// y, a igual fecha, por id.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// trimmedLen recorta s y devuelve el texto y su longitud en caracteres.
func trimmedLen(s string) (string, int) {
	t := strings.TrimSpace(s)
	return t, utf8.RuneCountInString(t)
}

func invalid(field, issue string) error {
	return shared.NewValidation(MessageInvalidData, shared.Detail{Field: field, Issue: issue})
}

func lengthIssue(minLen, maxLen int) string {
	return fmt.Sprintf("debe tener entre %d y %d caracteres", minLen, maxLen)
}
