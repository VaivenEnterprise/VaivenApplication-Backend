// Package http es el adaptador HTTP del chat (spec 008, T006): las 5 rutas de
// api/paths/chat.yaml, con el límite de 30 mensajes por minuto por persona.
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

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/usecase"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

// Service son los casos de uso que usa el adaptador (usecase.Service).
type Service interface {
	List(ctx context.Context, userID string, after *domain.Cursor, limit int) (usecase.Page, error)
	UnreadCount(ctx context.Context, userID string) (int, error)
	Messages(ctx context.Context, userID, conversationID string, q usecase.MessageQuery) (usecase.MessagePage, error)
	Send(ctx context.Context, userID, conversationID, body string, clientID *string) (domain.Message, bool, error)
	Read(ctx context.Context, userID, conversationID string) error
}

// ModuleDeps son las dependencias del módulo.
type ModuleDeps struct {
	Service Service
	// Authenticate es el middleware del módulo auth (spec 003).
	Authenticate func(http.Handler) http.Handler
	Log          *slog.Logger
	// Límite de mensajes (RF-11, POL-11): Redis con respaldo local.
	Limiter         ratelimit.Limiter
	LimiterFallback ratelimit.Limiter
}

// Module registra las rutas del chat bajo /api/v1 (Art. XII.3).
type Module struct {
	svc   Service
	log   *slog.Logger
	auth  func(http.Handler) http.Handler
	limit func(http.Handler) http.Handler
}

// NewModule crea el módulo. Falla si falta una dependencia obligatoria.
func NewModule(d ModuleDeps) (*Module, error) {
	if d.Service == nil || d.Authenticate == nil || d.Log == nil || d.Limiter == nil {
		return nil, errors.New("chat: faltan dependencias del módulo HTTP")
	}
	return &Module{
		svc: d.Service, log: d.Log, auth: d.Authenticate,
		limit: middleware.RateLimit(d.Limiter, d.LimiterFallback, middleware.RateLimitConfig{
			Limit: policy.POL11MessagesPerMinute, Window: time.Minute, KeyPrefix: "chat:",
			UserID: func(r *http.Request) (string, bool) { return auth.UserID(r.Context()) },
		}, d.Log),
	}, nil
}

// Name identifica el módulo.
func (m *Module) Name() string { return "chat" }

// Register monta las 5 rutas con sesión; unread-count (estática) va antes de
// {id}. El límite solo aplica a enviar mensajes (RF-11).
func (m *Module) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(m.auth)
		r.Get("/conversations", m.list)
		r.Get("/conversations/unread-count", m.unread)
		r.Get("/conversations/{id}/messages", m.messages)
		r.With(m.limit).Post("/conversations/{id}/messages", m.send)
		r.Post("/conversations/{id}/read", m.read)
	})
}

func userID(r *http.Request) string {
	id, _ := auth.UserID(r.Context())
	return id
}

