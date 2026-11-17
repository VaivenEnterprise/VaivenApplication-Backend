// Package cache declara el puerto de caché de lecturas con TTL corto
// (Art. XIV.2: categorías, resultados de búsqueda). Las implementaciones viven
// en internal/platform/redis. La caché nunca es fuente de verdad: si falla,
// quien la usa lee de MongoDB.
package cache

import (
	"context"
	"time"
)

// Cache guarda valores opacos por clave durante ttl.
type Cache interface {
	// Get devuelve el valor y true si existe y no expiró.
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}
