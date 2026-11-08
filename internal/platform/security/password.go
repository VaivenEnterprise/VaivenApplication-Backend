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
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/sync/semaphore"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// Params son los parámetros de argon2id.
type Params struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
}

// DefaultParams devuelve los parámetros de los hashes nuevos (plan P-01, enmienda
// A-07 de la spec 003): la configuración mínima recomendada por OWASP para
// argon2id (19 MiB, 2 iteraciones, paralelismo 1). Medido con los límites de
// la VM (2 vCPU, 160 MiB): ~50 ms y ~19 MiB por hash, frente a ~140 ms y
// 64 MiB de los parámetros anteriores (m=64 MiB, t=3, p=2), que dejaban el
// proceso en ~131 MiB de 160 con un solo hash.
// Es una función (no una variable) para que nadie pueda cambiarlos en tiempo
// de ejecución (constitución §5: sin variables globales mutables).
func DefaultParams() Params {
	return Params{MemoryKiB: defaultMemoryKiB, Time: defaultTime, Threads: defaultThreads}
}

const (
	defaultMemoryKiB = 19 * 1024
	defaultTime      = 2
	defaultThreads   = 1
)

const (
	argonSaltLen = 16
	argonKeyLen  = 32

	// Cotas al leer un hash guardado: evitan que un valor corrupto pida
	// memoria o tiempo desmedidos.
	maxMemoryKiB = 256 * 1024
	maxTime      = 10

	// DefaultMemoryBudgetKiB es la memoria máxima que pueden usar a la vez
	// los hashes en curso: 2 hashes con DefaultParams(). Un hash más grande que
	// el presupuesto (p. ej. uno antiguo de 64 MiB) se calcula solo.
	DefaultMemoryBudgetKiB = 40 * 1024
	// DefaultMaxWait es lo más que una petición espera su turno; después
	// recibe ErrBusy (503) en lugar de acumularse.
	DefaultMaxWait = 2 * time.Second
)

// ErrMalformedHash indica que el hash guardado no tiene formato PHC válido.
var ErrMalformedHash = errors.New("security: hash de contraseña con formato inválido")

// ErrBusy indica que no hubo turno para calcular un hash dentro de la espera
// máxima. Es un error de dominio service_unavailable (503): la petición se
// puede reintentar y no se acumula consumiendo memoria.
var ErrBusy = shared.NewUnavailable("Hay muchas solicitudes en este momento. Intenta de nuevo en unos segundos.")

var b64 = base64.RawStdEncoding

// Hasher calcula y verifica hashes argon2id en formato PHC:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<sal>$<hash>
//
// Para no agotar la memoria del contenedor (160 MiB en la VM, Art. XIV.5) los
// hashes en curso comparten un presupuesto de memoria; quien no consigue turno
// en maxWait recibe ErrBusy. Es seguro para uso concurrente.
type Hasher struct {
	params    Params
	budgetKiB int64
	sem       *semaphore.Weighted
	maxWait   time.Duration
	rand      io.Reader
	decoy     string
}

// NewHasher crea un Hasher con DefaultParams(), el presupuesto de memoria
// indicado (KiB; ≤ 0 usa DefaultMemoryBudgetKiB) y la espera máxima (≤ 0 usa
// DefaultMaxWait). Calcula al crearse el hash señuelo (ver DecoyHash), fuera de
// cualquier petición; falla solo si no hay fuente de aleatoriedad.
func NewHasher(budgetKiB int64, maxWait time.Duration) (*Hasher, error) {
	return newHasher(DefaultParams(), budgetKiB, maxWait, rand.Reader)
}

