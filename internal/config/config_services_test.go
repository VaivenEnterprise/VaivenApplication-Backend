package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// deployed devuelve un entorno staging completo y válido.
func deployed() env {
	e := valid()
	e["APP_ENV"] = "staging"
	e["REDIS_URL"] = "redis://:clave-redis@redis:6379/0"
	e["TLS_CERT_FILE"] = "/etc/letsencrypt/live/host/fullchain.pem"
	e["TLS_KEY_FILE"] = "/etc/letsencrypt/live/host/privkey.pem"
	return e
}

func TestServicesDeployedValid(t *testing.T) {
	for _, appEnv := range []string{"staging", "production"} {
		t.Run(appEnv, func(t *testing.T) {
			e := deployed()
			e["APP_ENV"] = appEnv

			cfg, err := load(e.lookup)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if !cfg.Redis.Enabled() || cfg.Redis.URL.Reveal() != "redis://:clave-redis@redis:6379/0" {
				t.Errorf("Redis = %+v", cfg.Redis)
			}
			if !cfg.TLS.Enabled() || cfg.TLS.KeyFile != "/etc/letsencrypt/live/host/privkey.pem" {
				t.Errorf("TLS = %+v", cfg.TLS)
			}
			if cfg.Cloudinary.BaseFolder != "vaiven/"+appEnv {
				t.Errorf("BaseFolder = %q", cfg.Cloudinary.BaseFolder)
			}
		})
	}
}

func TestServicesTable(t *testing.T) {
	tests := []struct {
		name string
		base func() env
		edit func(env)
		vars []string // variables que el error debe nombrar; vacío = sin error
	}{
		{"staging sin Redis ni TLS", deployed, func(e env) {
			delete(e, "REDIS_URL")
			delete(e, "TLS_CERT_FILE")
			delete(e, "TLS_KEY_FILE")
		}, []string{"REDIS_URL", "TLS_CERT_FILE", "TLS_KEY_FILE"}},
		{"production sin Cloudinary", deployed, func(e env) {
			e["APP_ENV"] = "production"
			delete(e, "CLOUDINARY_CLOUD_NAME")
			delete(e, "CLOUDINARY_API_KEY")
			delete(e, "CLOUDINARY_API_SECRET")
		}, []string{"CLOUDINARY_CLOUD_NAME", "CLOUDINARY_API_KEY", "CLOUDINARY_API_SECRET"}},
		{"development sin Cloudinary", valid, func(e env) {
			delete(e, "CLOUDINARY_API_SECRET")
		}, []string{"CLOUDINARY_API_SECRET"}},
		{"development sin Redis ni TLS es válido", valid, func(env) {}, nil},
		{"test sin Cloudinary, Redis ni TLS es válido", valid, func(e env) {
			e["APP_ENV"] = "test"
			delete(e, "CLOUDINARY_CLOUD_NAME")
			delete(e, "CLOUDINARY_API_KEY")
			delete(e, "CLOUDINARY_API_SECRET")
		}, nil},
		{"REDIS_URL con esquema inválido", valid, func(e env) {
			e["REDIS_URL"] = "http://redis:6379"
		}, []string{"REDIS_URL"}},
		{"TLS a medias en development", valid, func(e env) {
			e["TLS_CERT_FILE"] = "cert.pem"
		}, []string{"TLS_CERT_FILE/TLS_KEY_FILE"}},
		{"faltantes de base y de servicios juntos", deployed, func(e env) {
			delete(e, "MONGO_URI")
			delete(e, "REDIS_URL")
			e["LOG_LEVEL"] = "ruidoso"
		}, []string{"MONGO_URI", "LOG_LEVEL", "REDIS_URL"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.base()
			tt.edit(e)

			_, err := load(e.lookup)
			if len(tt.vars) == 0 {
				if err != nil {
					t.Fatalf("load: error inesperado: %v", err)
				}
				return
			}
			var cerr *Error
			if !errors.As(err, &cerr) {
				t.Fatalf("load: se esperaba *Error, hubo %v", err)
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

func TestStagingAccepted(t *testing.T) {
	cfg, err := load(deployed().lookup)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.AppEnv != EnvStaging || cfg.IsProduction() {
		t.Errorf("AppEnv = %q, IsProduction = %v", cfg.AppEnv, cfg.IsProduction())
	}
}

func TestSecretsAreMasked(t *testing.T) {
	cfg, err := load(deployed().lookup)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	const secret, redisPass = "secreto", "clave-redis"

	var logBuf bytes.Buffer
	slog.New(slog.NewJSONHandler(&logBuf, nil)).Info("config", "cloudinary", cfg.Cloudinary, "redis", cfg.Redis.URL)
	js, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	outputs := map[string]string{
		"%v":     fmt.Sprintf("%v", cfg),
		"%+v":    fmt.Sprintf("%+v", cfg),
		"%#v":    fmt.Sprintf("%#v", cfg),
		"Sprint": fmt.Sprint(cfg.Cloudinary.APISecret),
		"json":   string(js),
		"slog":   logBuf.String(),
	}
	for format, out := range outputs {
		if strings.Contains(out, secret) || strings.Contains(out, redisPass) {
			t.Errorf("%s revela un secreto: %s", format, out)
		}
		if !strings.Contains(out, "***") {
			t.Errorf("%s no muestra ***: %s", format, out)
		}
	}
	if Secret("").String() != "" {
		t.Error("un secreto vacío debe imprimirse vacío")
	}
	if cfg.Cloudinary.APISecret.Reveal() != secret {
		t.Error("Reveal no devuelve el valor real")
	}
}
