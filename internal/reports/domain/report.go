// Package domain contiene las reglas de los reportes de contenido (spec 011,
// T010): tipos de objetivo, motivos, validación y qué acciones admite cada
// tipo de reporte. Solo biblioteca estándar y los dominios compartidos
// (Art. III).
package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// MessageInvalidData es el mensaje general de validation_failed.
const MessageInvalidData = "Revisa los datos marcados."

// Límites (RF-08, plan §2).
const (
	MaxTargetIDLen = 64
	MaxDetailsLen  = 500
	MinNoteLen     = 3
	MaxNoteLen     = 500
	// SuspensionDays es la duración provisional de suspend_user (plan P-07).
	SuspensionDays = 7
)

// TargetType es lo que se reporta.
type TargetType string

// Tipos de objetivo (RF-08).
const (
	TargetItem               TargetType = "item"
	TargetMessage            TargetType = "message"
	TargetUser               TargetType = "user"
	TargetModerationDecision TargetType = "moderation_decision"
)

// Reason es el motivo del reporte.
type Reason string

// Motivos (RF-08).
const (
	ReasonProhibited    Reason = "prohibited_content"
	ReasonInappropriate Reason = "inappropriate"
	ReasonSpam          Reason = "spam"
	ReasonOther         Reason = "other"
)

// Status es el estado del reporte.
type Status string

// Estados.
const (
	StatusOpen      Status = "open"
	StatusActioned  Status = "actioned"
	StatusDismissed Status = "dismissed"
)

// Action es la decisión del administrador (RF-10).
type Action string

// Acciones.
const (
	ActionDismiss     Action = "dismiss"
	ActionHideItem    Action = "hide_item"
	ActionSuspendUser Action = "suspend_user"
)

// Resolution conserva quién decidió, qué y por qué (RF-11).
type Resolution struct {
	Action    Action
	Note      string
	ByAdminID string
	At        time.Time
}

// Report es un reporte de contenido.
type Report struct {
	ID         string
	ReporterID string
	TargetType TargetType
	TargetID   string
	Reason     Reason
	Details    *string
	Status     Status
	Resolution *Resolution
	CreatedAt  time.Time
}

// Errores del dominio.
var (
	// ErrReportNotFound: el reporte no existe.
	ErrReportNotFound = shared.NewNotFound("No encontramos ese reporte.")
	// ErrTargetNotFound: lo reportado no existe o la persona no lo puede ver.
	ErrTargetNotFound = shared.NewNotFound("No encontramos lo que quieres reportar.")
	// ErrOwnTarget: nadie reporta su propio objeto, mensaje o perfil.
	ErrOwnTarget = shared.NewConflict("No puedes reportar tu propio contenido.")
	// ErrAlreadyResolved: el reporte ya tiene una decisión.
	ErrAlreadyResolved = shared.NewConflict("Este reporte ya fue resuelto.")
	// ErrActionNotAllowed: la acción no aplica al tipo de reporte.
	ErrActionNotAllowed = shared.NewConflict("Esa acción no aplica a este tipo de reporte.")
)

func invalid(field, issue string) error {
	return shared.NewValidation(MessageInvalidData, shared.Detail{Field: field, Issue: issue})
}

// ParseTargetType valida el tipo de objetivo.
func ParseTargetType(s string) (TargetType, error) {
	switch t := TargetType(s); t {
	case TargetItem, TargetMessage, TargetUser, TargetModerationDecision:
		return t, nil
	}
	return "", invalid("targetType", "debe ser item, message, user o moderation_decision")
}

// ParseReason valida el motivo.
func ParseReason(s string) (Reason, error) {
	switch r := Reason(s); r {
	case ReasonProhibited, ReasonInappropriate, ReasonSpam, ReasonOther:
		return r, nil
	}
	return "", invalid("reason", "debe ser prohibited_content, inappropriate, spam u other")
}

// ParseStatus valida el estado de un filtro; vacío = open.
func ParseStatus(s string) (Status, error) {
	switch st := Status(s); st {
	case "":
		return StatusOpen, nil
	case StatusOpen, StatusActioned, StatusDismissed:
		return st, nil
	}
	return "", invalid("status", "debe ser open, actioned o dismissed")
}

// ParseAction valida la acción del administrador.
func ParseAction(s string) (Action, error) {
	switch a := Action(s); a {
	case ActionDismiss, ActionHideItem, ActionSuspendUser:
		return a, nil
	}
	return "", invalid("action", "debe ser dismiss, hide_item o suspend_user")
}

// NormalizeTargetID recorta targetId y valida su forma: item, message y user
// llevan un id de 24 hexadecimales; moderation_decision, una referencia de
// hasta 64 caracteres sin espacios ni controles.
func NormalizeTargetID(t TargetType, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", invalid("targetId", "es obligatorio")
	}
	if t == TargetModerationDecision {
		if len(id) > MaxTargetIDLen || strings.IndexFunc(id, func(r rune) bool { return r <= ' ' || r == 0x7f }) >= 0 {
			return "", invalid("targetId", "debe tener hasta 64 caracteres, sin espacios")
		}
		return id, nil
	}
	if !isObjectID(id) {
		return "", invalid("targetId", "no es un id válido")
	}
	return id, nil
}

// NormalizeDetails recorta el detalle opcional; vacío = nil y más de 500
// caracteres es inválido.
func NormalizeDetails(d *string) (*string, error) {
	if d == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*d)
	if t == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(t) > MaxDetailsLen {
		return nil, invalid("details", "debe tener como máximo 500 caracteres")
	}
	return &t, nil
}

// NormalizeNote recorta la nota de la decisión y exige de 3 a 500 caracteres.
func NormalizeNote(n string) (string, error) {
	t := strings.TrimSpace(n)
	if c := utf8.RuneCountInString(t); c < MinNoteLen || c > MaxNoteLen {
		return "", invalid("note", "debe tener entre 3 y 500 caracteres")
	}
	return t, nil
}

// CheckAction indica si la acción se puede aplicar a un reporte del tipo t:
// hide_item solo a objetos; suspend_user a objetos (su dueño), mensajes (quien
// lo escribió) y personas, pero no a decisiones de moderación, que no
// identifican a nadie; dismiss a todos.
func CheckAction(a Action, t TargetType) error {
	switch a {
	case ActionDismiss:
		return nil
	case ActionHideItem:
		if t == TargetItem {
			return nil
		}
	case ActionSuspendUser:
		if t != TargetModerationDecision {
			return nil
		}
	}
	return ErrActionNotAllowed
}

// Resolve devuelve el estado final tras aplicar a: dismiss descarta; las
// demás acciones dejan el reporte atendido.
func (a Action) Resolve() Status {
	if a == ActionDismiss {
		return StatusDismissed
	}
	return StatusActioned
}

func isObjectID(s string) bool {
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
