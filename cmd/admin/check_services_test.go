package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

const secret = "s3cr3t-cloudinary"

func okCheck(name string) check {
	return check{name: name, run: func(context.Context) error { return nil }}
}

func failCheck(name, msg string) check {
	return check{name: name, run: func(context.Context) error { return errors.New(msg) }}
}

func loadOK() (config.Config, error) {
	return config.Config{Cloudinary: config.Cloudinary{CloudName: "n", APIKey: "k", APISecret: config.Secret(secret)}}, nil
}

// CA-S1: todo responde → OK por servicio y código 0.
func TestCheckServicesAllOK(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"check-services"}, &out, &errb, loadOK, func(config.Config) []check {
		return []check{okCheck("MongoDB"), okCheck("Redis"), okCheck("Cloudinary")}
	})
	if code != 0 {
		t.Fatalf("código = %d, want 0; stderr=%s", code, errb.String())
	}
	for _, s := range []string{"MongoDB", "Redis", "Cloudinary"} {
		if !strings.Contains(out.String(), "OK       "+s) {
			t.Errorf("falta OK de %s:\n%s", s, out.String())
		}
	}
}

// CA-S2: Cloudinary rechaza las credenciales → FALLA sin el secreto y código ≠ 0.
func TestCheckServicesFailureHidesSecret(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"check-services"}, &out, &bytes.Buffer{}, loadOK, func(config.Config) []check {
		return []check{
			okCheck("MongoDB"),
			{name: "Redis", skip: "REDIS_URL no está definida"},
			failCheck("Cloudinary", "cloudinary: credenciales rechazadas (HTTP 401)"),
		}
	})
	if code == 0 {
		t.Fatal("con un servicio en FALLA el código debe ser ≠ 0")
	}
	if !strings.Contains(out.String(), "FALLA    Cloudinary") || !strings.Contains(out.String(), "OMITIDO  Redis") {
		t.Errorf("salida:\n%s", out.String())
	}
	if strings.Contains(out.String(), secret) {
		t.Errorf("la salida revela el secreto:\n%s", out.String())
	}
}

func TestRunUsageAndErrors(t *testing.T) {
	never := func(config.Config) []check { t.Fatal("no debía armar verificaciones"); return nil }

	for _, args := range [][]string{nil, {"a", "b"}, {"otro"}} {
		var errb bytes.Buffer
		if code := run(args, &bytes.Buffer{}, &errb, loadOK, never); code != 2 {
			t.Errorf("args %v: código = %d, want 2", args, code)
		}
		if !strings.Contains(errb.String(), "check-services") {
			t.Errorf("args %v: falta el uso en stderr: %s", args, errb.String())
		}
	}

	var errb bytes.Buffer
	badLoad := func() (config.Config, error) {
		return config.Config{}, errors.New("configuración inválida: MONGO_URI: obligatoria")
	}
	if code := run([]string{"check-services"}, &bytes.Buffer{}, &errb, badLoad, never); code != 1 {
		t.Errorf("config inválida: código = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "MONGO_URI") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestServiceChecksSkips(t *testing.T) {
	// Sin Redis ni Cloudinary (APP_ENV=test): ambos se omiten y no cuentan como falla.
	checks := serviceChecks(config.Config{MongoURI: "mongodb://127.0.0.1:1/?directConnection=true", MongoConnectTimeout: 200 * time.Millisecond})
	if len(checks) != 3 || checks[1].skip == "" || checks[2].skip == "" {
		t.Fatalf("checks = %+v", checks)
	}
	var out bytes.Buffer
	if runChecks(context.Background(), &out, checks) {
		t.Error("MongoDB inaccesible debía fallar")
	}
	if !strings.Contains(out.String(), "FALLA    MongoDB") || strings.Count(out.String(), "OMITIDO") != 2 {
		t.Errorf("salida:\n%s", out.String())
	}
}

func TestServiceChecksWithAllConfigured(t *testing.T) {
	cfg := config.Config{
		MongoURI:            "mongodb://127.0.0.1:1/?directConnection=true",
		MongoConnectTimeout: 200 * time.Millisecond,
		Redis:               config.Redis{URL: "redis://:clave-redis@127.0.0.1:1/0"},
		AppEnv:              "staging",
		Cloudinary:          config.Cloudinary{CloudName: "n", APIKey: "k", APISecret: config.Secret(secret)},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var out bytes.Buffer
	runChecks(ctx, &out, serviceChecks(cfg))
	if strings.Contains(out.String(), "clave-redis") || strings.Contains(out.String(), secret) {
		t.Errorf("la salida revela secretos:\n%s", out.String())
	}
	if strings.Count(out.String(), "FALLA") < 2 {
		t.Errorf("MongoDB y Redis inaccesibles debían fallar:\n%s", out.String())
	}
}
