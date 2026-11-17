package redis

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/cache"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

// Las implementaciones cumplen los puertos que declaran los consumidores.
var (
	_ ratelimit.Limiter = (*Limiter)(nil)
	_ ratelimit.Limiter = (*MemoryLimiter)(nil)
	_ cache.Cache       = (*Cache)(nil)
	_ cache.Cache       = (*MemoryCache)(nil)
)

// clock es un reloj controlable para las versiones en memoria.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestPrefix(t *testing.T) {
	if got := Prefix("staging"); got != "vaiven:staging:" {
		t.Errorf("Prefix = %q", got)
	}
}

func TestNewClientInvalidURLHidesPassword(t *testing.T) {
	_, err := NewClient("http://:super-secreta@redis:6379", "test")
	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("err = %v, se esperaba ErrInvalidURL", err)
	}
	if strings.Contains(err.Error(), "super-secreta") {
		t.Errorf("el error revela la contraseña: %q", err)
	}
}

func TestClientWithoutServer(t *testing.T) {
	c, err := NewClient("redis://127.0.0.1:1/0", "test")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = c.Close() }()
	ctx := context.Background()

	if err := c.Ping(ctx); err == nil {
		t.Error("Ping: se esperaba error sin servidor")
	}
	if _, err := NewLimiter(c).Allow(ctx, "ip:1", 5, time.Second); err == nil {
		t.Error("Allow: se esperaba error sin servidor")
	}
	k := NewCache(c)
	if _, _, err := k.Get(ctx, "x"); err == nil {
		t.Error("Get: se esperaba error sin servidor")
	}
	if err := k.Set(ctx, "x", []byte("v"), time.Second); err == nil {
		t.Error("Set: se esperaba error sin servidor")
	}
	if err := k.Delete(ctx, "x"); err == nil {
		t.Error("Delete: se esperaba error sin servidor")
	}
}

func TestInvalidArguments(t *testing.T) {
	c, _ := NewClient("redis://127.0.0.1:1/0", "test")
	defer func() { _ = c.Close() }()
	ctx := context.Background()

	for _, l := range []ratelimit.Limiter{NewLimiter(c), NewMemoryLimiter()} {
		if _, err := l.Allow(ctx, "k", 0, time.Second); err == nil {
			t.Errorf("%T: límite 0 debe ser error", l)
		}
		if _, err := l.Allow(ctx, "k", 1, 0); err == nil {
			t.Errorf("%T: ventana 0 debe ser error", l)
		}
	}
	for _, k := range []cache.Cache{NewCache(c), NewMemoryCache()} {
		if err := k.Set(ctx, "k", []byte("v"), 0); err == nil {
			t.Errorf("%T: TTL 0 debe ser error", k)
		}
	}
}

func TestMemoryLimiterWindow(t *testing.T) {
	clk := &clock{t: time.Unix(1_000, 0)}
	l := NewMemoryLimiter()
	l.now = clk.now
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		if ok, _ := l.Allow(ctx, "ip:1", 3, time.Minute); !ok {
			t.Fatalf("petición %d dentro del límite fue rechazada", i)
		}
	}
	if ok, _ := l.Allow(ctx, "ip:1", 3, time.Minute); ok {
		t.Error("la cuarta petición debía rechazarse")
	}
	if ok, _ := l.Allow(ctx, "ip:2", 3, time.Minute); !ok {
		t.Error("otra clave tiene su propio contador")
	}

	clk.advance(time.Minute)
	if ok, _ := l.Allow(ctx, "ip:1", 3, time.Minute); !ok {
		t.Error("en la ventana siguiente el contador se reinicia")
	}
	if _, ok := l.windows["ip:2"]; ok {
		t.Error("las ventanas vencidas deben borrarse")
	}
}

func TestMemoryLimiterConcurrent(t *testing.T) {
	l := NewMemoryLimiter()
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Allow(context.Background(), "k", 10, time.Minute); ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 10 {
		t.Errorf("permitidas = %d, want 10", allowed)
	}
}

func TestMemoryCache(t *testing.T) {
	clk := &clock{t: time.Unix(1_000, 0)}
	k := NewMemoryCache()
	k.now = clk.now
	ctx := context.Background()

	if _, ok, _ := k.Get(ctx, "categorias"); ok {
		t.Fatal("una clave inexistente no debe encontrarse")
	}
	val := []byte(`["hogar"]`)
	if err := k.Set(ctx, "categorias", val, time.Minute); err != nil {
		t.Fatal(err)
	}
	val[0] = 'X' // la caché guarda una copia
	got, ok, err := k.Get(ctx, "categorias")
	if err != nil || !ok || string(got) != `["hogar"]` {
		t.Fatalf("Get = %q, %v, %v", got, ok, err)
	}

	clk.advance(time.Minute)
	if _, ok, _ := k.Get(ctx, "categorias"); ok {
		t.Error("al vencer el TTL la clave no debe encontrarse")
	}

	_ = k.Set(ctx, "x", []byte("v"), time.Minute)
	if err := k.Delete(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := k.Get(ctx, "x"); ok {
		t.Error("Delete no borró la clave")
	}
}
