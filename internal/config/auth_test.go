package config

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// jwtSecret es un valor de prueba de baja entropía (no es un secreto real).
var jwtSecret = strings.Repeat("x", 32)

func TestAuthDefaults(t *testing.T) {
	cfg, err := load(valid().lookup)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := cfg.Auth
	if a.AccessSecret.Reveal() != jwtSecret || a.AccessTTL != 15*time.Minute ||
		a.RefreshTTL != 720*time.Hour || a.RateLimitPerMin != 10 {
		t.Errorf("Auth = %+v", a)
	}
}

func TestAuthOverrides(t *testing.T) {
	e := valid()
	e["JWT_ACCESS_TTL"] = "5m"
	e["REFRESH_TTL"] = "24h"
	e["RATE_LIMIT_AUTH_PER_MIN"] = "3"
	cfg, err := load(e.lookup)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth.AccessTTL != 5*time.Minute || cfg.Auth.RefreshTTL != 24*time.Hour || cfg.Auth.RateLimitPerMin != 3 {
		t.Errorf("Auth = %+v", cfg.Auth)
	}
}

func TestAuthInvalid(t *testing.T) {
	short := strings.Repeat("s", 31)
	tests := []struct {
		name  string
		edit  func(env)
		field string
	}{
		{"falta el secreto", func(e env) { delete(e, "JWT_ACCESS_SECRET") }, "JWT_ACCESS_SECRET: obligatoria"},
		{"secreto de 31", func(e env) { e["JWT_ACCESS_SECRET"] = short }, "JWT_ACCESS_SECRET: debe tener al menos 32"},
		{"TTL inválido", func(e env) { e["JWT_ACCESS_TTL"] = "mucho" }, "JWT_ACCESS_TTL"},
		{"refresh negativo", func(e env) { e["REFRESH_TTL"] = "-1h" }, "REFRESH_TTL"},
		{"límite cero", func(e env) { e["RATE_LIMIT_AUTH_PER_MIN"] = "0" }, "RATE_LIMIT_AUTH_PER_MIN"},
		{"límite no numérico", func(e env) { e["RATE_LIMIT_AUTH_PER_MIN"] = "diez" }, "RATE_LIMIT_AUTH_PER_MIN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := valid()
			tt.edit(e)
			_, err := load(e.lookup)
			var ce *Error
			if !errors.As(err, &ce) {
				t.Fatalf("load = %v, want *Error", err)
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Errorf("error = %q, want que mencione %q", err, tt.field)
			}
			if strings.Contains(err.Error(), short) {
				t.Error("el error revela el valor del secreto")
			}
		})
	}
}

func TestAuthOptionalInTest(t *testing.T) {
	e := valid()
	e["APP_ENV"] = "test"
	delete(e, "JWT_ACCESS_SECRET")
	if _, err := load(e.lookup); err != nil {
		t.Fatalf("en test el secreto es opcional: %v", err)
	}
	e["JWT_ACCESS_SECRET"] = "corto"
	if _, err := load(e.lookup); err == nil {
		t.Error("en test un secreto corto también es un error")
	}
}

func TestAuthSecretMasked(t *testing.T) {
	cfg, err := load(valid().lookup)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out := fmt.Sprintf("%+v", cfg); strings.Contains(out, jwtSecret) {
		t.Errorf("la configuración impresa revela JWT_ACCESS_SECRET: %s", out)
	}
}
