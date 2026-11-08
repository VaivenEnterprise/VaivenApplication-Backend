// Package domain contiene las reglas de negocio de los objetos (spec 005,
// T003): campos, modalidad y precio, sugerencia de POL-04, redondeo de la
// ubicación, visibilidad por estado y transiciones. Sin infraestructura
// (Art. III).
package domain

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// Condition es el estado físico del objeto.
type Condition string

// Estados físicos (RF-01).
const (
	ConditionNew     Condition = "new"
	ConditionLikeNew Condition = "like_new"
	ConditionGood    Condition = "good"
	ConditionFair    Condition = "fair"
	ConditionWorn    Condition = "worn"
)

// Valid indica si el estado físico existe.
func (c Condition) Valid() bool {
	switch c {
	case ConditionNew, ConditionLikeNew, ConditionGood, ConditionFair, ConditionWorn:
		return true
	}
	return false
}

// Modality es préstamo gratuito o renta por día.
type Modality string

// Modalidades (RF-01).
const (
	ModalityLoan Modality = "loan"
	ModalityRent Modality = "rent"
)

// Valid indica si la modalidad existe.
func (m Modality) Valid() bool { return m == ModalityLoan || m == ModalityRent }

// PriceSource indica si el precio lo escribió la persona o aceptó la
// sugerencia (RF-03).
type PriceSource string

// Orígenes del precio.
const (
	PriceManual    PriceSource = "manual"
	PriceSuggested PriceSource = "suggested"
)

// Valid indica si el origen del precio existe.
func (p PriceSource) Valid() bool { return p == PriceManual || p == PriceSuggested }

// Status es el estado de la publicación.
type Status string

// Estados (plan §3). blocked lo pone moderación (spec 011); removed es la baja
// lógica (RF-09).
const (
	StatusActive  Status = "active"
	StatusPaused  Status = "paused"
	StatusBlocked Status = "blocked"
	StatusRemoved Status = "removed"
)

// ListableStatus indica si s se puede pedir en GET /me/items (todos menos
// removed, RF-10).
func ListableStatus(s Status) bool {
	return s == StatusActive || s == StatusPaused || s == StatusBlocked
}

// Zone es la zona aproximada en texto.
type Zone struct {
	Municipality string
	Neighborhood *string
}

// Location es la ubicación aproximada, ya redondeada a 2 decimales (RF-04).
type Location struct {
	Lat float64
	Lng float64
}

// Item es un objeto publicado.
type Item struct {
	ID                 string
	OwnerID            string
	Title              string
	Description        string
	Category           string
	Condition          Condition
	Modality           Modality
	DeclaredValueCents int64
	PricePerDayCents   int64
	PriceSource        PriceSource
	UsageRules         *string
	Zone               Zone
	Location           *Location
	Photos             []images.Image
	Status             Status
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Version            int64
}

// Límites de texto (RF-01).
const (
	TitleMin           = 3
	TitleMax           = 80
	DescriptionMax     = 1000
	UsageRulesMax      = 500
	MunicipalityMin    = 2
	MunicipalityMax    = 60
	NeighborhoodMax    = 60
	issueRequired      = "es obligatorio"
	issueInvalidOption = "no es una opción válida"
)

// Normalize recorta espacios, convierte en nil los opcionales vacíos y
// redondea la ubicación (RF-04). Se llama antes de Validate.
func (it *Item) Normalize() {
	it.Title = strings.TrimSpace(it.Title)
	it.Description = strings.TrimSpace(it.Description)
	it.Zone.Municipality = strings.TrimSpace(it.Zone.Municipality)
	it.Zone.Neighborhood = trimOptional(it.Zone.Neighborhood)
	it.UsageRules = trimOptional(it.UsageRules)
	if it.Location != nil {
		it.Location = &Location{Lat: RoundCoord(it.Location.Lat), Lng: RoundCoord(it.Location.Lng)}
	}
}

