// Package logger crea el logger estructurado del servicio con log/slog
// (spec 002, plan P-04; Art. IX): JSON en producción y texto en otro caso.
//
// Quien registra es responsable de no incluir datos personales, tokens ni
// cuerpos de petición (Art. VI.5).
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// envProduction es el valor de APP_ENV que activa la salida en JSON.
const envProduction = "production"

// New devuelve un logger que escribe en la salida estándar.
// level acepta debug, info, warn o error (sin distinguir mayúsculas).
func New(level, env string) (*slog.Logger, error) {
	return newWithWriter(os.Stdout, level, env)
}

func newWithWriter(w io.Writer, level, env string) (*slog.Logger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	if env == envProduction {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h), nil
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("logger: nivel %q inválido: usa debug, info, warn o error", level)
}
