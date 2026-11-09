// Package http es el adaptador HTTP de las incidencias (spec 010, T009): las 6
// rutas de api/paths/incidents.yaml y de las incidencias de administración.
package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// roleAdmin es el valor de auth.Role para un administrador (spec 003).
const roleAdmin = "admin"

// Service son los casos de uso que usa el adaptador (usecase.Service).
type Service interface {
	Open(ctx context.Context, userID, bookingID string, in usecase.OpenInput) (usecase.View, error)
	Get(ctx context.Context, v usecase.Viewer, id string) (usecase.View, error)
	ListMine(ctx context.Context, userID string, status domain.Status, after *domain.Cursor, limit int) (usecase.Page, error)
	AdminList(ctx context.Context, status domain.Status, after *domain.Cursor, limit int) (usecase.Page, error)
	AddNote(ctx context.Context, adminID, id, text string, visibleToParties bool) (usecase.View, error)
	Close(ctx context.Context, adminID, id, finalResponse string, outcome domain.Outcome) (usecase.View, error)
}

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	// RequireAdmin deja pasar solo al rol admin (403 al resto); va después de
	// Authenticate (auth.RequireRole).
	RequireAdmin func(http.Handler) http.Handler
	Log          *slog.Logger
}

// Module registra las rutas de incidencias bajo /api/v1 (Art. XII.3).
type Module struct {
	svc   Service
	log   *slog.Logger
	auth  func(http.Handler) http.Handler
	admin func(http.Handler) http.Handler
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.RequireAdmin == nil || d.Log == nil {
		return nil, errors.New("incidents: faltan dependencias del módulo HTTP")
	}
	return &Module{svc: d.Service, log: d.Log, auth: d.Authenticate, admin: d.RequireAdmin}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "incidents" }

// Register monta las 6 rutas con sesión; las de administración exigen además el
// rol admin (RF-08, CA-05). /me/incidents y /admin/incidents son estáticas.
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.auth)
		r.Post("/bookings/{id}/incidents", m.open)
		r.Get("/incidents/{id}", m.get)
		r.Get("/me/incidents", m.listMine)
		r.Group(func(r chi.Router) {
			r.Use(m.admin)
			r.Get("/admin/incidents", m.adminList)
			r.Post("/admin/incidents/{id}/notes", m.addNote)
			r.Post("/admin/incidents/{id}/close", m.close)
		})
	})
}

func userID(r *http.Request) string {
	id, _ := auth.UserID(r.Context())
	return id
}

func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !errors.As(err, &de) {
		m.log.ErrorContext(r.Context(), "incidents: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

func invalid(field, issue string) error {
	return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: field, Issue: issue})
}

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

// decode lee un único objeto JSON (máx. 16 KiB) en dst.
func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	if err := dec.Decode(dst); err != nil || dec.More() {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return invalid(typeErr.Field, "tipo de dato inválido")
		}
		return shared.NewValidation("El cuerpo de la petición no es un JSON válido.")
	}
	return nil
}

// --- respuestas ---