func trimOptional(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// Validate comprueba campos, modalidad y precio (RF-01 a RF-03). Devuelve un
// validation_failed con un detalle por campo (nombres del JSON, es-MX), o nil.
// No revisa las fotos con Cloudinary: eso lo hace images.Service.
func (it Item) Validate() error {
	var d []shared.Detail
	add := func(field, issue string) { d = append(d, shared.Detail{Field: field, Issue: issue}) }

	if n := utf8.RuneCountInString(it.Title); n < TitleMin || n > TitleMax {
		add("title", "debe tener entre 3 y 80 caracteres")
	}
	if utf8.RuneCountInString(it.Description) > DescriptionMax {
		add("description", "debe tener como máximo 1000 caracteres")
	}
	if !ValidCategory(it.Category) {
		add("category", issueInvalidOption)
	}
	if !it.Condition.Valid() {
		add("condition", issueInvalidOption)
	}
	if !ValidDeclaredValue(it.DeclaredValueCents) {
		add("declaredValueCents", "debe estar entre $50 y $100,000")
	}
	if !it.PriceSource.Valid() {
		add("priceSource", issueInvalidOption)
	}
	if it.Modality.Valid() {
		d = append(d, it.priceDetails()...)
	} else {
		add("modality", issueInvalidOption)
	}
	if it.UsageRules != nil && utf8.RuneCountInString(*it.UsageRules) > UsageRulesMax {
		add("usageRules", "debe tener como máximo 500 caracteres")
	}
	if n := utf8.RuneCountInString(it.Zone.Municipality); n < MunicipalityMin || n > MunicipalityMax {
		add("zone.municipality", "debe tener entre 2 y 60 caracteres")
	}
	if it.Zone.Neighborhood != nil && utf8.RuneCountInString(*it.Zone.Neighborhood) > NeighborhoodMax {
		add("zone.neighborhood", "debe tener como máximo 60 caracteres")
	}
	if l := it.Location; l != nil {
		if !finite(l.Lat) || l.Lat < -90 || l.Lat > 90 {
			add("approxLocation.lat", "debe estar entre -90 y 90")
		}
		if !finite(l.Lng) || l.Lng < -180 || l.Lng > 180 {
			add("approxLocation.lng", "debe estar entre -180 y 180")
		}
	}
	if n := len(it.Photos); n < policy.POL13MinItemPhotos || n > policy.POL13MaxItemPhotos {
		add("photos", "debe tener de 1 a 5 fotos")
	}
	if len(d) > 0 {
		return shared.NewValidation(MessageInvalidData, d...)
	}
	return nil
}

// priceDetails aplica RF-02 y RF-03 (la modalidad ya es válida).
func (it Item) priceDetails() []shared.Detail {
	price := func(issue string) []shared.Detail {
		return []shared.Detail{{Field: "pricePerDayCents", Issue: issue}}
	}
	if it.Modality == ModalityLoan {
		if it.PricePerDayCents != policy.POL01LoanPriceCents {
			return price("debe ser 0 en un préstamo")
		}
		if it.PriceSource == PriceSuggested {
			return []shared.Detail{{Field: "priceSource", Issue: "un préstamo no usa precio sugerido"}}
		}
		return nil
	}
	if it.PricePerDayCents < policy.POL01MinRentPriceCents {
		return price("debe ser de al menos $10 en una renta")
	}
	if it.PricePerDayCents > it.DeclaredValueCents {
		return price("no puede superar el valor declarado")
	}
	if it.PriceSource == PriceSuggested && ValidDeclaredValue(it.DeclaredValueCents) &&
		it.PricePerDayCents != SuggestPrice(it.DeclaredValueCents) {
		return price("no coincide con el precio sugerido")
	}
	return nil
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// IsOwner indica si userID es el dueño.
func (it Item) IsOwner(userID string) bool { return userID != "" && it.OwnerID == userID }

// VisibleTo aplica RF-06: active lo ve cualquiera autenticado; paused y
// blocked solo el dueño y admin; removed nadie.
func (it Item) VisibleTo(viewerID string, admin bool) bool {
	switch it.Status {
	case StatusActive:
		return true
	case StatusPaused, StatusBlocked:
		return admin || it.IsOwner(viewerID)
	default:
		return false
	}
}

// ShowsDeclaredValue indica si quien consulta puede ver el valor declarado
// (plan P-09; confirmado el 28 de oct).
func (it Item) ShowsDeclaredValue(viewerID string, admin bool) bool {
	return admin || it.IsOwner(viewerID)
}

// CheckOwner valida que el objeto exista (no removed) y que userID sea el
// dueño (CA-05). Basta para eliminar: un objeto bloqueado se puede dar de baja.
func (it Item) CheckOwner(userID string) error {
	if it.Status == StatusRemoved {
		return ErrNotFound
	}
	if !it.IsOwner(userID) {
		return ErrNotOwner
	}
	return nil
}

// CheckOwnerAction valida que userID pueda editar, pausar o reanudar: además
// de CheckOwner, que el objeto no esté bloqueado por moderación.
func (it Item) CheckOwnerAction(userID string) error {
	if err := it.CheckOwner(userID); err != nil {
		return err
	}
	if it.Status == StatusBlocked {
		return ErrBlocked
	}
	return nil
}

// WithStatus devuelve el estado destino de pausar (pause = true) o reanudar,
// y si hay cambio. Se llama después de CheckOwnerAction.
func (it Item) WithStatus(pause bool) (Status, bool) {
	target := StatusActive
	if pause {
		target = StatusPaused
	}
	return target, it.Status != target
}

// Cursor marca la posición de una página de GET /me/items: el último objeto
// devuelto (orden createdAt descendente y, a igual fecha, id descendente).
type Cursor struct {
	CreatedAt time.Time
	ID        string
}
