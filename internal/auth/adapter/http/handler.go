// Package http es el adaptador HTTP del módulo auth (spec 003, T010): traduce
// las rutas de api/paths/auth.yaml y users.yaml a los casos de uso y expone el
// middleware Authenticate, RequireRole y RequireActive para los demás módulos.
package http

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Service son los casos de uso que usan los handlers. *usecase.Service lo cumple.
type Service interface {
	Register(ctx context.Context, in usecase.RegisterInput) (usecase.AuthResult, error)
	Login(ctx context.Context, in usecase.LoginInput) (usecase.AuthResult, error)
	Refresh(ctx context.Context, token string) (usecase.Tokens, error)
	Logout(ctx context.Context, token string) error
	Me(ctx context.Context, userID string) (domain.User, error)
	UpdateProfile(ctx context.Context, userID string, in usecase.ProfileChanges) (domain.User, error)
	Deactivate(ctx context.Context, userID, password string) error
	PublicProfile(ctx context.Context, id string) (domain.PublicUser, error)
}

type handlers struct {
	svc     Service
	dec     decoder
	log     *slog.Logger
	account accountLimiter
}

// fail responde el error con el envoltorio único y registra solo los fallos
// internos (los de dominio son respuestas esperadas). Los mensajes de error de
// los casos de uso nunca llevan contraseñas ni tokens (RNF-01).
func (h handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !asDomain(err, &de) {
		h.log.ErrorContext(r.Context(), "auth: error interno", "error", err, "path", r.URL.Path)
	}
	respond.Error(w, r, err)
}

func (h handlers) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := h.dec.decode(r.Body, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	res, err := h.svc.Register(r.Context(), usecase.RegisterInput{
		Email: req.Email, Password: req.Password, DisplayName: req.DisplayName,
		AcceptTerms: req.AcceptTerms, IsAdult: req.IsAdult,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusCreated, authResponse{User: toUserResponse(res.User), Tokens: toTokensResponse(res.Tokens)})
}

func (h handlers) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := h.dec.decode(r.Body, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if !h.account.allow(r.Context(), req.Email) {
		w.Header().Set("Retry-After", strconv.Itoa(int(AccountLoginWindow/time.Second)))
		respond.Failure(w, r, http.StatusTooManyRequests, "rate_limited",
			"Hiciste demasiados intentos de inicio de sesión con esta cuenta. Espera unos minutos e intenta de nuevo.")
		return
	}
	res, err := h.svc.Login(r.Context(), usecase.LoginInput{Email: req.Email, Password: req.Password})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, authResponse{User: toUserResponse(res.User), Tokens: toTokensResponse(res.Tokens)})
}

func (h handlers) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := h.dec.decode(r.Body, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	tk, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, refreshResponse{Tokens: toTokensResponse(tk)})
}

func (h handlers) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := h.dec.decode(r.Body, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusNoContent, nil)
}

func (h handlers) getMe(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.UserID(r.Context())
	u, err := h.svc.Me(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toUserResponse(u))
}

func (h handlers) updateMe(w http.ResponseWriter, r *http.Request) {
	var req updateMeRequest
	if err := h.dec.decode(r.Body, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	changes, err := req.changes()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.UserID(r.Context())
	u, err := h.svc.UpdateProfile(r.Context(), id, changes)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toUserResponse(u))
}

func (h handlers) deleteMe(w http.ResponseWriter, r *http.Request) {
	var req deleteMeRequest
	if err := h.dec.decode(r.Body, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.UserID(r.Context())
	if err := h.svc.Deactivate(r.Context(), id, req.Password); err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusNoContent, nil)
}

func (h handlers) getUser(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.PublicProfile(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toPublicUserResponse(p))
}