type publicUserResponse struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"displayName"`
	RatingAverage *float64  `json:"ratingAverage"`
	RatingCount   int64     `json:"ratingCount"`
	MemberSince   time.Time `json:"memberSince"`
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

type noteResponse struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
	// VisibleToParties solo se envía en la vista de administración.
	VisibleToParties *bool `json:"visibleToParties,omitempty"`
}

type finalResponse struct {
	Text    string    `json:"text"`
	Outcome string    `json:"outcome"`
	At      time.Time `json:"at"`
}

// incidentResponse es Incident (api/openapi.yaml).
type incidentResponse struct {
	ID            string             `json:"id"`
	BookingID     string             `json:"bookingId"`
	Reporter      publicUserResponse `json:"reporter"`
	Reported      publicUserResponse `json:"reported"`
	Reason        string             `json:"reason"`
	Description   string             `json:"description"`
	Evidence      []imageResponse    `json:"evidence"`
	Status        string             `json:"status"`
	Notes         []noteResponse     `json:"notes"`
	FinalResponse *finalResponse     `json:"finalResponse"`
	CreatedAt     time.Time          `json:"createdAt"`
	ClosedAt      *time.Time         `json:"closedAt"`
}

func toUser(p users.PublicUser) publicUserResponse {
	return publicUserResponse{
		ID: p.ID, DisplayName: p.DisplayName, RatingAverage: p.RatingAverage,
		RatingCount: p.RatingCount, MemberSince: p.MemberSince.UTC(),
	}
}

func toIncident(v usecase.View) incidentResponse {
	i := v.Incident
	out := incidentResponse{
		ID: i.ID, BookingID: i.BookingID, Reporter: toUser(v.Reporter), Reported: toUser(v.Reported),
		Reason: string(i.Reason), Description: i.Description, Status: string(i.Status),
		Evidence:  make([]imageResponse, 0, len(i.Evidence)),
		Notes:     make([]noteResponse, 0, len(v.Notes)),
		CreatedAt: i.CreatedAt.UTC(),
	}
	for n, img := range i.Evidence {
		ir := imageResponse{
			PublicID: img.PublicID, Width: img.Width, Height: img.Height, Format: img.Format,
			Bytes: img.Bytes, Delivery: string(img.Delivery),
		}
		if n < len(v.URLs) {
			ir.URL, ir.ThumbURL = v.URLs[n].URL, v.URLs[n].ThumbURL
		}
		out.Evidence = append(out.Evidence, ir)
	}
	for _, n := range v.Notes {
		nr := noteResponse{Text: n.Text, At: n.At.UTC()}
		if v.AdminView {
			visible := n.VisibleToParties
			nr.VisibleToParties = &visible
		}
		out.Notes = append(out.Notes, nr)
	}
	if f := i.Final; f != nil {
		out.FinalResponse = &finalResponse{Text: f.Text, Outcome: string(f.Outcome), At: f.At.UTC()}
	}
	if c := i.ClosedAt; c != nil {
		u := c.UTC()
		out.ClosedAt = &u
	}
	return out
}

type pageResponse struct {
	Items      []incidentResponse `json:"items"`
	NextCursor *string            `json:"nextCursor"`
}

func toPage(p usecase.Page) pageResponse {
	out := pageResponse{Items: make([]incidentResponse, 0, len(p.Items)), NextCursor: encodeCursor(p.Next)}
	for _, v := range p.Items {
		out.Items = append(out.Items, toIncident(v))
	}
	return out
}

// --- cursor: base64url("<ms>.<id>") ---

func encodeCursor(c *domain.Cursor) *string {
	if c == nil {
		return nil
	}
	s := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.CreatedAt.UnixMilli(), 10) + "." + c.ID))
	return &s
}

func decodeCursor(s string) (domain.Cursor, error) {
	if len(s) > 128 {
		return domain.Cursor{}, errors.New("cursor largo")
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return domain.Cursor{}, err
	}
	ms, id, ok := strings.Cut(string(b), ".")
	n, errN := strconv.ParseInt(ms, 10, 64)
	if !ok || errN != nil || !isObjectIDHex(id) {
		return domain.Cursor{}, errors.New("cursor inválido")
	}
	return domain.Cursor{CreatedAt: time.UnixMilli(n).UTC(), ID: id}, nil
}

// listQuery lee status, cursor y limit de la consulta.
func listQuery(r *http.Request) (status domain.Status, after *domain.Cursor, limit int, err error) {
	q := r.URL.Query()
	if s := q.Get("status"); s != "" {
		status = domain.Status(s)
		if !status.Valid() {
			return "", nil, 0, invalid("status", "debe ser open o closed")
		}
	}
	if s := q.Get("limit"); s != "" {
		n, errN := strconv.Atoi(s)
		if errN != nil || n < 1 || n > usecase.MaxPage {
			return "", nil, 0, invalid("limit", "debe ser un entero entre 1 y "+strconv.Itoa(usecase.MaxPage))
		}
		limit = n
	}
	if s := q.Get("cursor"); s != "" {
		c, errC := decodeCursor(s)
		if errC != nil {
			return "", nil, 0, invalid("cursor", "no es válido")
		}
		after = &c
	}
	return status, after, limit, nil
}

// --- handlers ---

type openRequest struct {
	Reason      string `json:"reason"`
	Description string `json:"description"`
	Evidence    []struct {
		PublicID string `json:"publicId"`
	} `json:"evidence"`
}

func (m *Module) open(w http.ResponseWriter, r *http.Request) {
	var req openRequest
	if err := decode(w, r, &req); err != nil {
		m.fail(w, r, err)
		return
	}
	in := usecase.OpenInput{Reason: domain.Reason(req.Reason), Description: req.Description}
	for _, e := range req.Evidence {
		in.Evidence = append(in.Evidence, images.Ref{PublicID: e.PublicID})
	}
	v, err := m.svc.Open(r.Context(), userID(r), chi.URLParam(r, "id"), in)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusCreated, toIncident(v))
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	viewer := usecase.Viewer{ID: userID(r), Admin: auth.Role(r.Context()) == roleAdmin}
	v, err := m.svc.Get(r.Context(), viewer, chi.URLParam(r, "id"))
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toIncident(v))
}

func (m *Module) listMine(w http.ResponseWriter, r *http.Request) {
	status, after, limit, err := listQuery(r)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	page, err := m.svc.ListMine(r.Context(), userID(r), status, after, limit)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toPage(page))
}

func (m *Module) adminList(w http.ResponseWriter, r *http.Request) {
	status, after, limit, err := listQuery(r)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	page, err := m.svc.AdminList(r.Context(), status, after, limit)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toPage(page))
}

type noteRequest struct {
	Text             *string `json:"text"`
	VisibleToParties *bool   `json:"visibleToParties"`
}

func (m *Module) addNote(w http.ResponseWriter, r *http.Request) {
	var req noteRequest
	if err := decode(w, r, &req); err != nil {
		m.fail(w, r, err)
		return
	}
	var details []shared.Detail
	if req.Text == nil {
		details = append(details, shared.Detail{Field: "text", Issue: "es obligatorio"})
	}
	if req.VisibleToParties == nil {
		details = append(details, shared.Detail{Field: "visibleToParties", Issue: "es obligatorio"})
	}
	if len(details) > 0 {
		m.fail(w, r, shared.NewValidation(domain.MessageInvalidData, details...))
		return
	}
	v, err := m.svc.AddNote(r.Context(), userID(r), chi.URLParam(r, "id"), *req.Text, *req.VisibleToParties)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toIncident(v))
}

type closeRequest struct {
	FinalResponse *string `json:"finalResponse"`
	Outcome       *string `json:"outcome"`
}

func (m *Module) close(w http.ResponseWriter, r *http.Request) {
	var req closeRequest
	if err := decode(w, r, &req); err != nil {
		m.fail(w, r, err)
		return
	}
	var details []shared.Detail
	if req.FinalResponse == nil {
		details = append(details, shared.Detail{Field: "finalResponse", Issue: "es obligatorio"})
	}
	if req.Outcome == nil {
		details = append(details, shared.Detail{Field: "outcome", Issue: "es obligatorio"})
	}
	if len(details) > 0 {
		m.fail(w, r, shared.NewValidation(domain.MessageInvalidData, details...))
		return
	}
	v, err := m.svc.Close(r.Context(), userID(r), chi.URLParam(r, "id"), *req.FinalResponse, domain.Outcome(*req.Outcome))
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toIncident(v))
}
