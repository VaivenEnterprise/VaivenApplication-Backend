// Package docs sirve Swagger UI y el contrato OpenAPI empaquetado para probar
// la API desde el navegador. Solo se monta en development y staging
// (DOCS_ENABLED, ver internal/config/docs.go); nunca en production.
//
// openapi.yaml es una copia GENERADA de api/openapi.yaml + api/paths/*.yaml
// (vive aquí porque la imagen solo copia cmd/ e internal/). No se edita a mano:
//
//	make docs-bundle
//
// El paso «OpenAPI bundle al día» del CI regenera la copia y falla si difiere;
// TestBundleMatchesContract es una guarda local más barata (versión y rutas).
package docs

import (
	_ "embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// SwaggerUIVersion es la versión fija de swagger-ui-dist que se carga del CDN.
// Al cambiarla hay que recalcular los hashes SRI de abajo:
//
//	curl -fsSL https://cdn.jsdelivr.net/npm/swagger-ui-dist@<v>/<archivo> |
//	  openssl dgst -sha384 -binary | openssl base64 -A
const SwaggerUIVersion = "5.33.3"

const (
	cdnBase   = "https://cdn.jsdelivr.net/npm/swagger-ui-dist@" + SwaggerUIVersion + "/"
	cssSRI    = "sha384-Ov4/wv3j2bmct8cDc5X4ngJZohVPzEmc6uDPH8WeljUxO5vtoykvMEfbu9Vh6RaW"
	bundleSRI = "sha384-h00llMzD1afyNow1pDmnZKlHsUZgp9KE9HXLFXF2CvbwsBY0+WBO4f75SfGwvM4K"
)

// Rutas públicas (el handler se monta en Prefix).
const (
	Prefix   = "/docs"
	specPath = Prefix + "/openapi.yaml"
	initPath = Prefix + "/init.js"
)

//go:embed openapi.yaml
var spec []byte

// contentSecurityPolicy permite solo los recursos de Swagger UI: scripts y
// estilos propios o del CDN fijado (con SRI), y peticiones al mismo origen
// («Try it out» contra esta misma API). Swagger UI usa atributos style en
// línea, por eso style-src lleva 'unsafe-inline'; los scripts no.
const contentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self' https://cdn.jsdelivr.net; " +
	"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
	"img-src 'self' data: https://cdn.jsdelivr.net; " +
	"connect-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

const indexHTML = `<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>VAIVÉN API — Swagger UI</title>
<link rel="stylesheet" href="` + cdnBase + `swagger-ui.css" integrity="` + cssSRI + `" crossorigin="anonymous">
</head>
<body>
<div id="swagger-ui"></div>
<script src="` + cdnBase + `swagger-ui-bundle.js" integrity="` + bundleSRI + `" crossorigin="anonymous"></script>
<script src="` + initPath + `"></script>
</body>
</html>
`

// initJS arranca Swagger UI con el contrato embebido. Sin validador externo
// (validatorUrl: null) y sin guardar el token en localStorage.
const initJS = `window.addEventListener("load", function () {
  window.ui = SwaggerUIBundle({
    url: "` + specPath + `",
    dom_id: "#swagger-ui",
    deepLinking: true,
    validatorUrl: null,
    persistAuthorization: false,
    displayRequestDuration: true
  });
});
`

// Handler devuelve el router de /docs: la página, el arranque y el contrato.
func Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(securityHeaders)
	r.Get("/", serve("text/html; charset=utf-8", []byte(indexHTML)))
	r.Get("/init.js", serve("text/javascript; charset=utf-8", []byte(initJS)))
	r.Get("/openapi.yaml", serve("application/yaml; charset=utf-8", spec))
	return r
}

func serve(contentType string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}
