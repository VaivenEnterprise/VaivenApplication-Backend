package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// EventType es el tipo de una interacción.
type EventType string

// Tipos de interacción (RF-01). Las solicitudes no llegan como evento: se leen
// de las reservas (spec 007).
const (
	EventView    EventType = "view"
	EventRequest EventType = "request"
)

// Weight devuelve el peso del tipo; 0 si no existe.
func (t EventType) Weight() float64 {
	switch t {
	case EventView:
		return WeightView
	case EventRequest:
		return WeightRequest
	default:
		return 0
	}
}

// Event es una interacción de una persona con un objeto. Solo lleva
// identificadores, tipo, peso y fecha (RF-09).
type Event struct {
	UserID string
	ItemID string
	Type   EventType
	Weight float64
	At     time.Time
}

// Bucket es el inicio del bloque de ViewBucket que contiene At (en UTC): dos
// vistas del mismo objeto en el mismo bloque cuentan como una.
func (e Event) Bucket() time.Time { return e.At.UTC().Truncate(ViewBucket) }

// NewView valida y crea una vista. rawType es el `type` que mandó la app.
func NewView(userID, itemID, rawType string, at time.Time) (Event, error) {
	var details []shared.Detail
	if !ValidObjectID(itemID) {
		details = append(details, shared.Detail{Field: "itemId", Issue: "debe ser un identificador válido"})
	}
	if EventType(rawType) != EventView {
		details = append(details, shared.Detail{Field: "type", Issue: "solo se acepta view"})
	}
	if len(details) > 0 {
		return Event{}, shared.NewValidation("Revisa los datos del evento.", details...)
	}
	return Event{UserID: userID, ItemID: itemID, Type: EventView, Weight: WeightView, At: at.UTC()}, nil
}

// ValidObjectID indica si s parece un ObjectID de MongoDB (24 hexadecimales en
// minúscula), sin importar el driver.
func ValidObjectID(s string) bool {
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

// ParseLimit lee el `limit` de GET /recommendations: vacío es DefaultLimit;
// fuera de 1–MaxLimit es un error de validación.
func ParseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultLimit, nil
	}
	n := 0
	for _, c := range raw {
		if c < '0' || c > '9' || n > MaxLimit {
			return 0, invalidLimit()
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 || n > MaxLimit {
		return 0, invalidLimit()
	}
	return n, nil
}

func invalidLimit() error {
	return shared.NewValidation("Revisa los parámetros.", shared.Detail{Field: "limit", Issue: "debe estar entre 1 y 20"})
}

// CleanMunicipality valida el `municipality` opcional: vacío es "", y si viene
// debe tener de 2 a 60 caracteres. Solo se usa para el arranque en frío y no se
// guarda (RF-04).
func CleanMunicipality(raw string) (string, error) {
	m := strings.Join(strings.Fields(raw), " ")
	if m == "" {
		return "", nil
	}
	if n := utf8.RuneCountInString(m); n < 2 || n > 60 {
		return "", shared.NewValidation("Revisa los parámetros.", shared.Detail{Field: "municipality", Issue: "debe tener de 2 a 60 caracteres"})
	}
	return m, nil
}
