package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

// Valores de CORS que el spec no fija: conjunto mínimo para la API REST con
// token en Authorization (sin cookies, por eso no se envía Allow-Credentials).
const (
	corsAllowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowHeaders = "Authorization, Content-Type, " + HeaderRequestID
	corsMaxAgeSecs   = 600
)

// CORS aplica una lista explícita de orígenes (RF-12, Art. VI.6). Solo un
// origen que coincida exactamente con la lista recibe los encabezados CORS; el
// origen nunca se refleja de otra forma y "*" no es un comodín. Responde 204 a
// los preflight (OPTIONS con Access-Control-Request-Method) de orígenes
// permitidos; los de orígenes no listados siguen al siguiente handler sin
// encabezados CORS. Con la lista vacía no se permite ningún origen.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// La respuesta depende del origen: evita que una caché la reutilice mal.
			w.Header().Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			if _, ok := allowed[origin]; !ok || origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", HeaderRequestID)

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				h.Set("Access-Control-Allow-Methods", corsAllowMethods)
				h.Set("Access-Control-Allow-Headers", corsAllowHeaders)
				h.Set("Access-Control-Max-Age", strconv.Itoa(corsMaxAgeSecs))
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
