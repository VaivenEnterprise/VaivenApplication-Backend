package http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/usecase"
)

// --- peticiones ---

type photoRef struct {
	PublicID string `json:"publicId" validate:"required,max=255"`
}

type zoneJSON struct {
	Municipality string  `json:"municipality" validate:"required"`
	Neighborhood *string `json:"neighborhood"`
}

type locationJSON struct {
	Lat *float64 `json:"lat" validate:"required"`
	Lng *float64 `json:"lng" validate:"required"`
}

// createRequest es ItemInput (api/openapi.yaml). validator/v10 comprueba en la
// frontera la presencia y la forma (Art. VI.2); las reglas de negocio las
// valida el dominio con mensajes por campo.
type createRequest struct {
	Title              *string       `json:"title" validate:"required"`
	Description        *string       `json:"description" validate:"required"`
	Category           *string       `json:"category" validate:"required"`
	Condition          *string       `json:"condition" validate:"required"`
	Modality           *string       `json:"modality" validate:"required"`
	DeclaredValueCents *int64        `json:"declaredValueCents" validate:"required"`
	PricePerDayCents   *int64        `json:"pricePerDayCents" validate:"required"`
	PriceSource        *string       `json:"priceSource" validate:"required"`
	UsageRules         *string       `json:"usageRules"`
	Zone               *zoneJSON     `json:"zone" validate:"required"`
	ApproxLocation     *locationJSON `json:"approxLocation"`
	Photos             []photoRef    `json:"photos" validate:"required,min=1,max=5,dive"`
}

func (r createRequest) input() usecase.ItemInput {
	return usecase.ItemInput{
		Title:              *r.Title,
		Description:        *r.Description,
		Category:           *r.Category,
		Condition:          domain.Condition(*r.Condition),
		Modality:           domain.Modality(*r.Modality),
		DeclaredValueCents: *r.DeclaredValueCents,
		PricePerDayCents:   *r.PricePerDayCents,
		PriceSource:        domain.PriceSource(*r.PriceSource),
		UsageRules:         r.UsageRules,
		Zone:               r.Zone.domain(),
		Location:           r.ApproxLocation.domain(),
		Photos:             refs(r.Photos),
	}
}

func (z *zoneJSON) domain() domain.Zone {
	return domain.Zone{Municipality: z.Municipality, Neighborhood: z.Neighborhood}
}

func (l *locationJSON) domain() *domain.Location {
	if l == nil {
		return nil
	}
	return &domain.Location{Lat: *l.Lat, Lng: *l.Lng}
}

func refs(in []photoRef) []images.Ref {
	if in == nil {
		return nil
	}
	out := make([]images.Ref, 0, len(in))
	for _, p := range in {
		out = append(out, images.Ref{PublicID: p.PublicID})
	}
	return out
}

// nullable distingue un campo ausente (Set = false) de uno en null.
type nullable[T any] struct {
	Set   bool
	Value *T
}

// isNull indica si el campo llegó explícitamente en null.
func (n nullable[T]) isNull() bool { return n.Set && n.Value == nil }

