// Package middleware reúne los middleware HTTP de la API (spec 002, plan §4.3).
// El orden de montaje lo define el router: RequestID, Logging, Recover, CORS.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
)

const (
	// HeaderRequestID es el encabezado que identifica cada petición (RF-07).
	HeaderRequestID = "X-Request-Id"

	maxRequestIDLen = 64
)

// RequestID garantiza que toda petición tenga un identificador: reutiliza el
// X-Request-Id entrante si es válido (hasta 64 caracteres alfanuméricos o '-')
// y, si no, genera uno de 32 caracteres hexadecimales (plan P-07). Lo guarda en
// el contexto (respond.RequestID) y lo devuelve en el encabezado de la respuesta.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(respond.WithRequestID(r.Context(), id)))
	})
}

// validRequestID acepta de 1 a 64 caracteres ASCII alfanuméricos o '-'.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '-':
		default:
			return false
		}
	}
	return true
}

// newRequestID devuelve 16 bytes aleatorios en hexadecimal. Desde Go 1.24
// rand.Read no devuelve error: ante un fallo del sistema el programa termina.
func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
