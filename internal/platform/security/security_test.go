package security

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

// testSecret es un valor de prueba de baja entropía (no es un secreto real).
const testSecret = "clave-de-prueba-clave-de-prueba-0000"

func TestHashAndVerify(t *testing.T) {
	h := NewHasher(1)
	ctx := context.Background()

	hash, err := h.Hash(ctx, "una-contrasena-larga")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	phc := regexp.MustCompile(`^\$argon2id\$v=19\$m=65536,t=3,p=2\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)
	if !phc.MatchString(hash) {
		t.Fatalf("hash %q no tiene formato PHC con los parámetros del plan P-01", hash)
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
	}{
		{"correcta", "una-contrasena-larga", hash, nil},
		{"incorrecta", "otra-contrasena", hash, ErrMismatch},
		{"vacía", "", hash, ErrMismatch},
		{"otro algoritmo", "x", "$argon2i$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash},
		{"otra versión", "x", "$argon2id$v=16$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash},
		{"sin partes", "x", "texto", ErrMalformedHash},
		{"parámetros ilegibles", "x", "$argon2id$v=19$m=a,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash},
		{"memoria excesiva", "x", "$argon2id$v=19$m=999999999,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash},
		{"tiempo cero", "x", "$argon2id$v=19$m=65536,t=0,p=2$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash},
		{"sal corta", "x", "$argon2id$v=19$m=65536,t=3,p=2$c2Fs$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash},
		{"sal inválida", "x", "$argon2id$v=19$m=65536,t=3,p=2$!!!!!!!!!!$aGFzaGhhc2hoYXNoaGFzaA", ErrMalformedHash},
		{"hash inválido", "x", "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$!!!!", ErrMalformedHash},
		{"hash de largo raro", "x", "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaA", ErrMalformedHash},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := h.Verify(ctx, tt.password, tt.encoded); !errors.Is(err, tt.want) {
				t.Errorf("Verify = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestVerifyAcceptsOtherParameters(t *testing.T) {
	// Un hash con parámetros más bajos (y clave de 16 bytes) sigue verificando.
	h := NewHasher(0) // se corrige a 1
	salt := []byte("sal-de-16-bytes!")
	key := argonKey(t, "pw", salt, 1, 1024, 1, 16)
	enc := "$argon2id$v=19$m=1024,t=1,p=1$" + b64.EncodeToString(salt) + "$" + b64.EncodeToString(key)
	if err := h.Verify(context.Background(), "pw", enc); err != nil {
		t.Fatalf("Verify = %v, want nil", err)
	}
}

func TestHasherRespectsContext(t *testing.T) {
	h := NewHasher(1)
	h.slots <- struct{}{} // ocupa el único turno
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Hash(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("Hash con contexto cancelado = %v, want context.Canceled", err)
	}
	enc := "$argon2id$v=19$m=1024,t=1,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA"
	if err := h.Verify(ctx, "x", enc); !errors.Is(err, context.Canceled) {
		t.Errorf("Verify con contexto cancelado = %v, want context.Canceled", err)
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
