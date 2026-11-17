package redis

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryLimiter implementa ratelimit.Limiter en memoria (ventana fija). Sirve
// para pruebas y para el modo degradado cuando Redis falla: el límite pasa a
// ser por réplica, no global (Art. XIV.2).
type MemoryLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	windows map[string]memWindow
}

type memWindow struct {
	count int
	ends  time.Time
}

// NewMemoryLimiter crea un limitador en memoria.
func NewMemoryLimiter() *MemoryLimiter {
	return &MemoryLimiter{now: time.Now, windows: map[string]memWindow{}}
}

// Allow permite como máximo limit peticiones por ventana para key.
func (m *MemoryLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 || window <= 0 {
		return false, fmt.Errorf("memoria: limiter: límite y ventana deben ser positivos")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	w := m.windows[key]
	if !now.Before(w.ends) {
		w = memWindow{ends: now.Add(window)}
		m.sweep(now)
	}
	w.count++
	m.windows[key] = w
	return w.count <= limit, nil
}

// sweep borra ventanas vencidas para que el mapa no crezca sin límite.
func (m *MemoryLimiter) sweep(now time.Time) {
	for k, w := range m.windows {
		if !now.Before(w.ends) {
			delete(m.windows, k)
		}
	}
}

// MemoryCache implementa cache.Cache en memoria.
type MemoryCache struct {
	mu    sync.Mutex
	now   func() time.Time
	items map[string]memItem
}

type memItem struct {
	value   []byte
	expires time.Time
}

// NewMemoryCache crea una caché en memoria.
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{now: time.Now, items: map[string]memItem{}}
}

// Get devuelve una copia del valor y true si existe y no expiró.
func (m *MemoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[key]
	if !ok || !m.now().Before(it.expires) {
		delete(m.items, key)
		return nil, false, nil
	}
	return append([]byte(nil), it.value...), true, nil
}

// Set guarda una copia de value durante ttl.
func (m *MemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("memoria: cache set: el TTL debe ser positivo")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = memItem{value: append([]byte(nil), value...), expires: m.now().Add(ttl)}
	return nil
}

// Delete borra la clave.
func (m *MemoryCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
	return nil
}
