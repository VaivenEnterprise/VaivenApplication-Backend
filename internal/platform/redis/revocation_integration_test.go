//go:build integration

package redis

import (
	"context"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

// revocationRedis permite probar el almacén en la red interna del laboratorio
// cuando Testcontainers no puede publicar sus puertos en Windows.
func revocationRedis(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("VAIVEN_LAB_REVOCATION") != "1" {
		return startRedis(t)
	}
	c, err := NewClient("redis://lab-redis:6379/0", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// newReplica simula otra instancia de la API: otro cliente, mismo Redis.
func newReplica(t *testing.T, url string) *Revocations {
	t.Helper()
	c, err := NewClient(url, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return NewRevocations(c)
}

func TestIntegrationRevocationSharedAcrossReplicas(t *testing.T) {
	ctx := context.Background()
	base := revocationRedis(t)
	url := base.rdb.Options().Addr
	a := NewRevocations(base)
	b := newReplica(t, "redis://"+url)
	now := time.Now()

	// 1. Un token válido funciona en ambas réplicas.
	for _, r := range []*Revocations{a, b} {
		if rev, err := r.IsRevoked(ctx, "u1", "j1", now); err != nil || rev {
			t.Fatalf("token válido = %v %v", rev, err)
		}
	}
	// 2. Logout en la réplica A: la B lo rechaza de inmediato. Se mide el
	// tiempo desde que A confirma hasta que B lo ve.
	start := time.Now()
	if err := a.RevokeToken(ctx, "j1", now.Add(15*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if rev, _ := b.IsRevoked(ctx, "u1", "j1", now); !rev {
		t.Fatal("la réplica B debe ver la revocación")
	}
	t.Logf("propagación de la revocación entre réplicas: %v (objetivo ≤ 60 s)", time.Since(start))
	// Otra sesión del mismo usuario sigue válida; otro usuario, también.
	if rev, _ := b.IsRevoked(ctx, "u1", "j2", now); rev {
		t.Error("logout no debe cerrar otras sesiones")
	}
	// 3. Baja de cuenta en B: A rechaza todo lo emitido antes.
	if err := b.RevokeUserBefore(ctx, "u2", now, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	if rev, _ := a.IsRevoked(ctx, "u2", "x", now.Add(-time.Minute)); !rev {
		t.Error("baja de cuenta en todas las réplicas")
	}
	if rev, _ := a.IsRevoked(ctx, "u9", "x", now.Add(-time.Minute)); rev {
		t.Error("no se invalidan sesiones de otros usuarios")
	}
	// Un corte viejo no deshace el nuevo (carrera entre logout y baja).
	_ = a.RevokeUserBefore(ctx, "u2", now.Add(-time.Hour), time.Minute)
	if rev, _ := b.IsRevoked(ctx, "u2", "x", now.Add(-time.Minute)); !rev {
		t.Error("el corte no debe bajar")
	}
	// 4. «Reiniciar la API» (cliente nuevo): la revocación sigue.
	c := newReplica(t, "redis://"+url)
	if rev, _ := c.IsRevoked(ctx, "u1", "j1", now); !rev {
		t.Error("un reinicio no rehabilita el token")
	}
	// 5. Toda clave tiene TTL (el espacio no crece sin límite) y no guarda el token.
	for _, k := range []string{base.key(revokeJTIPrefix + "j1"), base.key(revokeUserPrefix + "u2")} {
		ttl, err := base.rdb.PTTL(ctx, k).Result()
		if err != nil || ttl <= 0 || ttl > 16*time.Minute {
			t.Errorf("TTL de %s = %v %v", k, ttl, err)
		}
	}
	// 6. Un token ya vencido no se guarda.
	_ = a.RevokeToken(ctx, "viejo", now.Add(-time.Second))
	if n, _ := base.rdb.Exists(ctx, base.key(revokeJTIPrefix+"viejo")).Result(); n != 0 {
		t.Error("no se guarda lo vencido")
	}
}

func TestIntegrationRevocationLatency(t *testing.T) {
	ctx := context.Background()
	r := NewRevocations(revocationRedis(t))
	now := time.Now()
	const n = 2000
	d := make([]time.Duration, 0, n)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ { // 8 peticiones concurrentes, como varias réplicas
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n/8; i++ {
				s := time.Now()
				if _, err := r.IsRevoked(ctx, "u1", "j", now); err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				d = append(d, time.Since(s))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	p50, p95 := d[len(d)/2], d[len(d)*95/100]
	t.Logf("IsRevoked, %d consultas con 8 en paralelo: p50 %v, p95 %v", len(d), p50, p95)
	if p95 > 50*time.Millisecond {
		t.Errorf("p95 demasiado alto: %v", p95)
	}
}

func TestIntegrationRevocationConcurrentCutoff(t *testing.T) {
	ctx := t.Context()
	base := revocationRedis(t)
	now := time.Now().Truncate(time.Second)
	cutoffs := []time.Time{now.Add(-time.Minute), now.Add(-15 * time.Second), now, now.Add(-time.Second)}
	var writers sync.WaitGroup
	for _, cutoff := range cutoffs {
		replica := newReplica(t, "redis://"+base.rdb.Options().Addr)
		writers.Add(1)
		go func() {
			defer writers.Done()
			if err := replica.RevokeUserBefore(ctx, "concurrent-user", cutoff, time.Minute); err != nil {
				t.Error(err)
			}
		}()
	}
	writers.Wait()
	reader := NewRevocations(base)
	old, err := reader.IsRevoked(ctx, "concurrent-user", "i", now)
	if err != nil || !old {
		t.Fatalf("el corte concurrente debe conservar el mayor valor: %v %v", old, err)
	}
	fresh, err := reader.IsRevoked(ctx, "concurrent-user", "i", now.Add(time.Second))
	if err != nil || fresh {
		t.Fatalf("un token posterior al corte debe seguir vigente: %v %v", fresh, err)
	}
}

func TestIntegrationRevocationFailsClosedWhenRedisDown(t *testing.T) {
	ctx := context.Background()
	c, err := NewClient("redis://127.0.0.1:1", "test") // nadie escucha
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	r := NewRevocations(c)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rev, err := r.IsRevoked(ctx, "u1", "j1", time.Now())
	if err == nil {
		t.Fatal("sin Redis debe devolver error para que la ruta falle cerrada")
	}
	if rev {
		t.Error("con error no se afirma nada")
	}
	if r.RevokeToken(ctx, "j1", time.Now().Add(time.Minute)) == nil || r.RevokeUserBefore(ctx, "u1", time.Now(), time.Minute) == nil {
		t.Error("revocar sin Redis debe fallar")
	}
}
