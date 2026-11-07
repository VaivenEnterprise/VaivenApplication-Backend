package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// fixedWindow cuenta en una ventana fija de forma atómica: INCR y, si es la
// primera petición de la ventana, PEXPIRE. Devuelve el conteo actual.
var fixedWindow = goredis.NewScript(`
local n = redis.call("INCR", KEYS[1])
if n == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return n
`)

// Limiter implementa ratelimit.Limiter con Redis (ventana fija).
type Limiter struct {
	c *Client
}

// NewLimiter crea el limitador sobre un cliente de Redis.
func NewLimiter(c *Client) *Limiter {
	return &Limiter{c: c}
}

// Allow permite como máximo limit peticiones por ventana para key.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 || window <= 0 {
		return false, fmt.Errorf("redis: limiter: límite y ventana deben ser positivos")
	}
	n, err := fixedWindow.Run(ctx, l.c.rdb, []string{l.c.key("rl:" + key)}, window.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("redis: limiter: %w", err)
	}
	return n <= int64(limit), nil
}
