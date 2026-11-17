package docs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newServer() http.Handler {
	r := chi.NewRouter()
	r.Mount(Prefix, Handler())
	return r
}

func TestRoutes(t *testing.T) {
	cases := []struct {
		path, contentType, contains string
	}{
		{"/docs", "text/html", "swagger-ui-bundle.js"},
		{"/docs/", "text/html", `integrity="` + bundleSRI},
		{"/docs/init.js", "text/javascript", `url: "/docs/openapi.yaml"`},
		{"/docs/openapi.yaml", "application/yaml", "openapi: 3.1.0"},
	}
	srv := newServer()
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.contentType) {
				t.Errorf("Content-Type = %q, want %q", ct, tc.contentType)
			}
			if !strings.Contains(rec.Body.String(), tc.contains) {
				t.Errorf("el cuerpo no contiene %q", tc.contains)
			}
			h := rec.Header()
			if csp := h.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") ||
				!strings.Contains(csp, "frame-ancestors 'none'") || strings.Contains(csp, "unsafe-eval") {
				t.Errorf("CSP inesperada: %q", csp)
			}
			if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" {
				t.Errorf("faltan encabezados de seguridad: %v", h)
			}
		})
	}
}

func TestIndexPinsSwaggerUIVersion(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(SwaggerUIVersion) {
		t.Fatalf("SwaggerUIVersion = %q: debe ser una versión exacta", SwaggerUIVersion)
	}
	if !strings.Contains(indexHTML, "swagger-ui-dist@"+SwaggerUIVersion+"/") {
		t.Error("la página no usa la versión fijada de swagger-ui-dist")
	}
	if strings.Count(indexHTML, "integrity=") != 2 {
		t.Error("cada recurso del CDN debe llevar integrity (SRI)")
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs/../../go.mod", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("status = %d, no debe servir otros archivos", rec.Code)
	}
}

// TestBundleMatchesContract es una guarda local barata contra el desfase entre
// la copia embebida y api/: misma versión, mismas rutas y sin $ref externos.
// La comparación exacta la hace el CI regenerando el bundle con Redocly.
func TestBundleMatchesContract(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("no se pudo leer api/openapi.yaml: %v", err)
	}
	bundle := string(spec)

	version := regexp.MustCompile(`(?m)^  version: (\S+)$`)
	want, got := version.FindStringSubmatch(string(src)), version.FindStringSubmatch(bundle)
	if want == nil || got == nil || want[1] != got[1] {
		t.Errorf("info.version: api=%v bundle=%v; ejecuta make docs-bundle", want, got)
	}

	// Rutas: claves de dos espacios bajo paths: en el contrato fuente.
	pathKey := regexp.MustCompile(`(?m)^  (/\S*):\s*$`)
	for _, m := range pathKey.FindAllStringSubmatch(string(src), -1) {
		if !strings.Contains(bundle, "\n  "+m[1]+":") {
			t.Errorf("la ruta %s falta en el bundle; ejecuta make docs-bundle", m[1])
		}
	}

	if regexp.MustCompile(`\$ref: ['"]?\.?\.?/?paths/`).MatchString(bundle) {
		t.Error("el bundle tiene $ref externos; regenera con make docs-bundle")
	}
}
