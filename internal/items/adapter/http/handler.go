// Package http es el adaptador HTTP del módulo de objetos (spec 005, T007):
// las 8 rutas de api/paths/items.yaml.
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/usecase"
)

// Service son los casos de uso que usa el adaptador (items/usecase.Service).
type Service interface {
	Create(ctx context.Context, viewer usecase.Viewer, in usecase.ItemInput) (usecase.ItemView, error)
	Get(ctx context.Context, viewer usecase.Viewer, id string) (usecase.ItemView, error)
	Update(ctx context.Context, viewer usecase.Viewer, id string, in usecase.UpdateInput) (usecase.ItemView, error)
	Pause(ctx context.Context, viewer usecase.Viewer, id string) (usecase.ItemView, error)
	Resume(ctx context.Context, viewer usecase.Viewer, id string) (usecase.ItemView, error)
	Delete(ctx context.Context, viewer usecase.Viewer, id string) error
	ListMine(ctx context.Context, viewer usecase.Viewer, q usecase.ListQuery) (usecase.Page, error)
	SuggestPrice(declaredValueCents int64) (int64, error)
	Categories() []domain.Category
}

type handlers struct {
	svc Service
	dec decoder
	log *slog.Logger
}

// roleAdmin es el rol de administración (spec 003).
const roleAdmin = "admin"

// viewer lee la identidad que dejó Authenticate.
func viewer(r *http.Request) usecase.Viewer {
	id, _ := auth.UserID(r.Context())
	return usecase.Viewer{ID: id, Admin: auth.Role(r.Context()) == roleAdmin}
}

// fail responde el error; los que no son de dominio se registran (sin
// cuerpos ni datos personales) y salen como internal_error.
func (h handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	var de *shared.Error
	if !errors.As(err, &de) {
		h.log.ErrorContext(r.Context(), "items: error interno", "error", err)
	}
	respond.Error(w, r, err)
}

func (h handlers) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := h.dec.decode(r.Body, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	v, err := h.svc.Create(r.Context(), viewer(r), req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusCreated, toItemResponse(v))
}

func (h handlers) get(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Get(r.Context(), viewer(r), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toItemResponse(v))
}

func (h handlers) update(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := h.dec.decode(r.Body, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.empty() {
		h.fail(w, r, shared.NewValidation("Envía al menos un campo para cambiar."))
		return
	}
	in, err := req.input()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	v, err := h.svc.Update(r.Context(), viewer(r), chi.URLParam(r, "id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toItemResponse(v))
}

func (h handlers) pause(w http.ResponseWriter, r *http.Request) {
	h.status(w, r, h.svc.Pause)
}

func (h handlers) resume(w http.ResponseWriter, r *http.Request) {
	h.status(w, r, h.svc.Resume)
}

func (h handlers) status(w http.ResponseWriter, r *http.Request, op func(context.Context, usecase.Viewer, string) (usecase.ItemView, error)) {
	v, err := op(r.Context(), viewer(r), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, toItemResponse(v))
}

func (h handlers) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), viewer(r), chi.URLParam(r, "id")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handlers) listMine(w http.ResponseWriter, r *http.Request) {
	q, err := parseListQuery(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page, err := h.svc.ListMine(r.Context(), viewer(r), q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := pageResponse{Items: make([]itemResponse, 0, len(page.Items)), NextCursor: encodeCursor(page.Next)}
	for _, v := range page.Items {
		out.Items = append(out.Items, toItemResponse(v))
	}
	respond.JSON(w, http.StatusOK, out)
}

func (h handlers) priceSuggestion(w http.ResponseWriter, r *http.Request) {
	declared, err := parseDeclaredValue(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	cents, err := h.svc.SuggestPrice(declared)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	respond.JSON(w, http.StatusOK, suggestionResponse{SuggestedPricePerDayCents: cents})
}

func (h handlers) categories(w http.ResponseWriter, _ *http.Request) {
	cats := h.svc.Categories()
	out := make([]categoryResponse, 0, len(cats))
	for _, c := range cats {
		out = append(out, categoryResponse{Key: c.Key, Label: c.Label})
	}
	respond.JSON(w, http.StatusOK, out)
}
