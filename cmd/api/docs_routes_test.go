package main

// Swagger UI por entorno (spec 020, T005; RF-17, CA-11): las 3 rutas de /docs
// responden 200 en development y staging y no existen (404 con el sobre de
// error estándar) en production. Se arma lo mismo que cmd/api: config.Load con
// el entorno, docsHandler y el router real.

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

// docsEnv fija el mínimo de variables de un despliegue (datos sintéticos).
func docsEnv(t *testing.T, appEnv string) {
	t.Helper()
	for k, v := range map[string]string{
		"APP_ENV":               appEnv,
		"MONGO_URI":             "mongodb://localhost:27017/?replicaSet=rs0",
		"MONGO_DB":              "vaiven_docs_test",
		"CLOUDINARY_CLOUD_NAME": "nube",
		"CLOUDINARY_API_KEY":    "clave",
		"CLOUDINARY_API_SECRET": "secreto",
		"JWT_ACCESS_SECRET":     "0123456789abcdef0123456789abcdef-docs-test",
		"DOCS_ENABLED":          "",
	} {
		t.Setenv(k, v)
	}
	if appEnv == config.EnvStaging || appEnv == config.EnvProduction {
		t.Setenv("REDIS_URL", "redis://:clave@redis:6379/0")
		t.Setenv("TLS_CERT_FILE", "/certs/fullchain.pem")
		t.Setenv("TLS_KEY_FILE", "/certs/privkey.pem")
	}
}

func docsRouter(t *testing.T, appEnv string) http.Handler {
	t.Helper()
	docsEnv(t, appEnv)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load(%s): %v", appEnv, err)
	}
	return apihttp.NewRouter(apihttp.Deps{
		Log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Docs: docsHandler(cfg.DocsEnabled),
	})
}

var docsRoutes = []struct{ path, contentType, contains string }{
	{"/docs", "text/html", "swagger-ui-bundle.js"},
	{"/docs/init.js", "text/javascript", "SwaggerUIBundle"},
	{"/docs/openapi.yaml", "application/yaml", "openapi: 3.1.0"},
}

func TestSwaggerUIServedInDevelopmentAndStaging(t *testing.T) {
	for _, appEnv := range []string{config.EnvDevelopment, config.EnvStaging} {
		h := docsRouter(t, appEnv)
		for _, rt := range docsRoutes {
			t.Run(appEnv+" "+rt.path, func(t *testing.T) {
				rec := do(h, http.MethodGet, rt.path, "")
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, quería 200", rec.Code)
				}
				if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, rt.contentType) {
					t.Errorf("Content-Type = %q, quería %q", ct, rt.contentType)
				}
				if !strings.Contains(rec.Body.String(), rt.contains) {
					t.Errorf("el cuerpo no contiene %q", rt.contains)
				}
				if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
					t.Errorf("CSP de /docs inesperada: %q", csp)
				}
			})
		}
	}
}

func TestSwaggerUINotFoundInProduction(t *testing.T) {
	h := docsRouter(t, config.EnvProduction)
	for _, rt := range append(docsRoutes, struct{ path, contentType, contains string }{"/docs/", "", ""}) {
		t.Run(rt.path, func(t *testing.T) {
			assertError(t, do(h, http.MethodGet, rt.path, ""), http.StatusNotFound, "not_found")
		})
	}
}

// TestProductionRefusesToStartWithDocs: pedir /docs en producción es un error
// de configuración, no una ruta abierta.
func TestProductionRefusesToStartWithDocs(t *testing.T) {
	docsEnv(t, config.EnvProduction)
	t.Setenv("DOCS_ENABLED", "true")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DOCS_ENABLED") {
		t.Fatalf("config.Load con DOCS_ENABLED=true en production = %v; debe rechazarlo", err)
	}
}
