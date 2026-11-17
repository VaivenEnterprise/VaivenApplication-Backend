// Package http es el adaptador HTTP de los reportes (spec 011, T011): las 3
// rutas de api/paths/reports.yaml. POST /reports exige sesión; las rutas
// /admin/* exigen además el rol admin (RF-10).
package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/usecase"
)

// Service son los casos de uso que usa el adaptador (usecase.Service).
type Service interface {
	Create(ctx context.Context, reporterID string, in usecase.Input) (domain.Report, bool, error)
	List(ctx context.Context, status, after string, limit int) (usecase.Page, error)
	Resolve(ctx context.Context, adminID, reportID, action, note string) (usecase.View, error)
}

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	// RequireAdmin deja pasar solo al rol admin (auth.RequireRole).
	RequireAdmin func(http.Handler) http.Handler
	Log          *slog.Logger
}

// Module registra las rutas de reportes bajo /api/v1 (Art. XII.3).
type Module struct {
	svc   Service
	log   *slog.Logger
	auth  func(http.Handler) http.Handler
	admin func(http.Handler) http.Handler
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.RequireAdmin == nil || d.Log == nil {
		return nil, errors.New("reports: faltan dependencias del módulo HTTP")
	}
	return &Module{svc: d.Service, log: d.Log, auth: d.Authenticate, admin: d.RequireAdmin}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "reports" }

// Register monta las 3 rutas: reportar con sesión y las de administración con
// sesión y rol admin.
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.auth)
		r.Post("/reports", m.create)
		r.Group(func(r chi.Router) {
			r.Use(m.admin)
			r.Get("/admin/reports", m.list)
			r.Post("/admin/reports/{id}/resolve", m.resolve)
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
		m.log.ErrorContext(r.Context(), "reports: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

func invalid(field, issue string) error {
	return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: field, Issue: issue})
}

func (m *Module) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil || dec.More() {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			m.fail(w, r, invalid(typeErr.Field, "tipo de dato inválido"))
			return false
		}
		m.fail(w, r, shared.NewValidation("El cuerpo de la petición no es un JSON válido."))
		return false
	}
	return true
}

// --- respuestas ---

const timeLayout = "2006-01-02T15:04:05.000Z07:00"

type reporterResponse struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"displayName"`
	RatingAverage *float64 `json:"ratingAverage"`
	RatingCount   int64    `json:"ratingCount"`
	MemberSince   string   `json:"memberSince"`
}

type resolutionResponse struct {
	Action string `json:"action"`
	Note   string `json:"note"`
	At     string `json:"at"`
}

// reportResponse es Report (api/openapi.yaml). byAdminId se guarda (RF-11) pero
// no se expone.
type reportResponse struct {
	ID         string              `json:"id"`
	TargetType string              `json:"targetType"`
	TargetID   string              `json:"targetId"`
	Reason     string              `json:"reason"`
	Details    *string             `json:"details"`
	Reporter   reporterResponse    `json:"reporter"`
	Status     string              `json:"status"`
	Target     targetResponse      `json:"target"`
	Resolution *resolutionResponse `json:"resolution"`
	CreatedAt  string              `json:"createdAt"`
}

type targetResponse struct {
	Summary string `json:"summary"`
}

func toReport(v usecase.View) reportResponse {
	r := v.Report
	out := reportResponse{
		ID: r.ID, TargetType: string(r.TargetType), TargetID: r.TargetID, Reason: string(r.Reason), Details: r.Details,
		Status: string(r.Status), Target: targetResponse{Summary: v.TargetSummary}, CreatedAt: r.CreatedAt.UTC().Format(timeLayout),
		Reporter: reporterResponse{
			ID: v.Reporter.ID, DisplayName: v.Reporter.DisplayName, RatingAverage: v.Reporter.RatingAverage,
			RatingCount: v.Reporter.RatingCount, MemberSince: v.Reporter.MemberSince.UTC().Format(timeLayout),
		},
	}
	if v.Reporter.ID == "" {
		out.Reporter.ID = r.ReporterID
	}
	if res := r.Resolution; res != nil {
		out.Resolution = &resolutionResponse{Action: string(res.Action), Note: res.Note, At: res.At.UTC().Format(timeLayout)}
	}
	return out
}

// --- cursor: base64url(<id del último reporte>) ---

func encodeCursor(id *string) *string {
	if id == nil {
		return nil
	}
	s := base64.RawURLEncoding.EncodeToString([]byte(*id))
	return &s
}

func decodeCursor(s string) (string, error) {
	if len(s) > 64 {
		return "", errors.New("cursor largo")
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || !isObjectID(string(b)) {
		return "", errors.New("cursor inválido")
	}
	return string(b), nil
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

// --- handlers ---

type createRequest struct {
	TargetType *string `json:"targetType"`
	TargetID   *string `json:"targetId"`
	Reason     *string `json:"reason"`
	Details    *string `json:"details"`
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !m.decode(w, r, &req) {
		return
	}
	for _, f := range []struct {
		name string
		v    *string
	}{{"targetType", req.TargetType}, {"targetId", req.TargetID}, {"reason", req.Reason}} {
		if f.v == nil {
			m.fail(w, r, invalid(f.name, "es obligatorio"))
			return
		}
	}
	rep, created, err := m.svc.Create(r.Context(), userID(r), usecase.Input{
		TargetType: *req.TargetType, TargetID: *req.TargetID, Reason: *req.Reason, Details: req.Details,
	})
	if err != nil {
		m.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	respond.JSON(w, status, struct {
		ID string `json:"id"`
	}{ID: rep.ID})
}

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > usecase.MaxPage {
			m.fail(w, r, invalid("limit", "debe ser un entero entre 1 y "+strconv.Itoa(usecase.MaxPage)))
			return
		}
		limit = n
	}
	after := ""
	if s := r.URL.Query().Get("cursor"); s != "" {
		id, err := decodeCursor(s)
		if err != nil {
			m.fail(w, r, invalid("cursor", "no es válido"))
			return
		}
		after = id
	}
	page, err := m.svc.List(r.Context(), r.URL.Query().Get("status"), after, limit)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	out := struct {
		Items      []reportResponse `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}{Items: make([]reportResponse, 0, len(page.Items)), NextCursor: encodeCursor(page.Next)}
	for _, v := range page.Items {
		out.Items = append(out.Items, toReport(v))
	}
	respond.JSON(w, http.StatusOK, out)
}

type resolveRequest struct {
	Action *string `json:"action"`
	Note   *string `json:"note"`
}

func (m *Module) resolve(w http.ResponseWriter, r *http.Request) {
	var req resolveRequest
	if !m.decode(w, r, &req) {
		return
	}
	if req.Action == nil {
		m.fail(w, r, invalid("action", "es obligatorio"))
		return
	}
	if req.Note == nil {
		m.fail(w, r, invalid("note", "es obligatorio"))
		return
	}
	v, err := m.svc.Resolve(r.Context(), userID(r), chi.URLParam(r, "id"), *req.Action, *req.Note)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toReport(v))
}
