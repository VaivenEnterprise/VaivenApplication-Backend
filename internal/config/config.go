// Package config lee y valida la configuración del servicio desde variables
// de entorno (spec 002, plan §4.1). Es la única fuente de configuración: no hay
// valores escondidos en el código ni archivos de configuración (Art. VI).
package config

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// Entornos válidos de APP_ENV.
const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"
)

// Config es la configuración validada del servicio.
type Config struct {
	AppEnv     string
	AppVersion string

	HTTPAddr         string
	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	ShutdownTimeout  time.Duration

	MongoURI            string
	MongoDB             string
	MongoConnectTimeout time.Duration

	CORSAllowedOrigins []string
	LogLevel           slog.Level
}

// IsProduction indica si el servicio corre en producción (logs en JSON, plan P-04).
func (c Config) IsProduction() bool {
	return c.AppEnv == EnvProduction
}

// Error reúne todas las variables faltantes o inválidas, no solo la primera.
type Error struct {
	Problems []string
}

func (e *Error) Error() string {
	return "configuración inválida: " + strings.Join(e.Problems, "; ")
}

// Load lee la configuración del entorno del proceso.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

// lookupFunc tiene la firma de os.LookupEnv; las pruebas pasan un mapa.
type lookupFunc func(key string) (string, bool)

// load aplica los valores por defecto de plan §4.1 y valida todo de una vez.
func load(lookup lookupFunc) (Config, error) {
	r := reader{lookup: lookup}

	cfg := Config{
		AppEnv:              r.oneOf("APP_ENV", EnvDevelopment, EnvDevelopment, EnvTest, EnvProduction),
		AppVersion:          r.text("APP_VERSION", "dev"),
		HTTPAddr:            r.text("HTTP_ADDR", ":8080"),
		HTTPReadTimeout:     r.duration("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTPWriteTimeout:    r.duration("HTTP_WRITE_TIMEOUT", 15*time.Second),
		ShutdownTimeout:     r.duration("SHUTDOWN_TIMEOUT", 15*time.Second),
		MongoURI:            r.required("MONGO_URI"),
		MongoDB:             r.required("MONGO_DB"),
		MongoConnectTimeout: r.duration("MONGO_CONNECT_TIMEOUT", 10*time.Second),
		CORSAllowedOrigins:  r.list("CORS_ALLOWED_ORIGINS"),
		LogLevel:            r.logLevel("LOG_LEVEL", slog.LevelInfo),
	}

	if len(r.problems) > 0 {
		return Config{}, &Error{Problems: r.problems}
	}
	return cfg, nil
}

// reader lee variables y acumula los problemas en lugar de cortar en el primero.
type reader struct {
	lookup   lookupFunc
	problems []string
}

// value devuelve el valor sin espacios; una variable vacía cuenta como ausente.
func (r *reader) value(key string) (string, bool) {
	v, ok := r.lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (r *reader) fail(key, issue string) {
	r.problems = append(r.problems, key+": "+issue)
}

func (r *reader) text(key, def string) string {
	if v, ok := r.value(key); ok {
		return v
	}
	return def
}

func (r *reader) required(key string) string {
	v, ok := r.value(key)
	if !ok {
		r.fail(key, "obligatoria")
	}
	return v
}

func (r *reader) oneOf(key, def string, allowed ...string) string {
	v, ok := r.value(key)
	if !ok {
		return def
	}
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	r.fail(key, "debe ser uno de "+strings.Join(allowed, ", "))
	return def
}

func (r *reader) duration(key string, def time.Duration) time.Duration {
	v, ok := r.value(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		r.fail(key, "duración inválida (ejemplos: 500ms, 10s, 1m)")
		return def
	}
	if d <= 0 {
		r.fail(key, "debe ser mayor que cero")
		return def
	}
	return d
}

func (r *reader) list(key string) []string {
	v, ok := r.value(key)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (r *reader) logLevel(key string, def slog.Level) slog.Level {
	v, ok := r.value(key)
	if !ok {
		return def
	}
	var level slog.Level
	switch strings.ToLower(v) {
	case "debug", "info", "warn", "error":
		_ = level.UnmarshalText([]byte(v))
		return level
	}
	r.fail(key, "debe ser debug, info, warn o error")
	return def
}