func (m *Module) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !errors.As(err, &de) {
		m.log.ErrorContext(r.Context(), "chat: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

func invalid(field, issue string) error {
	return shared.NewValidation(domain.MessageInvalidData, shared.Detail{Field: field, Issue: issue})
}

func parseLimit(r *http.Request, maxN int) (int, error) {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > maxN {
		return 0, invalid("limit", "debe ser un entero entre 1 y "+strconv.Itoa(maxN))
	}
	return n, nil
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

// --- respuestas ---

type messageResponse struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	SenderID       string    `json:"senderId"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"createdAt"`
	ClientID       *string   `json:"clientId"`
}

func toMessage(m domain.Message) messageResponse {
	return messageResponse{ID: m.ID, ConversationID: m.ConversationID, SenderID: m.SenderID, Body: m.Body, CreatedAt: m.CreatedAt.UTC(), ClientID: m.ClientID}
}

type publicUserResponse struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"displayName"`
	RatingAverage *float64  `json:"ratingAverage"`
	RatingCount   int64     `json:"ratingCount"`
	MemberSince   time.Time `json:"memberSince"`
}

type itemResponse struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	ThumbURL *string `json:"thumbUrl"`
}

type lastMessageResponse struct {
	Preview  string    `json:"preview"`
	At       time.Time `json:"at"`
	SenderID string    `json:"senderId"`
}

// conversationResponse es Conversation (api/openapi.yaml). unreadCount es
// solo el propio: nunca se expone el de la otra parte (RF-08).
type conversationResponse struct {
	ID            string               `json:"id"`
	BookingID     string               `json:"bookingId"`
	BookingStatus string               `json:"bookingStatus"`
	Counterpart   publicUserResponse   `json:"counterpart"`
	Item          itemResponse         `json:"item"`
	CanWrite      bool                 `json:"canWrite"`
	LastMessage   *lastMessageResponse `json:"lastMessage"`
	UnreadCount   int                  `json:"unreadCount"`
}

func toConversation(v usecase.ConversationView) conversationResponse {
	c, p := v.Conversation, v.Counterpart
	out := conversationResponse{
		ID: c.ID, BookingID: c.BookingID, BookingStatus: v.Booking.Status,
		Counterpart: publicUserResponse{ID: p.ID, DisplayName: p.DisplayName, RatingAverage: p.RatingAverage, RatingCount: p.RatingCount, MemberSince: p.MemberSince.UTC()},
		Item:        itemResponse{ID: v.Booking.ItemID, Title: v.Booking.ItemTitle, ThumbURL: v.ThumbURL},
		CanWrite:    v.CanWrite, UnreadCount: v.Unread,
	}
	if lm := c.LastMessage; lm != nil {
		out.LastMessage = &lastMessageResponse{Preview: lm.Preview, At: lm.At.UTC(), SenderID: lm.SenderID}
	}
	return out
}

// --- cursor de conversaciones: base64url("<ms>.<id>") ---

func encodeCursor(c *domain.Cursor) *string {
	if c == nil {
		return nil
	}
	s := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.At.UnixMilli(), 10) + "." + c.ID))
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
	return domain.Cursor{At: time.UnixMilli(n).UTC(), ID: id}, nil
}

// --- handlers ---

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r, usecase.MaxConversations)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	var after *domain.Cursor
	if s := r.URL.Query().Get("cursor"); s != "" {
		c, err := decodeCursor(s)
		if err != nil {
			m.fail(w, r, invalid("cursor", "no es válido"))
			return
		}
		after = &c
	}
	page, err := m.svc.List(r.Context(), userID(r), after, limit)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	out := struct {
		Items      []conversationResponse `json:"items"`
		NextCursor *string                `json:"nextCursor"`
	}{Items: make([]conversationResponse, 0, len(page.Items)), NextCursor: encodeCursor(page.Next)}
	for _, v := range page.Items {
		out.Items = append(out.Items, toConversation(v))
	}
	respond.JSON(w, http.StatusOK, out)
}

func (m *Module) unread(w http.ResponseWriter, r *http.Request) {
	n, err := m.svc.UnreadCount(r.Context(), userID(r))
	if err != nil {
		m.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, struct {
		Count int `json:"count"`
	}{n})
}

func (m *Module) messages(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r, usecase.MaxMessages)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	q := usecase.MessageQuery{Before: r.URL.Query().Get("before"), After: r.URL.Query().Get("after"), Limit: limit}
	for field, v := range map[string]string{"before": q.Before, "after": q.After} {
		if v != "" && !isObjectIDHex(v) {
			m.fail(w, r, invalid(field, "debe ser el id de un mensaje"))
			return
		}
	}
	page, err := m.svc.Messages(r.Context(), userID(r), chi.URLParam(r, "id"), q)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	out := struct {
		Items      []messageResponse `json:"items"`
		NextCursor *string           `json:"nextCursor"`
	}{Items: make([]messageResponse, 0, len(page.Items)), NextCursor: page.Next}
	for _, msg := range page.Items {
		out.Items = append(out.Items, toMessage(msg))
	}
	respond.JSON(w, http.StatusOK, out)
}

type sendRequest struct {
	Body     *string `json:"body"`
	ClientID *string `json:"clientId"`
}

func (m *Module) send(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil || dec.More() {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			m.fail(w, r, invalid(typeErr.Field, "tipo de dato inválido"))
			return
		}
		m.fail(w, r, shared.NewValidation("El cuerpo de la petición no es un JSON válido."))
		return
	}
	if req.Body == nil {
		m.fail(w, r, invalid("body", "es obligatorio"))
		return
	}
	msg, created, err := m.svc.Send(r.Context(), userID(r), chi.URLParam(r, "id"), *req.Body, req.ClientID)
	if err != nil {
		m.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK // mismo clientId (CA-03)
	}
	respond.JSON(w, status, toMessage(msg))
}

func (m *Module) read(w http.ResponseWriter, r *http.Request) {
	if err := m.svc.Read(r.Context(), userID(r), chi.URLParam(r, "id")); err != nil {
		m.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
