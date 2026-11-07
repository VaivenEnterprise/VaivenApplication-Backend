// Package security reúne las primitivas criptográficas de la autenticación
// (spec 003, T003; plan P-01 a P-03; Art. VI.3): contraseñas con argon2id,
// tokens de acceso JWT HS256 y refresh tokens opacos. No registra ni devuelve
// en errores contraseñas, tokens ni hashes (RNF-01).
package security

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parámetros de argon2id del plan P-01.
const (
	argonMemoryKiB = 64 * 1024 // 64 MiB
	argonTime      = 3
	argonThreads   = 2
	argonSaltLen   = 16
	argonKeyLen    = 32

	// Cotas al leer un hash guardado: evitan que un valor corrupto pida
	// memoria o tiempo desmedidos.
	maxMemoryKiB = 256 * 1024
	maxTime      = 10
)

// ErrMismatch indica que la contraseña no coincide con el hash.
var ErrMismatch = errors.New("security: la contraseña no coincide")

// ErrMalformedHash indica que el hash guardado no tiene formato PHC válido.
var ErrMalformedHash = errors.New("security: hash de contraseña con formato inválido")

var b64 = base64.RawStdEncoding

// Hasher calcula y verifica hashes argon2id en formato PHC:
//
//	$argon2id$v=19$m=65536,t=3,p=2$<sal>$<hash>
//
// Cada hash reserva 64 MiB; para no agotar la memoria del contenedor (160 MiB
// en la VM, Art. XIV.5) solo se calculan maxConcurrent a la vez y las demás
// peticiones esperan su turno (o a que se cancele su contexto).
type Hasher struct {
	slots chan struct{}
}

// NewHasher crea un Hasher que calcula como máximo maxConcurrent hashes a la
// vez (mínimo 1).
func NewHasher(maxConcurrent int) *Hasher {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Hasher{slots: make(chan struct{}, maxConcurrent)}
}

func (h *Hasher) acquire(ctx context.Context) error {
	select {
	case h.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("security: esperando turno para el hash: %w", ctx.Err())
	}
}

func (h *Hasher) release() { <-h.slots }

// Hash devuelve el hash PHC de password con una sal aleatoria nueva.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("security: generar sal: %w", err)
	}
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	h.release()
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Verify compara password con el hash PHC en tiempo constante. Devuelve nil si
// coinciden, ErrMismatch si no y ErrMalformedHash si el hash es inválido. Usa
// los parámetros guardados en el hash, así que admite hashes hechos con
// parámetros anteriores.
func (h *Hasher) Verify(ctx context.Context, password, encoded string) error {
	p, err := parsePHC(encoded)
	if err != nil {
		return err
	}
	if err := h.acquire(ctx); err != nil {
		return err
	}
	key := argon2.IDKey([]byte(password), p.salt, p.time, p.memory, p.threads, p.keyLen)
	h.release()
	if subtle.ConstantTimeCompare(key, p.key) != 1 {
		return ErrMismatch
	}
	return nil
}

type phcParams struct {
	memory  uint32
	time    uint32
	threads uint8
	keyLen  uint32
	salt    []byte
	key     []byte
}

// parsePHC lee un hash $argon2id$v=19$m=..,t=..,p=..$<sal>$<hash>.
func parsePHC(encoded string) (phcParams, error) {
	var p phcParams
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return p, ErrMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return p, ErrMalformedHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return p, ErrMalformedHash
	}
	if p.memory == 0 || p.memory > maxMemoryKiB || p.time == 0 || p.time > maxTime || p.threads == 0 {
		return p, ErrMalformedHash
	}
	var err error
	if p.salt, err = b64.DecodeString(parts[4]); err != nil || len(p.salt) < 8 {
		return p, ErrMalformedHash
	}
	if p.key, err = b64.DecodeString(parts[5]); err != nil {
		return p, ErrMalformedHash
	}
	switch len(p.key) {
	case 16:
		p.keyLen = 16
	case 32:
		p.keyLen = 32
	case 64:
		p.keyLen = 64
	default:
		return p, ErrMalformedHash
	}
	return p, nil
}
