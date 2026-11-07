//go:build integration

package redis

import (
	"context"
	"testing"
	"time"

	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// startRedis levanta Redis 7 real y devuelve un cliente con prefijo de prueba.
func startRedis(t *testing.T) *Client {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatalf("no se pudo iniciar Redis con Docker: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}

	c, err := NewClient(url, "test")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	return c
}

func TestIntegrationLimiter(t *testing.T) {
	c := startRedis(t)
	l := NewLimiter(c)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		if ok, err := l.Allow(ctx, "ip:1", 3, time.Second); err != nil || !ok {
			t.Fatalf("petición %d: ok=%v err=%v", i, ok, err)
		}
	}
	if ok, _ := l.Allow(ctx, "ip:1", 3, time.Second); ok {
		t.Error("la cuarta petición debía rechazarse")
	}

	// La clave lleva el prefijo del entorno y expira con la ventana.
	if n, _ := c.rdb.Exists(ctx, "vaiven:test:rl:ip:1").Result(); n != 1 {
		t.Error("la clave no lleva el prefijo vaiven:test:")
	}
	time.Sleep(1100 * time.Millisecond)
	if ok, _ := l.Allow(ctx, "ip:1", 3, time.Second); !ok {
		t.Error("tras la ventana el contador debía reiniciarse")
	}
}

func TestIntegrationCache(t *testing.T) {
	c := startRedis(t)
	k := NewCache(c)
	ctx := context.Background()

	if _, ok, err := k.Get(ctx, "categorias"); err != nil || ok {
		t.Fatalf("clave inexistente: ok=%v err=%v", ok, err)
	}
	if err := k.Set(ctx, "categorias", []byte(`["hogar"]`), time.Second); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := k.Get(ctx, "categorias"); err != nil || !ok || string(v) != `["hogar"]` {
		t.Fatalf("Get = %q, %v, %v", v, ok, err)
	}
	if ttl, _ := c.rdb.PTTL(ctx, "vaiven:test:cache:categorias").Result(); ttl <= 0 {
		t.Errorf("la clave debe tener TTL, PTTL = %v", ttl)
	}
	if err := k.Delete(ctx, "categorias"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := k.Get(ctx, "categorias"); ok {
		t.Error("Delete no borró la clave")
	}
}
