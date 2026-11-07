package security

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// testSecret es un valor de prueba de baja entropía (no es un secreto real).
const testSecret = "clave-de-prueba-clave-de-prueba-0000"

// legacyParams son los parámetros anteriores del plan P-01 (m=64 MiB, t=3,
// p=2): los hashes ya guardados con ellos deben seguir verificando.
var legacyParams = Params{MemoryKiB: 64 * 1024, Time: 3, Threads: 2}

func newTestHasher(t *testing.T, budgetKiB int64, wait time.Duration) *Hasher {
	t.Helper()
	h, err := NewHasher(budgetKiB, wait)
	if err != nil {
		t.Fatalf("NewHasher: %v", err)
	}
	return h
}

func TestHashAndVerify(t *testing.T) {
	h := newTestHasher(t, 0, 0)
	ctx := context.Background()

	hash, err := h.Hash(ctx, "una-contrasena-larga")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	phc := regexp.MustCompile(`^\$argon2id\$v=19\$m=19456,t=2,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)
	if !phc.MatchString(hash) {
		t.Fatalf("hash %q no tiene formato PHC con los parámetros de OWASP (P-01, A-07)", hash)
	}
	if strings.Contains(hash, "una-contrasena-larga") {
		t.Fatal("el hash contiene la contraseña")
	}

	other, err := h.Hash(ctx, "una-contrasena-larga")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if other == hash {
		t.Error("dos hashes de la misma contraseña son iguales: la sal no es aleatoria")
	}

	tests := []struct {
		name     string
		password string
		encoded  string
		want     error
		match    bool
	}{
		{"correcta", "una-contrasena-larga", hash, nil, true},
		{"incorrecta", "otra-contrasena", hash, nil, false},
		{"vacía", "", hash, nil, false},
		{"otro algoritmo", "x", "$argon2i$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash, false},
		{"otra versión", "x", "$argon2id$v=16$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash, false},
		{"sin partes", "x", "texto", ErrMalformedHash, false},
		{"parámetros ilegibles", "x", "$argon2id$v=19$m=a,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash, false},
		{"memoria excesiva", "x", "$argon2id$v=19$m=999999999,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash, false},
		{"tiempo cero", "x", "$argon2id$v=19$m=65536,t=0,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash, false},
		{"sal corta", "x", "$argon2id$v=19$m=65536,t=3,p=2$c2Fs$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash, false},
		{"sal inválida", "x", "$argon2id$v=19$m=65536,t=3,p=2$!!!!!!!!!!$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash, false},
		{"hash inválido", "x", "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$!!!!", ErrMalformedHash, false},
		{"hash de largo raro", "x", "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaA", ErrMalformedHash, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := h.Verify(ctx, tt.password, tt.encoded)
			if !errors.Is(err, tt.want) || ok != tt.match {
				t.Errorf("Verify = %v, %v; want %v, %v", ok, err, tt.match, tt.want)
			}
		})
	}
}

func TestVerifyAcceptsLegacyAndOtherParameters(t *testing.T) {
	ctx := context.Background()
	h := newTestHasher(t, 0, 0)
	// Hash antiguo de P-01 (m=64 MiB, t=3, p=2): sigue verificando aunque
	// supera el presupuesto de memoria (se calcula solo).
	legacy, err := newHasher(legacyParams, 64*1024, time.Second, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old, err := legacy.Hash(ctx, "pw-antigua")
	if err != nil || !strings.Contains(old, "$m=65536,t=3,p=2$") {
		t.Fatalf("hash antiguo = %q, %v", old, err)
	}
	if ok, err := h.Verify(ctx, "pw-antigua", old); err != nil || !ok {
		t.Errorf("Verify de un hash antiguo = %v, %v; want true", ok, err)
	}
	if ok, err := h.Verify(ctx, "otra", old); err != nil || ok {
		t.Errorf("Verify de un hash antiguo con otra contraseña = %v, %v; want false", ok, err)
	}
	// Parámetros más bajos y clave de 16 bytes.
	salt := []byte("sal-de-16-bytes!")
	key := argonKey(t, "pw", salt, 1, 1024, 1, 16)
	enc := "$argon2id$v=19$m=1024,t=1,p=1$" + b64.EncodeToString(salt) + "$" + b64.EncodeToString(key)
	if ok, err := h.Verify(ctx, "pw", enc); err != nil || !ok {
		t.Fatalf("Verify = %v, %v; want true, nil", ok, err)
	}
}

func TestDecoyHash(t *testing.T) {
	h := newTestHasher(t, 0, 0)
	decoy := h.DecoyHash()
	if !strings.HasPrefix(decoy, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("el señuelo no usa los parámetros actuales: %q", decoy)
	}
	if h.DecoyHash() != decoy {
		t.Error("el señuelo no debe cambiar")
	}
	for _, pw := range []string{"", "una-contrasena-larga", "admin"} {
		if ok, err := h.Verify(context.Background(), pw, decoy); err != nil || ok {
			t.Errorf("Verify(%q, señuelo) = %v, %v; want false, nil", pw, ok, err)
		}
	}
	if other := newTestHasher(t, 0, 0).DecoyHash(); other == decoy {
		t.Error("cada Hasher debe tener un señuelo distinto (valor aleatorio)")
	}
}

// failingReader simula que no hay fuente de aleatoriedad.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("sin entropía") }

func TestNewHasherFailsWithoutRandomness(t *testing.T) {
	if _, err := newHasher(DefaultParams, 0, 0, failingReader{}); err == nil {
		t.Fatal("sin aleatoriedad NewHasher debe fallar al arrancar")
	}
	h := newTestHasher(t, 0, 0)
	h.rand = failingReader{}
	if _, err := h.Hash(context.Background(), "x"); err == nil {
		t.Error("Hash sin aleatoriedad debe fallar")
	}
}

func TestHasherRespectsContextAndWait(t *testing.T) {
	h := newTestHasher(t, 0, 50*time.Millisecond)
	// Ocupa todo el presupuesto.
	if err := h.sem.Acquire(context.Background(), h.budgetKiB); err != nil {
		t.Fatal(err)
	}
	enc := h.DecoyHash()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Hash(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("Hash con contexto cancelado = %v, want context.Canceled", err)
	}
	if _, err := h.Verify(ctx, "x", enc); !errors.Is(err, context.Canceled) {
		t.Errorf("Verify con contexto cancelado = %v, want context.Canceled", err)
	}

	start := time.Now()
	_, err := h.Verify(context.Background(), "x", enc)
	var de *shared.Error
	if !errors.Is(err, ErrBusy) || !errors.As(err, &de) || de.Code != shared.CodeUnavailable {
		t.Errorf("sin turno = %v, want ErrBusy (service_unavailable)", err)
	}
	if waited := time.Since(start); waited < 40*time.Millisecond || waited > time.Second {
		t.Errorf("esperó %v, want ≈ maxWait (50 ms)", waited)
	}

	// Al liberar, vuelve a funcionar: una cancelación no deja estado.
	h.sem.Release(h.budgetKiB)
	if ok, err := h.Verify(context.Background(), "x", enc); err != nil || ok {
		t.Errorf("tras liberar = %v, %v; want false, nil", ok, err)
	}
}

func TestHasherBudgetLimitsConcurrency(t *testing.T) {
	// Presupuesto para dos hashes de 19 MiB: un tercero espera.
	h := newTestHasher(t, 2*int64(DefaultParams.MemoryKiB), 30*time.Millisecond)
	w1, err := h.acquire(context.Background(), int64(DefaultParams.MemoryKiB))
	if err != nil {
		t.Fatal(err)
	}
	w2, err := h.acquire(context.Background(), int64(DefaultParams.MemoryKiB))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.acquire(context.Background(), int64(DefaultParams.MemoryKiB)); !errors.Is(err, ErrBusy) {
		t.Errorf("tercer hash concurrente = %v, want ErrBusy", err)
	}
	h.sem.Release(w1)
	h.sem.Release(w2)
	// Un hash más grande que el presupuesto se limita al presupuesto (exclusivo).
	if w, err := h.acquire(context.Background(), 64*1024); err != nil || w != h.budgetKiB {
		t.Errorf("hash enorme: peso %d, %v; want %d", w, err, h.budgetKiB)
	} else {
		h.sem.Release(w)
	}
}

func TestHasherConcurrent(t *testing.T) {
	h := newTestHasher(t, 0, 5*time.Second)
	ctx := context.Background()
	hash, err := h.Hash(ctx, "pw")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pw := "pw"
			if i%2 == 1 {
				pw = "mala"
			}
			ok, err := h.Verify(ctx, pw, hash)
			if err != nil || ok != (i%2 == 0) {
				errs <- fmt.Errorf("goroutine %d: %v, %v", i, ok, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func BenchmarkHashDefault(b *testing.B) { benchHash(b, DefaultParams) }

func BenchmarkHashLegacy(b *testing.B) { benchHash(b, legacyParams) }

func benchHash(b *testing.B, p Params) {
	h, err := newHasher(p, int64(p.MemoryKiB), time.Minute, rand.Reader)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := h.Hash(context.Background(), "una-contrasena-larga"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestNewJWTValidation(t *testing.T) {
	if _, err := NewJWT(strings.Repeat("a", MinSecretLen-1), time.Minute); err == nil {
		t.Error("secreto corto aceptado")
	}
	if _, err := NewJWT(testSecret, 0); err == nil {
		t.Error("TTL cero aceptado")
	}
	j, err := NewJWT(testSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewJWT: %v", err)
	}
	if j.TTL() != 15*time.Minute {
		t.Errorf("TTL = %v", j.TTL())
	}
}

func TestJWTIssueAndParse(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	j, _ := NewJWT(testSecret, 15*time.Minute)
	j.now = func() time.Time { return now }

	tok, err := j.Issue("u1", "admin")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	c, err := j.Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.UserID != "u1" || c.Role != "admin" || !c.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Errorf("claims = %+v", c)
	}

	other, _ := NewJWT(testSecret+"-otra", 15*time.Minute)
	other.now = j.now
	noRole := sign(t, jwt.SigningMethodHS256, testSecret, jwt.MapClaims{"sub": "u1", "exp": now.Add(time.Minute).Unix()})
	noExp := sign(t, jwt.SigningMethodHS256, testSecret, jwt.MapClaims{"sub": "u1", "role": "user"})
	hs512 := sign(t, jwt.SigningMethodHS512, testSecret, jwt.MapClaims{"sub": "u1", "role": "user", "exp": now.Add(time.Minute).Unix()})
	future := sign(t, jwt.SigningMethodHS256, testSecret, jwt.MapClaims{"sub": "u1", "role": "user", "exp": now.Add(time.Hour).Unix(), "iat": now.Add(time.Hour).Unix()})
	// alg "none": cabecera y cuerpo sin firma.
	none := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u1","role":"admin","exp":4102444800}`)) + "."

	tests := []struct {
		name  string
		token string
		with  *JWT
		at    time.Time
	}{
		{"vencido", tok, j, now.Add(16 * time.Minute)},
		{"otro secreto", tok, other, now},
		{"alg none", none, j, now},
		{"HS512", hs512, j, now},
		{"sin rol", noRole, j, now},
		{"sin exp", noExp, j, now},
		{"emitido en el futuro", future, j, now},
		{"basura", "a.b.c", j, now},
		{"vacío", "", j, now},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := tt.at
			saved := tt.with.now
			tt.with.now = func() time.Time { return at }
			defer func() { tt.with.now = saved }()
			if _, err := tt.with.Parse(tt.token); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("Parse = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestRefreshToken(t *testing.T) {
	tok, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil || len(raw) != refreshTokenBytes {
		t.Fatalf("token %q no es base64url de %d bytes", tok, refreshTokenBytes)
	}
	if hash != HashToken(tok) || len(hash) != 64 || strings.Contains(hash, tok) {
		t.Errorf("hash = %q, no es el SHA-256 en hex del token", hash)
	}
	tok2, hash2, _ := NewRefreshToken()
	if tok2 == tok || hash2 == hash {
		t.Error("dos tokens iguales")
	}
	// Vector conocido de SHA-256.
	if got := HashToken("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("HashToken(abc) = %s", got)
	}
}

func sign(t *testing.T, m jwt.SigningMethod, secret string, c jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(m, c).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("firmar: %v", err)
	}
	return s
}

func argonKey(t *testing.T, pw string, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte {
	t.Helper()
	return argon2.IDKey([]byte(pw), salt, time, memory, threads, keyLen)
}
