package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	allowedOrigin = "https://app.vaiven.mx"
	otherOrigin   = "https://evil.example"
)

func corsHandler(origins ...string) (http.Handler, *bool) {
	called := new(bool)
	h := CORS(origins)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	}))
	return h, called
}

func TestCORSAllowedOrigin(t *testing.T) {
	h, called := corsHandler(allowedOrigin, "http://localhost:8081")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", allowedOrigin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !*called || rec.Code != http.StatusOK {
		t.Fatalf("debe llegar al handler: called=%v status=%d", *called, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("Allow-Origin = %q, want %q", got, allowedOrigin)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != HeaderRequestID {
		t.Errorf("Expose-Headers = %q", got)
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
		t.Errorf("Vary = %q, want Origin", rec.Header().Get("Vary"))
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Error("no debe enviar Allow-Credentials")
	}
}

func TestCORSDoesNotReflectUnlistedOrigins(t *testing.T) {
	tests := []struct {
		name    string
		origins []string
		origin  string
	}{
		{"origen no listado", []string{allowedOrigin}, otherOrigin},
		{"sin encabezado Origin", []string{allowedOrigin}, ""},
		{"lista vacía", nil, allowedOrigin},
		{"comodín no es válido", []string{"*"}, otherOrigin},
		{"coincidencia parcial", []string{allowedOrigin}, allowedOrigin + ".evil.example"},
		{"entradas en blanco", []string{"", "  "}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, called := corsHandler(tt.origins...)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if !*called {
				t.Error("la petición debe seguir al handler")
			}
			for k := range rec.Header() {
				if strings.HasPrefix(k, "Access-Control-") {
					t.Errorf("no debe haber encabezados CORS, hay %s", k)
				}
			}
		})
	}
}

func TestCORSPreflightAllowed(t *testing.T) {
	h, called := corsHandler(allowedOrigin)
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/meta", nil)
	req.Header.Set("Origin", allowedOrigin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if *called {
		t.Error("el preflight no debe llegar al handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	want := map[string]string{
		"Access-Control-Allow-Origin":  allowedOrigin,
		"Access-Control-Allow-Methods": corsAllowMethods,
		"Access-Control-Allow-Headers": corsAllowHeaders,
		"Access-Control-Max-Age":       "600",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestCORSPreflightFromUnlistedOriginContinues(t *testing.T) {
	h, called := corsHandler(allowedOrigin)
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", otherOrigin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !*called {
		t.Error("el preflight de un origen no listado debe seguir al handler")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("no debe reflejar el origen")
	}
}

func TestCORSOptionsWithoutRequestMethodIsNotPreflight(t *testing.T) {
	h, called := corsHandler(allowedOrigin)
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", allowedOrigin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !*called {
		t.Error("un OPTIONS normal debe llegar al handler")
	}
	if rec.Header().Get("Access-Control-Allow-Methods") != "" {
		t.Error("no debe responder como preflight")
	}
}
