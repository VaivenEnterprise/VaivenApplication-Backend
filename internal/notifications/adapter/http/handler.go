// Package http es el adaptador HTTP de los avisos (spec 011, T005): las 5 rutas
// de api/paths/notifications.yaml. Todas exigen sesión y solo tocan los avisos
// y los tokens de quien las llama.
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
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/usecase"
)

// Service son los casos de uso que usa el adaptador (usecase.Service).
type Service interface {
	List(ctx context.Context, userID, after string, limit int) (usecase.Page, error)
	UnreadCount(ctx context.Context, userID string) (int64, error)
	MarkRead(ctx context.Context, userID string, ids []string, all bool) error
	RegisterToken(ctx context.Context, userID, token, platform string) error
	UnregisterToken(ctx context.Context, userID, token string) error
}

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	Log          *slog.Logger
}

// Module registra las rutas de avisos bajo /api/v1 (Art. XII.3).
type Module struct {
	svc  Service
	log  *slog.Logger
	auth func(http.Handler) http.Handler
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.Log == nil {
		return nil, errors.New("notifications: faltan dependencias del módulo HTTP")
	}
	return &Module{svc: d.Service, log: d.Log, auth: d.Authenticate}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "notifications" }

// Register monta las 5 rutas, todas con sesión.
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.auth)
		r.Get("/me/notifications", m.list)
		r.Get("/me/notifications/unread-count", m.unreadCount)
		r.Post("/me/notifications/read", m.markRead)
		r.Post("/me/push-tokens", m.registerToken)
		r.Delete("/me/push-tokens", m.deleteToken)
	})
}

func userID(r *http.Request) string {
	id, _ := auth.UserID(r.Context())
	return id
}

func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !errors.As(err, &de) {
		m.log.ErrorContext(r.Context(), "notifications: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

// decode lee un cuerpo JSON de hasta 8 KiB sin campos de más ni basura al final.
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

func invalid(field, issue string) error {
	return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: field, Issue: issue})
}

// --- respuestas ---

// notificationResponse es Notification (api/openapi.yaml).
type notificationResponse struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Data      map[string]string `json:"data"`
	ReadAt    *string           `json:"readAt"`
	CreatedAt string            `json:"createdAt"`
}

const timeLayout = "2006-01-02T15:04:05.000Z07:00"

func toNotification(n domain.Notification) notificationResponse {
	data := n.Data
	if data == nil {
		data = map[string]string{}
	}
	out := notificationResponse{ID: n.ID, Type: n.Type, Title: n.Title, Body: n.Body, Data: data, CreatedAt: n.CreatedAt.UTC().Format(timeLayout)}
	if n.ReadAt != nil {
		s := n.ReadAt.UTC().Format(timeLayout)
		out.ReadAt = &s
	}
	return out
}

// --- cursor: base64url(<id del último aviso>) ---

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
	if err != nil || !domain.IsObjectID(string(b)) {
		return "", errors.New("cursor inválido")
	}
	return string(b), nil
}

// --- handlers ---

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
	page, err := m.svc.List(r.Context(), userID(r), after, limit)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	out := struct {
		Items      []notificationResponse `json:"items"`
		NextCursor *string                `json:"nextCursor"`
	}{Items: make([]notificationResponse, 0, len(page.Items)), NextCursor: encodeCursor(page.Next)}
	for _, n := range page.Items {
		out.Items = append(out.Items, toNotification(n))
	}
	respond.JSON(w, http.StatusOK, out)
}

func (m *Module) unreadCount(w http.ResponseWriter, r *http.Request) {
	n, err := m.svc.UnreadCount(r.Context(), userID(r))
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, struct {
		Count int64 `json:"count"`
	}{Count: n})
}

type markReadRequest struct {
	IDs []string `json:"ids"`
	All *bool    `json:"all"`
}

func (m *Module) markRead(w http.ResponseWriter, r *http.Request) {
	var req markReadRequest
	if !m.decode(w, r, &req) {
		return
	}
	all := req.All != nil && *req.All
	if err := m.svc.MarkRead(r.Context(), userID(r), req.IDs, all); err != nil {
		m.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type tokenRequest struct {
	Token    *string `json:"token"`
	Platform *string `json:"platform"`
}

func (m *Module) registerToken(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if !m.decode(w, r, &req) {
		return
	}
	if req.Token == nil {
		m.fail(w, r, invalid("token", "es obligatorio"))
		return
	}
	if req.Platform == nil {
		m.fail(w, r, invalid("platform", "es obligatorio"))
		return
	}
	if err := m.svc.RegisterToken(r.Context(), userID(r), *req.Token, *req.Platform); err != nil {
		m.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) deleteToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token *string `json:"token"`
	}
	if !m.decode(w, r, &req) {
		return
	}
	if req.Token == nil {
		m.fail(w, r, invalid("token", "es obligatorio"))
		return
	}
	if err := m.svc.UnregisterToken(r.Context(), userID(r), *req.Token); err != nil {
		m.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
