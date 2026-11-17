// Package ratelimit declara el puerto de límite de tasa (Art. XIV.2). Lo
// consumen el middleware HTTP y, más adelante, los casos de uso (POL-11, intentos
// de inicio de sesión). Las implementaciones viven en internal/platform/redis.
package ratelimit

import (
	"context"
	"time"
)

// Limiter decide si una acción identificada por key puede ocurrir otra vez:
// permite como máximo limit veces por ventana. Debe ser seguro para uso
// concurrente y no guardar estado de negocio (Redis nunca es fuente de verdad).
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}