func (n *nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		n.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

// updateRequest es ItemUpdate: version obligatoria y al menos otro campo.
// Todos los campos son nullable[T] para distinguir «ausente» de «null»: solo
// usageRules y approxLocation admiten null (los borra); en los demás, null es
// un error (el contrato no los declara anulables).
type updateRequest struct {
	Version            *int64                 `json:"version" validate:"required,min=1"`
	Title              nullable[string]       `json:"title" validate:"-"`
	Description        nullable[string]       `json:"description" validate:"-"`
	Category           nullable[string]       `json:"category" validate:"-"`
	Condition          nullable[string]       `json:"condition" validate:"-"`
	Modality           nullable[string]       `json:"modality" validate:"-"`
	DeclaredValueCents nullable[int64]        `json:"declaredValueCents" validate:"-"`
	PricePerDayCents   nullable[int64]        `json:"pricePerDayCents" validate:"-"`
	PriceSource        nullable[string]       `json:"priceSource" validate:"-"`
	UsageRules         nullable[string]       `json:"usageRules" validate:"-"`
	Zone               nullable[zoneJSON]     `json:"zone" validate:"-"`
	ApproxLocation     nullable[locationJSON] `json:"approxLocation" validate:"-"`
	Photos             nullable[[]photoRef]   `json:"photos" validate:"-"`
}

func (r updateRequest) empty() bool {
	return !r.Title.Set && !r.Description.Set && !r.Category.Set && !r.Condition.Set &&
		!r.Modality.Set && !r.DeclaredValueCents.Set && !r.PricePerDayCents.Set &&
		!r.PriceSource.Set && !r.UsageRules.Set && !r.Zone.Set && !r.ApproxLocation.Set && !r.Photos.Set
}

// nullDetails lista los campos no anulables que llegaron en null.
func (r updateRequest) nullDetails() []shared.Detail {
	var d []shared.Detail
	for _, f := range []struct {
		name string
		null bool
	}{
		{"title", r.Title.isNull()}, {"description", r.Description.isNull()},
		{"category", r.Category.isNull()}, {"condition", r.Condition.isNull()},
		{"modality", r.Modality.isNull()}, {"declaredValueCents", r.DeclaredValueCents.isNull()},
		{"pricePerDayCents", r.PricePerDayCents.isNull()}, {"priceSource", r.PriceSource.isNull()},
		{"zone", r.Zone.isNull()}, {"photos", r.Photos.isNull()},
	} {
		if f.null {
			d = append(d, shared.Detail{Field: f.name, Issue: "no puede ser null"})
		}
	}
	return d
}

func (r updateRequest) input() (usecase.UpdateInput, error) {
	in := usecase.UpdateInput{
		Version:            *r.Version,
		Title:              r.Title.Value,
		Description:        r.Description.Value,
		Category:           r.Category.Value,
		Condition:          (*domain.Condition)(r.Condition.Value),
		Modality:           (*domain.Modality)(r.Modality.Value),
		DeclaredValueCents: r.DeclaredValueCents.Value,
		PricePerDayCents:   r.PricePerDayCents.Value,
		PriceSource:        (*domain.PriceSource)(r.PriceSource.Value),
		SetUsageRules:      r.UsageRules.Set,
		UsageRules:         r.UsageRules.Value,
		SetLocation:        r.ApproxLocation.Set,
	}
	if d := r.nullDetails(); len(d) > 0 {
		return in, shared.NewValidation(domain.MessageInvalidData, d...)
	}
	if p := r.Photos.Value; p != nil {
		if len(*p) < 1 || len(*p) > 5 {
			return in, invalidParam("photos", "debe tener de 1 a 5 fotos")
		}
		for i, ph := range *p {
			if ph.PublicID == "" || len(ph.PublicID) > 255 {
				return in, invalidParam(fmt.Sprintf("photos[%d].publicId", i), "es obligatorio y de máximo 255 caracteres")
			}
		}
		in.Photos = refs(*p)
	}
	if z := r.Zone.Value; z != nil {
		if z.Municipality == "" {
			return in, invalidParam("zone.municipality", "es obligatorio")
		}
		dz := z.domain()
		in.Zone = &dz
	}
	if l := r.ApproxLocation.Value; l != nil {
		if l.Lat == nil || l.Lng == nil {
			return in, invalidParam("approxLocation", "lat y lng son obligatorios")
		}
		in.Location = l.domain()
	}
	return in, nil
}

// decoder lee y valida cuerpos JSON con detalles por campo (es-MX).
type decoder struct{ v *validator.Validate }

func newDecoder() decoder {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			return ""
		}
		return name
	})
	return decoder{v: v}
}

func (d decoder) decode(body io.Reader, dst any) error {
	dec := json.NewDecoder(body)
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return shared.NewValidation("El cuerpo de la petición es demasiado grande.")
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: typeErr.Field, Issue: "tipo de dato inválido"})
		}
		return shared.NewValidation("El cuerpo de la petición no es un JSON válido.")
	}
	if dec.More() {
		return shared.NewValidation("El cuerpo de la petición debe ser un solo objeto JSON.")
	}
	if err := d.v.Struct(dst); err != nil {
		var verrs validator.ValidationErrors
		if !errors.As(err, &verrs) {
			return shared.NewValidation(domain.MessageInvalidData)
		}
		details := make([]shared.Detail, 0, len(verrs))
		for _, fe := range verrs {
			details = append(details, shared.Detail{Field: fieldPath(fe.Namespace()), Issue: issueFor(fe.Tag())})
		}
		return shared.NewValidation(domain.MessageInvalidData, details...)
	}
	return nil
}

// fieldPath quita el nombre del tipo raíz: "createRequest.zone.municipality"
// → "zone.municipality"; "createRequest.photos[0].publicId" igual sin raíz.
func fieldPath(ns string) string {
	_, rest, ok := strings.Cut(ns, ".")
	if !ok {
		return ns
	}
	return rest
}

func issueFor(tag string) string {
	switch tag {
	case "required":
		return "es obligatorio"
	case "max":
		return "excede el máximo permitido"
	case "min":
		return "no alcanza el mínimo permitido"
	default:
		return "valor inválido"
	}
}

// --- parámetros de consulta ---

func invalidParam(field, issue string) error {
	return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: field, Issue: issue})
}

func parseDeclaredValue(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("declaredValueCents")
	if raw == "" {
		return 0, invalidParam("declaredValueCents", "es obligatorio")
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, invalidParam("declaredValueCents", "debe ser un número entero de centavos")
	}
	return v, nil
}

func parseListQuery(r *http.Request) (usecase.ListQuery, error) {
	q := r.URL.Query()
	var lq usecase.ListQuery
	if s := q.Get("status"); s != "" {
		st := domain.Status(s)
		if !domain.ListableStatus(st) {
			return lq, invalidParam("status", "no es una opción válida")
		}
		lq.Status = &st
	}
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > usecase.MaxPageSize {
			return lq, invalidParam("limit", "debe ser un entero entre 1 y 50")
		}
		lq.Limit = n
	}
	if s := q.Get("cursor"); s != "" {
		c, err := decodeCursor(s)
		if err != nil {
			return lq, invalidParam("cursor", "no es válido")
		}
		lq.After = &c
	}
	return lq, nil
}

