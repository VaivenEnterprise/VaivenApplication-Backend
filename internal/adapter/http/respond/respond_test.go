package respond

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

const wantContentType = "application/json; charset=utf-8"

// decodeError decodifica la respuesta como envoltorio de error.
func decodeError(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("cuerpo no es JSON válido: %v (%q)", err, rec.Body.String())
	}
	body, ok := env["error"]
	if !ok {
		t.Fatalf("falta la clave error: %q", rec.Body.String())
	}
	return body
}

func requestWithID(id string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if id == "" {
		return r
	}
	return r.WithContext(WithRequestID(r.Context(), id))
}

func TestErrorMapsDomainCodes(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"validación", domain.NewValidation("Datos inválidos."), http.StatusBadRequest, "validation_failed"},
		{"no autenticado", domain.NewUnauthorized("Inicia sesión."), http.StatusUnauthorized, "unauthorized"},
		{"prohibido", domain.NewForbidden("Sin permiso."), http.StatusForbidden, "forbidden"},
		{"no encontrado", domain.NewNotFound("No existe."), http.StatusNotFound, "not_found"},
		{"conflicto", domain.NewConflict("Ya existe."), http.StatusConflict, "conflict"},
		{"envuelto con %w", fmt.Errorf("get thing: %w", domain.NewNotFound("No existe.")), http.StatusNotFound, "not_found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Error(rec, requestWithID("req-123"), tt.err)

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			if ct := rec.Header().Get("Content-Type"); ct != wantContentType {
				t.Errorf("Content-Type = %q, want %q", ct, wantContentType)
			}
			body := decodeError(t, rec)
			if body["code"] != tt.code {
				t.Errorf("code = %v, want %q", body["code"], tt.code)
			}
			if body["requestId"] != "req-123" {
				t.Errorf("requestId = %v, want req-123", body["requestId"])
			}
			if msg, _ := body["message"].(string); msg == "" {
				t.Error("message vacío")
			}
			if _, ok := body["details"]; ok {
				t.Error("details no debe aparecer si no hay detalles")
			}
		})
	}
}

func TestErrorUsesDomainMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, requestWithID("x"), domain.NewConflict("El correo ya está registrado."))

	if got := decodeError(t, rec)["message"]; got != "El correo ya está registrado." {
		t.Errorf("message = %v", got)
	}
}

func TestErrorValidationDetails(t *testing.T) {
	err := domain.NewValidation("Datos inválidos.",
		domain.Detail{Field: "name", Issue: "es obligatorio"},
		domain.Detail{Field: "email", Issue: "no es válido"},
	)
	rec := httptest.NewRecorder()
	Error(rec, requestWithID("req-1"), err)

	details, ok := decodeError(t, rec)["details"].([]any)
	if !ok || len(details) != 2 {
		t.Fatalf("details = %v, want 2 elementos", decodeError(t, rec)["details"])
	}
	first := details[0].(map[string]any)
	if first["field"] != "name" || first["issue"] != "es obligatorio" {
		t.Errorf("details[0] = %v", first)
	}
}

func TestErrorInternalIsGeneric(t *testing.T) {
	secret := "mongodb://user:password@host/db"
	tests := []struct {
		name string
		err  error
	}{
		{"error genérico", errors.New(secret)},
		{"nil", nil},
		{"código de dominio desconocido", &domain.Error{Code: "otro", Message: "detalle interno"}},
		{"causa interna oculta", domain.NewNotFound("No existe.").Wrap(errors.New(secret))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Error(rec, requestWithID("req-9"), tt.err)

			wantStatus := http.StatusInternalServerError
			if strings.Contains(tt.name, "causa") {
				wantStatus = http.StatusNotFound
			}
			if rec.Code != wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, wantStatus)
			}
			if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "detalle interno") {
				t.Errorf("la respuesta filtra información interna: %q", rec.Body.String())
			}
			if wantStatus != http.StatusInternalServerError {
				return
			}
			body := decodeError(t, rec)
			if body["code"] != "internal_error" {
				t.Errorf("code = %v, want internal_error", body["code"])
			}
			if body["message"] != "Ocurrió un error inesperado. Intenta de nuevo." {
				t.Errorf("message = %v", body["message"])
			}
			if body["requestId"] != "req-9" {
				t.Errorf("requestId = %v, want req-9", body["requestId"])
			}
		})
	}
}

func TestErrorWithoutRequestID(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, requestWithID(""), domain.NewNotFound("No existe."))

	body := decodeError(t, rec)
	if id, ok := body["requestId"]; !ok || id != "" {
		t.Errorf("requestId = %v (presente=%v), want cadena vacía", id, ok)
	}
}

func TestJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusCreated, map[string]string{"status": "ok"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if ct := rec.Header().Get("Content-Type"); ct != wantContentType {
		t.Errorf("Content-Type = %q, want %q", ct, wantContentType)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Errorf("body = %q", got)
	}
}

func TestJSONNilWritesOnlyStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusNoContent, nil)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want vacío", rec.Body.String())
	}
}

func TestJSONEncodeFailureIsInternalError(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusOK, make(chan int))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := decodeError(t, rec)
	if body["code"] != "internal_error" || body["message"] != "Ocurrió un error inesperado. Intenta de nuevo." {
		t.Errorf("body = %v", body)
	}
}

func TestRequestIDContext(t *testing.T) {
	if got := RequestID(context.Background()); got != "" {
		t.Errorf("RequestID sin valor = %q, want vacío", got)
	}
	ctx := WithRequestID(context.Background(), "abc")
	if got := RequestID(ctx); got != "abc" {
		t.Errorf("RequestID = %q, want abc", got)
	}
}
