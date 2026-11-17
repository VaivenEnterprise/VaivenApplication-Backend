package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
)

// Recover captura un panic del handler, lo registra con el requestId y la pila,
// y responde 500 internal_error sin exponer detalles (RF-10). El servidor
// sigue atendiendo peticiones. http.ErrAbortHandler se vuelve a lanzar: es la
// señal de net/http para abortar la respuesta a propósito.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.LogAttrs(r.Context(), slog.LevelError, "panic recuperado",
					slog.String("panic", fmt.Sprint(v)),
					slog.String("stack", string(debug.Stack())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("requestId", respond.RequestID(r.Context())),
				)
				respond.Error(w, r, errors.New("panic en el handler"))
			}()
			next.ServeHTTP(w, r)
		})
	}
}