// El cursor es opaco para el cliente: base64url("<createdAt en ms>.<id>").
func encodeCursor(c *domain.Cursor) *string {
	if c == nil {
		return nil
	}
	s := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.CreatedAt.UnixMilli(), 10) + "." + c.ID))
	return &s
}

func decodeCursor(s string) (domain.Cursor, error) {
	if len(s) > 128 {
		return domain.Cursor{}, errors.New("cursor demasiado largo")
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return domain.Cursor{}, err
	}
	ms, id, ok := strings.Cut(string(b), ".")
	if !ok {
		return domain.Cursor{}, errors.New("cursor sin separador")
	}
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return domain.Cursor{}, err
	}
	if !isObjectIDHex(id) {
		return domain.Cursor{}, errors.New("cursor con id inválido")
	}
	return domain.Cursor{CreatedAt: time.UnixMilli(n).UTC(), ID: id}, nil
}

// --- respuestas ---

type publicUserResponse struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"displayName"`
	RatingAverage *float64  `json:"ratingAverage"`
	RatingCount   int64     `json:"ratingCount"`
	MemberSince   time.Time `json:"memberSince"`
}

type zoneResponse struct {
	Municipality string  `json:"municipality"`
	Neighborhood *string `json:"neighborhood"`
}

type locationResponse struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type imageResponse struct {
	PublicID string `json:"publicId"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Format   string `json:"format"`
	Bytes    int64  `json:"bytes"`
	Delivery string `json:"delivery"`
	URL      string `json:"url"`
	ThumbURL string `json:"thumbUrl"`
}

// itemResponse es Item (api/openapi.yaml). declaredValueCents es null si
// quien consulta no es el dueño ni admin (plan P-09); approxLocation ya está
// redondeada (RNF-02).
type itemResponse struct {
	ID                 string             `json:"id"`
	Owner              publicUserResponse `json:"owner"`
	Title              string             `json:"title"`
	Description        string             `json:"description"`
	Category           string             `json:"category"`
	Condition          string             `json:"condition"`
	Modality           string             `json:"modality"`
	DeclaredValueCents *int64             `json:"declaredValueCents"`
	PricePerDayCents   int64              `json:"pricePerDayCents"`
	PriceSource        string             `json:"priceSource"`
	UsageRules         *string            `json:"usageRules"`
	Zone               zoneResponse       `json:"zone"`
	ApproxLocation     *locationResponse  `json:"approxLocation"`
	Photos             []imageResponse    `json:"photos"`
	Status             string             `json:"status"`
	CreatedAt          time.Time          `json:"createdAt"`
	UpdatedAt          time.Time          `json:"updatedAt"`
	Version            int64              `json:"version"`
}

func toItemResponse(v usecase.ItemView) itemResponse {
	it := v.Item
	out := itemResponse{
		ID: it.ID,
		Owner: publicUserResponse{
			ID: v.Owner.ID, DisplayName: v.Owner.DisplayName, RatingAverage: v.Owner.RatingAverage,
			RatingCount: v.Owner.RatingCount, MemberSince: v.Owner.MemberSince.UTC(),
		},
		Title:            it.Title,
		Description:      it.Description,
		Category:         it.Category,
		Condition:        string(it.Condition),
		Modality:         string(it.Modality),
		PricePerDayCents: it.PricePerDayCents,
		PriceSource:      string(it.PriceSource),
		UsageRules:       it.UsageRules,
		Zone:             zoneResponse{Municipality: it.Zone.Municipality, Neighborhood: it.Zone.Neighborhood},
		Photos:           make([]imageResponse, 0, len(it.Photos)),
		Status:           string(it.Status),
		CreatedAt:        it.CreatedAt.UTC(),
		UpdatedAt:        it.UpdatedAt.UTC(),
		Version:          it.Version,
	}
	if v.ShowDeclaredValue {
		dv := it.DeclaredValueCents
		out.DeclaredValueCents = &dv
	}
	if l := it.Location; l != nil {
		out.ApproxLocation = &locationResponse{Lat: domain.RoundCoord(l.Lat), Lng: domain.RoundCoord(l.Lng)}
	}
	for i, p := range it.Photos {
		img := imageResponse{
			PublicID: p.PublicID, Width: p.Width, Height: p.Height, Format: p.Format,
			Bytes: p.Bytes, Delivery: string(p.Delivery),
		}
		if i < len(v.PhotoURLs) {
			img.URL, img.ThumbURL = v.PhotoURLs[i].URL, v.PhotoURLs[i].ThumbURL
		}
		out.Photos = append(out.Photos, img)
	}
	return out
}

type pageResponse struct {
	Items      []itemResponse `json:"items"`
	NextCursor *string        `json:"nextCursor"`
}

type suggestionResponse struct {
	SuggestedPricePerDayCents int64 `json:"suggestedPricePerDayCents"`
}

type categoryResponse struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// isObjectIDHex indica si s son 24 caracteres hexadecimales en minúsculas.
func isObjectIDHex(s string) bool {
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
