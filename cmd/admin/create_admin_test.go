package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

// memStore es un adminStore en memoria con índice único de correo.
type memStore struct {
	users map[string]domain.User
	err   error
}

func (m *memStore) Create(_ context.Context, u domain.User) (domain.User, error) {
	if m.err != nil {
		return domain.User{}, m.err
	}
	if _, ok := m.users[u.Email]; ok {
		return domain.User{}, domain.ErrEmailTaken
	}
	u.ID = "6730f1c2a9b3e4d5f6a7b8c9"
	m.users[u.Email] = u
	return u, nil
}

type fakeHasher struct{ err error }

func (h fakeHasher) Hash(_ context.Context, pw string) (string, error) { return "hash:" + pw, h.err }

// never falla si se arman verificaciones de servicios.
func never(config.Config) []check { panic("no debía armar verificaciones") }

func newAdminDeps(store *memStore, h fakeHasher) adminDeps {
	return adminDeps{store: store, hasher: h, close: func() {}}
}

const adminPwd = "contrasena-admin-de-prueba"

func TestCreateAdmin(t *testing.T) {
	store := &memStore{users: map[string]domain.User{}}
	var out, errb bytes.Buffer
	stdin := strings.NewReader(" Admin@Example.com \n" + adminPwd + "\r\n")
	code := createAdmin(context.Background(), stdin, &out, &errb, newAdminDeps(store, fakeHasher{}), time.Now())
	if code != 0 {
		t.Fatalf("código = %d; stderr=%s", code, errb.String())
	}
	u, ok := store.users["admin@example.com"]
	if !ok || u.Role != domain.RoleAdmin || !u.IsActive() || u.PasswordHash != "hash:"+adminPwd {
		t.Fatalf("cuenta = %+v", u)
	}
	if strings.Contains(out.String()+errb.String(), adminPwd) {
		t.Error("la salida muestra la contraseña")
	}

	// Idempotente: la segunda vez no modifica y sale con ≠ 0.
	errb.Reset()
	stdin = strings.NewReader("admin@example.com\n" + adminPwd + "\n")
	if code := createAdmin(context.Background(), stdin, &out, &errb, newAdminDeps(store, fakeHasher{}), time.Now()); code == 0 ||
		!strings.Contains(errb.String(), "Ya existe") {
		t.Errorf("duplicado: código = %d, stderr = %q", code, errb.String())
	}
}

func TestCreateAdminRejects(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		deps  adminDeps
	}{
		{"correo inválido", "no-es-correo\n" + adminPwd + "\n", newAdminDeps(&memStore{users: map[string]domain.User{}}, fakeHasher{})},
		{"contraseña corta", "a@b.mx\ncorta\n", newAdminDeps(&memStore{users: map[string]domain.User{}}, fakeHasher{})},
		{"sin entrada", "", newAdminDeps(&memStore{users: map[string]domain.User{}}, fakeHasher{})},
		{"hash falla", "a@b.mx\n" + adminPwd + "\n", newAdminDeps(&memStore{users: map[string]domain.User{}}, fakeHasher{err: errors.New("x")})},
		{"repo falla", "a@b.mx\n" + adminPwd + "\n", newAdminDeps(&memStore{err: errors.New("x")}, fakeHasher{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errb bytes.Buffer
			if code := createAdmin(context.Background(), strings.NewReader(tt.stdin), &bytes.Buffer{}, &errb, tt.deps, time.Now()); code != 1 {
				t.Errorf("código = %d, want 1", code)
			}
			if strings.Contains(errb.String(), adminPwd) {
				t.Error("el error muestra la contraseña")
			}
		})
	}
}

func TestRunCreateAdmin(t *testing.T) {
	store := &memStore{users: map[string]domain.User{}}
	factory := func(context.Context, config.Config) (adminDeps, error) { return newAdminDeps(store, fakeHasher{}), nil }
	stdin := strings.NewReader("admin@example.com\n" + adminPwd + "\n")
	if code := run([]string{"create-admin"}, &bytes.Buffer{}, &bytes.Buffer{}, loadOK, never, stdin, factory, nil); code != 0 {
		t.Fatalf("código = %d", code)
	}
	// La contraseña nunca se acepta como argumento.
	if code := run([]string{"create-admin", adminPwd}, &bytes.Buffer{}, &bytes.Buffer{}, loadOK, never, stdin, factory, nil); code != 2 {
		t.Errorf("con argumento extra: código = %d, want 2", code)
	}
	badLoad := func() (config.Config, error) { return config.Config{}, errors.New("sin MONGO_URI") }
	if code := run([]string{"create-admin"}, &bytes.Buffer{}, &bytes.Buffer{}, badLoad, never, stdin, factory, nil); code != 1 {
		t.Errorf("config inválida: código = %d", code)
	}
	failing := func(context.Context, config.Config) (adminDeps, error) { return adminDeps{}, errors.New("sin Mongo") }
	if code := run([]string{"create-admin"}, &bytes.Buffer{}, &bytes.Buffer{}, loadOK, never, stdin, failing, nil); code != 1 {
		t.Errorf("Mongo caído: código = %d", code)
	}
}
