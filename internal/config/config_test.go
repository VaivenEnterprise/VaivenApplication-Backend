package config

import (
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

// env simula variables de entorno para load.
type env map[string]string

func (e env) lookup(key string) (string, bool) {
	v, ok := e[key]
	return v, ok
}

// valid devuelve el mínimo de variables obligatorias en development.
func valid() env {
	return env{
		"MONGO_URI":             "mongodb://localhost:27017/?replicaSet=rs0",
		"MONGO_DB":              "vaiven_dev",
		"CLOUDINARY_CLOUD_NAME": "nube",
		"CLOUDINARY_API_KEY":    "clave",
		"CLOUDINARY_API_SECRET": "secreto",
		"JWT_ACCESS_SECRET":     jwtSecret,
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(valid().lookup)
	if err != nil {
		t.Fatalf("load: error inesperado: %v", err)
	}

	want := Config{
		AppEnv:              EnvDevelopment,
		AppVersion:          "dev",
		HTTPAddr:            ":8080",
		HTTPReadTimeout:     10 * time.Second,
		HTTPWriteTimeout:    15 * time.Second,
		ShutdownTimeout:     15 * time.Second,
		MongoURI:            "mongodb://localhost:27017/?replicaSet=rs0",
		MongoDB:             "vaiven_dev",
		MongoConnectTimeout: 10 * time.Second,
		CORSAllowedOrigins:  nil,
		LogLevel:            slog.LevelInfo,
		Cloudinary: Cloudinary{
			CloudName:  "nube",
			APIKey:     "clave",
			APISecret:  "secreto",
			BaseFolder: "vaiven/development",
		},
		Auth: Auth{
			AccessSecret:    Secret(jwtSecret),
			AccessTTL:       15 * time.Minute,
			RefreshTTL:      720 * time.Hour,
			RateLimitPerMin: 10,
		},
		Images: Images{MaxBytes: 5 * 1024 * 1024, SignaturesPerHour: 30},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("load:\n got %+v\nwant %+v", cfg, want)
	}
}

func TestLoadEmptyMeansDefault(t *testing.T) {
	e := valid()
	for _, k := range []string{"APP_ENV", "APP_VERSION", "HTTP_ADDR", "HTTP_READ_TIMEOUT", "LOG_LEVEL", "CORS_ALLOWED_ORIGINS"} {
		e[k] = " "
	}

	cfg, err := load(e.lookup)
	if err != nil {
		t.Fatalf("load: error inesperado: %v", err)
	}
	if cfg.AppEnv != EnvDevelopment || cfg.AppVersion != "dev" || cfg.HTTPAddr != ":8080" ||
		cfg.HTTPReadTimeout != 10*time.Second || cfg.LogLevel != slog.LevelInfo || cfg.CORSAllowedOrigins != nil {
		t.Errorf("una variable vacía debe usar su valor por defecto: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	e := deployed() // producción exige además Redis y TLS
	e["APP_ENV"] = "production"
	e["APP_VERSION"] = "1.2.3"
	e["HTTP_ADDR"] = ":9090"
	e["HTTP_READ_TIMEOUT"] = "3s"
	e["HTTP_WRITE_TIMEOUT"] = "4s"
	e["SHUTDOWN_TIMEOUT"] = "30s"
	e["MONGO_CONNECT_TIMEOUT"] = "500ms"
	e["CORS_ALLOWED_ORIGINS"] = " https://a.mx , https://b.mx ,,"
	e["LOG_LEVEL"] = "WARN"

	cfg, err := load(e.lookup)
	if err != nil {
		t.Fatalf("load: error inesperado: %v", err)
	}
	if cfg.AppEnv != EnvProduction || cfg.AppVersion != "1.2.3" || cfg.HTTPAddr != ":9090" {
		t.Errorf("valores de texto no aplicados: %+v", cfg)
	}
	if cfg.HTTPReadTimeout != 3*time.Second || cfg.HTTPWriteTimeout != 4*time.Second ||
		cfg.ShutdownTimeout != 30*time.Second || cfg.MongoConnectTimeout != 500*time.Millisecond {
		t.Errorf("duraciones no aplicadas: %+v", cfg)
	}
	if want := []string{"https://a.mx", "https://b.mx"}; !reflect.DeepEqual(cfg.CORSAllowedOrigins, want) {
		t.Errorf("CORSAllowedOrigins = %q, want %q", cfg.CORSAllowedOrigins, want)
	}
	if cfg.LogLevel != slog.LevelWarn {
		t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, slog.LevelWarn)
	}
	if !cfg.IsProduction() {
		t.Error("IsProduction() = false con APP_ENV=production")
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name string
		edit func(env)
		vars []string // variables que el error debe nombrar
	}{
		{"falta MONGO_URI", func(e env) { delete(e, "MONGO_URI") }, []string{"MONGO_URI"}},
		{"MONGO_URI vacía", func(e env) { e["MONGO_URI"] = "  " }, []string{"MONGO_URI"}},
		{"falta MONGO_DB", func(e env) { delete(e, "MONGO_DB") }, []string{"MONGO_DB"}},
		{"duración inválida", func(e env) { e["HTTP_READ_TIMEOUT"] = "diez" }, []string{"HTTP_READ_TIMEOUT"}},
		{"duración no positiva", func(e env) { e["SHUTDOWN_TIMEOUT"] = "0s" }, []string{"SHUTDOWN_TIMEOUT"}},
		{"LOG_LEVEL inválido", func(e env) { e["LOG_LEVEL"] = "verbose" }, []string{"LOG_LEVEL"}},
		{"APP_ENV inválido", func(e env) { e["APP_ENV"] = "qa" }, []string{"APP_ENV"}},
		{
			"varias a la vez",
			func(e env) {
				delete(e, "MONGO_URI")
				e["MONGO_CONNECT_TIMEOUT"] = "x"
				e["LOG_LEVEL"] = "ruidoso"
			},
			[]string{"MONGO_URI", "MONGO_CONNECT_TIMEOUT", "LOG_LEVEL"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := valid()
			tt.edit(e)

			_, err := load(e.lookup)
			if err == nil {
				t.Fatal("load: se esperaba error")
			}
			var cerr *Error
			if !errors.As(err, &cerr) {
				t.Fatalf("load: error de tipo %T, se esperaba *Error", err)
			}
			if len(cerr.Problems) != len(tt.vars) {
				t.Errorf("Problems = %q, se esperaban %d", cerr.Problems, len(tt.vars))
			}
			for _, v := range tt.vars {
				if !strings.Contains(err.Error(), v) {
					t.Errorf("el error %q no nombra %s", err, v)
				}
			}
		})
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://db:27017")
	t.Setenv("MONGO_DB", "vaiven_test")
	t.Setenv("APP_ENV", "test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: error inesperado: %v", err)
	}
	if cfg.MongoDB != "vaiven_test" || cfg.AppEnv != EnvTest {
		t.Errorf("Load no leyó el entorno: %+v", cfg)
	}
	if cfg.IsProduction() {
		t.Error("IsProduction() = true con APP_ENV=test")
	}
}
