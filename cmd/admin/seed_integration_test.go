//go:build integration

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	authusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

// seedTestConfig es una configuración de prueba: secreto JWT ficticio y
// carpeta de imágenes de prueba (las fotos son locales, sin Cloudinary).
func seedTestConfig() config.Config {
	var c config.Config
	c.AppEnv = config.EnvTest
	c.Auth.AccessSecret = config.Secret("secreto-jwt-ficticio-de-prueba-0123456789")
	c.Auth.AccessTTL = 15 * time.Minute
	c.Auth.RefreshTTL = 24 * time.Hour
	c.Cloudinary.BaseFolder = "vaiven/test"
	return c
}

// CA-02 sobre MongoDB real: seed dos veces no duplica; las reservas quedan en
// pending, accepted y cancelled; las personas inician sesión con la
// contraseña de su archivo.
func TestIntegrationSeedTwice(t *testing.T) {
	client, _ := testMongo(t)
	db := client.Database("vaiven_seed")
	cfg := seedTestConfig()
	open := func(ctx context.Context, _ config.Config, _ bool) (seedDeps, error) {
		d, err := seedDepsOn(ctx, cfg, db, localImages{baseFolder: cfg.Cloudinary.BaseFolder}, localPhotos{baseFolder: cfg.Cloudinary.BaseFolder})
		if err != nil {
			return seedDeps{}, err
		}
		return d, nil
	}
	path := writeCreds(t, nil)
	env := func(string) string { return path }

	for run := 1; run <= 2; run++ {
		var out, errb bytes.Buffer
		if code := runSeed(nil, &out, &errb, cfg, env, open); code != 0 {
			t.Fatalf("ejecución %d: código %d\n%s\n%s", run, code, out.String(), errb.String())
		}
		if run == 2 && !strings.Contains(out.String(), "personas 0 nuevas/7 existentes, objetos 0/15, reservas 0/3") {
			t.Fatalf("la segunda ejecución creó datos:\n%s", out.String())
		}
	}

	ctx := context.Background()
	count := func(col string, filter bson.D) int64 {
		n, err := db.Collection(col).CountDocuments(ctx, filter)
		if err != nil {
			t.Fatalf("contar %s: %v", col, err)
		}
		return n
	}
	if n := count("users", bson.D{}); n != 7 {
		t.Errorf("users = %d, want 7", n)
	}
	if n := count("users", bson.D{{Key: "role", Value: "admin"}}); n != 1 {
		t.Errorf("admins = %d, want 1", n)
	}
	if n := count("items", bson.D{}); n != 15 {
		t.Errorf("items = %d, want 15", n)
	}
	for _, st := range []string{"pending", "accepted", "cancelled"} {
		if n := count("bookings", bson.D{{Key: "status", Value: st}}); n != 1 {
			t.Errorf("reservas %s = %d, want 1", st, n)
		}
	}
	if n := count("conversations", bson.D{}); n != 3 {
		t.Errorf("conversaciones = %d, want 3 (una por reserva)", n)
	}

	// Las contraseñas son las del archivo (y la de una persona no abre otra).
	d, err := open(ctx, cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	svc := d.auth.(*authusecase.Service)
	emails := seedRequiredEmails()
	if _, err := svc.Login(ctx, authusecase.LoginInput{Email: emails[1], Password: testPassword(1)}); err != nil {
		t.Errorf("login con la contraseña del archivo: %v", err)
	}
	if _, err := svc.Login(ctx, authusecase.LoginInput{Email: emails[1], Password: testPassword(2)}); err == nil {
		t.Error("login con la contraseña de otra cuenta: want error")
	}
}

// --scale crea copias con fotos locales, también de forma idempotente.
func TestIntegrationSeedScale(t *testing.T) {
	client, _ := testMongo(t)
	db := client.Database("vaiven_seed_scale")
	cfg := seedTestConfig()
	open := func(ctx context.Context, _ config.Config, local bool) (seedDeps, error) {
		if !local {
			t.Fatal("--scale debe usar fotos locales")
		}
		return seedDepsOn(ctx, cfg, db, localImages{baseFolder: cfg.Cloudinary.BaseFolder}, localPhotos{baseFolder: cfg.Cloudinary.BaseFolder})
	}
	path := writeCreds(t, nil)
	for run := 1; run <= 2; run++ {
		var out, errb bytes.Buffer
		if code := runSeed([]string{"--scale", "2"}, &out, &errb, cfg, func(string) string { return path }, open); code != 0 {
			t.Fatalf("ejecución %d: código %d\n%s", run, code, errb.String())
		}
	}
	n, err := db.Collection("items").CountDocuments(context.Background(), bson.D{})
	if err != nil || n != 45 {
		t.Fatalf("items = %d (err %v), want 45 (15 × 3)", n, err)
	}
}
