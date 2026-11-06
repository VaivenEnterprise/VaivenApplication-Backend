package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
)

var generatedID = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestRequestID(t *testing.T) {
	tests := []struct {
		name   string
		header string
		reuse  bool
	}{
		{"sin encabezado", "", false},
		{"válido", "abc-123-XYZ", true},
		{"64 caracteres", strings.Repeat("a", 64), true},
		{"65 caracteres", strings.Repeat("a", 65), false},
		{"caracter inválido", "abc_123", false},
		{"espacio", "abc 123", false},
		{"no ASCII", "idé", false},
		{"inyección de encabezado", "abc\r\nX-Evil: 1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctxID string
			h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				ctxID = respond.RequestID(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set(HeaderRequestID, tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get(HeaderRequestID)
			if tt.reuse && got != tt.header {
				t.Errorf("X-Request-Id = %q, want %q", got, tt.header)
			}
			if !tt.reuse && !generatedID.MatchString(got) {
				t.Errorf("X-Request-Id = %q, want 32 hex generados", got)
			}
			if ctxID != got {
				t.Errorf("id en contexto = %q, want %q", ctxID, got)
			}
		})
	}
}

func TestRequestIDGeneratesDifferentIDs(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	seen := map[string]bool{}
	for range 20 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		seen[rec.Header().Get(HeaderRequestID)] = true
	}
	if len(seen) != 20 {
		t.Errorf("ids distintos = %d, want 20", len(seen))
	}
}
