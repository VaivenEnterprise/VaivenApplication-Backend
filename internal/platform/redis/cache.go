package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Cache implementa cache.Cache con Redis.
type Cache struct {
	c *Client
}

// NewCache crea la caché sobre un cliente de Redis.
func NewCache(c *Client) *Cache {
	return &Cache{c: c}
}

// Get devuelve el valor y true si existe.
func (k *Cache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	v, err := k.c.rdb.Get(ctx, k.c.key("cache:"+key)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("redis: cache get: %w", err)
	}
	return v, true, nil
}

// Set guarda value durante ttl (debe ser positivo: la caché siempre expira).
func (k *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("redis: cache set: el TTL debe ser positivo")
	}
	if err := k.c.rdb.Set(ctx, k.c.key("cache:"+key), value, ttl).Err(); err != nil {
		return fmt.Errorf("redis: cache set: %w", err)
	}
	return nil
}

// Delete borra la clave; no es error si no existía.
func (k *Cache) Delete(ctx context.Context, key string) error {
	if err := k.c.rdb.Del(ctx, k.c.key("cache:"+key)).Err(); err != nil {
		return fmt.Errorf("redis: cache delete: %w", err)
	}
	return nil
}
