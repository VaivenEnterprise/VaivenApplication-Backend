package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
)

// Pruebas del canal de enumeración por tiempo del login (auditoría 2026-11-07,
// hallazgo 1): el señuelo ya no depende de la petición ni puede quedar en un
// estado de error.

func TestNewServiceRequiresDecoy(t *testing.T) { // error de inicialización
	_, err := NewService(Deps{
		Users: newFakeUsers(), Refresh: newFakeRefresh(), Hasher: &fakeHasher{noDecoy: true},
		Access: fakeAccess{}, NewRefresh: func() (string, string, error) { return "", "", nil },
		HashRefresh: func(s string) string { return s }, RefreshTTL: time.Hour,
	})
	if err == nil {
		t.Fatal("sin hash señuelo NewService debe fallar al arrancar")
	}
}

func TestLoginVerifiesAgainstDecoyForUnknownEmail(t *testing.T) {
	e := newEnv(t)
	reg := e.mustRegister(t)
	ctx := context.Background()

	// Cuenta existente con contraseña incorrecta y correo inexistente: un
	// Verify cada uno, el mismo error genérico.
	_, errExisting := e.svc.Login(ctx, LoginInput{Email: "ana@example.com", Password: "mala"})
	_, errUnknown := e.svc.Login(ctx, LoginInput{Email: "nadie@example.com", Password: "mala"})
	if !domain.Is(errExisting, domain.ErrInvalidCredentials) || !domain.Is(errUnknown, domain.ErrInvalidCredentials) {
		t.Fatalf("errores = %v / %v", errExisting, errUnknown)
	}
	if errExisting.Error() != errUnknown.Error() {
		t.Errorf("respuestas distintas: %q vs %q", errExisting, errUnknown)
	}
	if e.hasher.verifies != 2 || e.hasher.encodings[0] != reg.User.PasswordHash || e.hasher.encodings[1] != fakeDecoy {
		t.Errorf("verificaciones = %d contra %v", e.hasher.verifies, e.hasher.encodings)
	}
	// Aunque alguien lograra que el señuelo coincida, nunca autentica.
	if ok, err := e.svc.checkPassword(ctx, nil, "decoy"); ok || err != nil {
		t.Errorf("checkPassword(nil) = %v, %v; want false", ok, err)
	}
}

func TestLoginCancelledFirstRequestDoesNotPoison(t *testing.T) { // primera cancelada + recuperación
	e := newEnv(t)
	e.mustRegister(t)

	// Primera petición de la vida del servicio, cancelada: correo inexistente
	// y cuenta existente fallan igual (el error del contexto, no 401).
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, errUnknown := e.svc.Login(cancelled, LoginInput{Email: "nadie@example.com", Password: "x"})
	_, errExisting := e.svc.Login(cancelled, LoginInput{Email: "ana@example.com", Password: "x"})
	if !errors.Is(errUnknown, context.Canceled) || !errors.Is(errExisting, context.Canceled) {
		t.Fatalf("cancelada: %v / %v, want context.Canceled en ambos", errUnknown, errExisting)
	}

	// Recuperación: las siguientes peticiones verifican contra el señuelo como
	// siempre; la cancelación no dejó ningún estado.
	before := e.hasher.verifies
	for i := 0; i < 3; i++ {
		_, err := e.svc.Login(context.Background(), LoginInput{Email: fmt.Sprintf("nadie%d@example.com", i), Password: "x"})
		if !domain.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("tras la cancelación: %v", err)
		}
	}
	if e.hasher.verifies != before+3 {
		t.Errorf("tras la cancelación se verificó %d veces, want 3", e.hasher.verifies-before)
	}
	for _, enc := range e.hasher.encodings[len(e.hasher.encodings)-3:] {
		if enc != fakeDecoy {
			t.Errorf("verificó contra %q, want el señuelo", enc)
		}
	}
}

func TestLoginConcurrentKnownAndUnknown(t *testing.T) { // solicitudes concurrentes
	e := newEnv(t)
	e.mustRegister(t)
	const n = 40
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			email := "ana@example.com"
			if i%2 == 1 {
				email = fmt.Sprintf("nadie%d@example.com", i)
			}
			_, err := e.svc.Login(context.Background(), LoginInput{Email: email, Password: "mala"})
			if !domain.Is(err, domain.ErrInvalidCredentials) {
				errs <- fmt.Errorf("petición %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if e.hasher.verifies != n {
		t.Errorf("verificaciones = %d, want %d (una por petición)", e.hasher.verifies, n)
	}
}

// TestLoginTimingWithRealHasher compara, con argon2id real, el tiempo del login
// con un correo existente (contraseña mala) y con uno inexistente, también
// después de una primera petición cancelada (el escenario del hallazgo).
func TestLoginTimingWithRealHasher(t *testing.T) {
	if testing.Short() {
		t.Skip("argon2id real")
	}
	h, err := security.NewHasher(0, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, func(d *Deps) { d.Hasher = h })
	ctx := context.Background()
	if _, err := e.svc.Register(ctx, validRegister()); err != nil {
		t.Fatal(err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, _ = e.svc.Login(cancelled, LoginInput{Email: "nadie@example.com", Password: "x"})

	median := func(email string) time.Duration {
		var ds []time.Duration
		for i := 0; i < 7; i++ {
			start := time.Now()
			_, err := e.svc.Login(ctx, LoginInput{Email: email, Password: "contrasena-mala"})
			ds = append(ds, time.Since(start))
			if !domain.Is(err, domain.ErrInvalidCredentials) {
				t.Fatalf("login %s: %v", email, err)
			}
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		return ds[len(ds)/2]
	}
	existing, unknown := median("ana@example.com"), median("nadie@example.com")
	ratio := float64(unknown) / float64(existing)
	t.Logf("mediana: existente %v, inexistente %v (razón %.2f)", existing, unknown, ratio)
	if ratio < 0.5 || ratio > 2 {
		t.Errorf("el tiempo revela si el correo existe: existente %v, inexistente %v", existing, unknown)
	}
	if unknown < 5*time.Millisecond {
		t.Errorf("el correo inexistente no ejecutó argon2id (%v)", unknown)
	}
}
