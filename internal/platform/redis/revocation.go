package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Claves (con el prefijo vaiven:<APP_ENV>: del cliente):
//
//	revoke:jti:<jti>   "1", vence con el token
//	revoke:user:<id>   corte en segundos Unix, vence tras la vida del token
//
// Nunca se guarda el token ni ningún dato del usuario además de su id. Toda
// clave tiene TTL, así que el espacio no crece sin límite.
const (
	revokeJTIPrefix  = "revoke:jti:"
	revokeUserPrefix = "revoke:user:"
)

// raiseCutoff sube el corte solo si el nuevo es mayor y renueva el TTL.
var raiseCutoff = goredis.NewScript(`
local cur = tonumber(redis.call('GET', KEYS[1]) or '0')
local new = tonumber(ARGV[1])
if new > cur then
  redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
else
  redis.call('PEXPIRE', KEYS[1], ARGV[2], 'GT')
end
return 1`)

// Revocations implementa revocation.Store con Redis.
type Revocations struct {
	c   *Client
	now func() time.Time
}

// NewRevocations crea el almacén sobre un cliente de Redis.
func NewRevocations(c *Client) *Revocations {
	return &Revocations{c: c, now: time.Now}
}

// RevokeToken guarda el jti hasta que el token vence.
func (r *Revocations) RevokeToken(ctx context.Context, jti string, expiresAt time.Time) error {
	if jti == "" {
		return errors.New("redis: revoke: jti vacío")
	}
	ttl := expiresAt.Sub(r.now())
	if ttl <= 0 {
		return nil // ya venció: no hace falta recordarlo
	}
	if err := r.c.rdb.Set(ctx, r.c.key(revokeJTIPrefix+jti), "1", ttl).Err(); err != nil {
		return fmt.Errorf("redis: revoke token: %w", err)
	}
	return nil
}

// RevokeUserBefore sube el corte del usuario.
func (r *Revocations) RevokeUserBefore(ctx context.Context, userID string, cutoff time.Time, ttl time.Duration) error {
	if userID == "" || ttl <= 0 {
		return errors.New("redis: revoke user: usuario vacío o TTL no positivo")
	}
	err := raiseCutoff.Run(ctx, r.c.rdb, []string{r.c.key(revokeUserPrefix + userID)},
		strconv.FormatInt(cutoff.Unix(), 10), ttl.Milliseconds()).Err()
	if err != nil {
		return fmt.Errorf("redis: revoke user: %w", err)
	}
	return nil
}

// IsRevoked consulta las dos claves en una sola ida y vuelta (MGET).
func (r *Revocations) IsRevoked(ctx context.Context, userID, jti string, issuedAt time.Time) (bool, error) {
	vals, err := r.c.rdb.MGet(ctx, r.c.key(revokeJTIPrefix+jti), r.c.key(revokeUserPrefix+userID)).Result()
	if err != nil {
		return false, fmt.Errorf("redis: is revoked: %w", err)
	}
	return revokedFrom(vals[0] != nil, vals[1], issuedAt)
}

// revokedFrom decide con lo leído: jti revocado, o emitido en o antes del corte.
func revokedFrom(jtiRevoked bool, cutoff any, issuedAt time.Time) (bool, error) {
	if jtiRevoked {
		return true, nil
	}
	if cutoff == nil {
		return false, nil
	}
	s, ok := cutoff.(string)
	if !ok {
		return false, errors.New("redis: is revoked: corte con tipo inesperado")
	}
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return false, fmt.Errorf("redis: is revoked: corte inválido: %w", err)
	}
	return issuedAt.Unix() <= sec, nil
}

// MemoryRevocations implementa revocation.Store en memoria (pruebas y
// desarrollo sin Redis; nunca con varias réplicas).
type MemoryRevocations struct {
	mu   sync.Mutex
	jti  map[string]time.Time
	user map[string]memCutoff
	now  func() time.Time
}

type memCutoff struct {
	at      int64
	expires time.Time
}

// NewMemoryRevocations crea el almacén en memoria.
func NewMemoryRevocations() *MemoryRevocations {
	return &MemoryRevocations{jti: map[string]time.Time{}, user: map[string]memCutoff{}, now: time.Now}
}

// RevokeToken guarda el jti hasta expiresAt.
func (m *MemoryRevocations) RevokeToken(_ context.Context, jti string, expiresAt time.Time) error {
	if jti == "" {
		return errors.New("memoria: revoke: jti vacío")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if expiresAt.After(m.now()) {
		m.jti[jti] = expiresAt
	}
	return nil
}

// RevokeUserBefore sube el corte del usuario.
func (m *MemoryRevocations) RevokeUserBefore(_ context.Context, userID string, cutoff time.Time, ttl time.Duration) error {
	if userID == "" || ttl <= 0 {
		return errors.New("memoria: revoke user: usuario vacío o TTL no positivo")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.user[userID]
	exp := m.now().Add(ttl)
	if !ok || cur.expires.Before(m.now()) || cutoff.Unix() > cur.at {
		m.user[userID] = memCutoff{at: cutoff.Unix(), expires: exp}
	} else if exp.After(cur.expires) {
		cur.expires = exp
		m.user[userID] = cur
	}
	return nil
}

// IsRevoked consulta y limpia lo vencido.
func (m *MemoryRevocations) IsRevoked(_ context.Context, userID, jti string, issuedAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if exp, ok := m.jti[jti]; ok {
		if exp.After(now) {
			return true, nil
		}
		delete(m.jti, jti)
	}
	if c, ok := m.user[userID]; ok {
		if c.expires.After(now) {
			return issuedAt.Unix() <= c.at, nil
		}
		delete(m.user, userID)
	}
	return false, nil
}
