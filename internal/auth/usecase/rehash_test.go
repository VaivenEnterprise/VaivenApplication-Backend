package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
)

// Pruebas de la migración de hashes antiguos al iniciar sesión (auditoría
// 2026-11-07, hallazgo N2).

// withLegacyHash registra a Ana y le pone un hash "antiguo".
func withLegacyHash(t *testing.T, e *env) string {
	t.Helper()
	reg := e.mustRegister(t)
	u := e.users.byID[reg.User.ID]
	u.PasswordHash = "legacy:una-contrasena-larga"
	e.users.byID[u.ID] = u
	return u.ID
}

func TestLoginUpgradesLegacyHash(t *testing.T) {
	e := newEnv(t)
	id := withLegacyHash(t, e)
	before := e.users.byID[id].Version

	res, err := e.svc.Login(context.Background(), LoginInput{Email: "ana@example.com", Password: "una-contrasena-larga"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	u := e.users.byID[id]
	if u.PasswordHash != "hash:una-contrasena-larga" || u.Version != before+1 {
		t.Errorf("hash tras el login = %q (versión %d), want migrado", u.PasswordHash, u.Version)
	}
	if res.User.PasswordHash != u.PasswordHash {
		t.Error("el usuario devuelto debe llevar el hash nuevo")
	}
	// Un segundo login ya no migra (el hash es actual).
	if _, err := e.svc.Login(context.Background(), LoginInput{Email: "ana@example.com", Password: "una-contrasena-larga"}); err != nil {
		t.Fatal(err)
	}
	if e.users.byID[id].Version != before+1 {
		t.Error("un hash actual no debe volver a migrarse")
	}
}

func TestLoginDoesNotUpgradeOnFailure(t *testing.T) {
	e := newEnv(t)
	id := withLegacyHash(t, e)
	e.hasher.failHash = errBoom // si se intentara migrar, Hash fallaría
	if _, err := e.svc.Login(context.Background(), LoginInput{Email: "ana@example.com", Password: "mala"}); !domain.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("contraseña mala = %v", err)
	}
	if e.users.byID[id].PasswordHash != "legacy:una-contrasena-larga" {
		t.Error("un intento fallido no debe migrar el hash")
	}
}

func TestLoginSucceedsWhenUpgradeFails(t *testing.T) {
	for name, breakIt := range map[string]func(*env){
		"Hash falla":   func(e *env) { e.hasher.failHash = errBoom },
		"Update falla": func(e *env) { e.users.failUpdateHash = errBoom },
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			id := withLegacyHash(t, e)
			breakIt(e)
			if _, err := e.svc.Login(context.Background(), LoginInput{Email: "ana@example.com", Password: "una-contrasena-larga"}); err != nil {
				t.Fatalf("el login debe funcionar aunque la migración falle: %v", err)
			}
			if e.users.byID[id].PasswordHash != "legacy:una-contrasena-larga" {
				t.Error("si la migración falla, el hash anterior se conserva")
			}
		})
	}
}

// legacyPHC arma un hash argon2id con los parámetros anteriores de P-01
// (m=64 MiB, t=3, p=2).
func legacyPHC(t *testing.T, password string) string {
	t.Helper()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	b := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", b.EncodeToString(salt), b.EncodeToString(key))
}

// TestLegacyHashMigratesAndTimingMatches: con argon2id real, una cuenta con
// hash antiguo tarda más que el señuelo hasta su primer login; después se
// migra y el tiempo de los intentos fallidos queda igual al de un correo
// inexistente.
func TestLegacyHashMigratesAndTimingMatches(t *testing.T) {
	if testing.Short() {
		t.Skip("argon2id real")
	}
	h, err := security.NewHasher(0, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, func(d *Deps) { d.Hasher = h })
	ctx := context.Background()
	reg, err := e.svc.Register(ctx, validRegister())
	if err != nil {
		t.Fatal(err)
	}
	u := e.users.byID[reg.User.ID]
	u.PasswordHash = legacyPHC(t, "una-contrasena-larga")
	e.users.byID[u.ID] = u

	if _, err := e.svc.Login(ctx, LoginInput{Email: "ana@example.com", Password: "una-contrasena-larga"}); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if got := e.users.byID[u.ID].PasswordHash; !strings.HasPrefix(got, "$argon2id$v=19$m=19456,t=2,p=1$") || h.NeedsRehash(got) {
		t.Fatalf("el hash no se migró: %q", got)
	}
	if _, err := e.svc.Login(ctx, LoginInput{Email: "ana@example.com", Password: "una-contrasena-larga"}); err != nil {
		t.Fatalf("login con el hash migrado: %v", err)
	}

	median := func(email string) time.Duration {
		var ds []time.Duration
		for i := 0; i < 7; i++ {
			start := time.Now()
			_, _ = e.svc.Login(ctx, LoginInput{Email: email, Password: "contrasena-mala"})
			ds = append(ds, time.Since(start))
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		return ds[len(ds)/2]
	}
	migrated, unknown := median("ana@example.com"), median("nadie@example.com")
	ratio := float64(unknown) / float64(migrated)
	t.Logf("mediana tras migrar: cuenta %v, inexistente %v (razón %.2f)", migrated, unknown, ratio)
	if ratio < 0.5 || ratio > 2 {
		t.Errorf("tras migrar, el tiempo aún revela la cuenta: %v vs %v", migrated, unknown)
	}
}
