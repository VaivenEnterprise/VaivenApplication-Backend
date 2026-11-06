package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
)

// Logging registra cada petición al terminar (RF-11): método, ruta, estado,
// duración y requestId. No registra cuerpo, encabezados ni la query string,
// porque pueden llevar datos personales o tokens (Art. VI.5). Debe montarse
// después de RequestID para que el requestId esté en el contexto.
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			log.LogAttrs(r.Context(), slog.LevelInfo, "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Float64("durationMs", float64(time.Since(start).Microseconds())/1000),
				slog.String("requestId", respond.RequestID(r.Context())),
			)
		})
	}
}

// statusRecorder recuerda el primer estado definitivo que escribió el handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote && code >= http.StatusOK {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

// Unwrap permite a http.ResponseController llegar al ResponseWriter original.
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