func newHasher(p Params, budgetKiB int64, maxWait time.Duration, random io.Reader) (*Hasher, error) {
	if budgetKiB <= 0 {
		budgetKiB = DefaultMemoryBudgetKiB
	}
	if maxWait <= 0 {
		maxWait = DefaultMaxWait
	}
	h := &Hasher{
		params:    p,
		budgetKiB: budgetKiB,
		sem:       semaphore.NewWeighted(budgetKiB),
		maxWait:   maxWait,
		rand:      random,
	}
	// El señuelo es el hash de un valor aleatorio que nadie conoce: verificar
	// contra él cuesta lo mismo que contra un hash real y nunca coincide.
	secret := make([]byte, 32)
	if _, err := io.ReadFull(random, secret); err != nil {
		return nil, fmt.Errorf("security: generar el señuelo: %w", err)
	}
	decoy, err := h.hash(context.Background(), string(secret))
	if err != nil {
		return nil, fmt.Errorf("security: calcular el señuelo: %w", err)
	}
	h.decoy = decoy
	return h, nil
}

// DecoyHash devuelve un hash válido, con los parámetros actuales, de un valor
// que nadie conoce. El login verifica contra él cuando el correo no existe,
// para que el tiempo de respuesta sea el mismo que con una cuenta real
// (RF-04). No cambia durante la vida del Hasher.
func (h *Hasher) DecoyHash() string { return h.decoy }

// acquire reserva memKiB del presupuesto. Si el contexto de la petición se
// cancela devuelve su error; si se agota maxWait, ErrBusy.
func (h *Hasher) acquire(ctx context.Context, memKiB int64) (int64, error) {
	weight := min(memKiB, h.budgetKiB) // un hash enorme se calcula solo
	wctx, cancel := context.WithTimeout(ctx, h.maxWait)
	defer cancel()
	if err := h.sem.Acquire(wctx, weight); err != nil {
		if ctx.Err() != nil {
			return 0, fmt.Errorf("security: esperando turno para el hash: %w", ctx.Err())
		}
		return 0, ErrBusy
	}
	return weight, nil
}

// Hash devuelve el hash PHC de password con una sal aleatoria nueva.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	return h.hash(ctx, password)
}

func (h *Hasher) hash(ctx context.Context, password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := io.ReadFull(h.rand, salt); err != nil {
		return "", fmt.Errorf("security: generar sal: %w", err)
	}
	w, err := h.acquire(ctx, int64(h.params.MemoryKiB))
	if err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, h.params.Time, h.params.MemoryKiB, h.params.Threads, argonKeyLen)
	h.sem.Release(w)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.params.MemoryKiB, h.params.Time, h.params.Threads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Verify compara password con el hash PHC en tiempo constante: (true, nil) si
// coinciden, (false, nil) si no y un error si el hash es inválido
// (ErrMalformedHash), se canceló el contexto o no hubo turno (ErrBusy). Usa
// los parámetros guardados en el hash, así que admite hashes hechos con
// parámetros anteriores (p. ej. m=64 MiB, t=3, p=2).
func (h *Hasher) Verify(ctx context.Context, password, encoded string) (bool, error) {
	p, err := parsePHC(encoded)
	if err != nil {
		return false, err
	}
	w, err := h.acquire(ctx, int64(p.memory))
	if err != nil {
		return false, err
	}
	key := argon2.IDKey([]byte(password), p.salt, p.time, p.memory, p.threads, p.keyLen)
	h.sem.Release(w)
	return subtle.ConstantTimeCompare(key, p.key) == 1, nil
}

// NeedsRehash indica si encoded se calculó con parámetros distintos de los
// actuales (p. ej. un hash antiguo de m=64 MiB). El login lo recalcula tras
// verificar la contraseña: así las cuentas antiguas migran solas y desaparece
// la diferencia de tiempo frente al señuelo (auditoría 2026-11-07, N2). Un hash
// mal formado devuelve false: no se puede verificar, así que no se migra.
func (h *Hasher) NeedsRehash(encoded string) bool {
	p, err := parsePHC(encoded)
	if err != nil {
		return false
	}
	return p.memory != h.params.MemoryKiB || p.time != h.params.Time ||
		p.threads != h.params.Threads || p.keyLen != argonKeyLen
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
