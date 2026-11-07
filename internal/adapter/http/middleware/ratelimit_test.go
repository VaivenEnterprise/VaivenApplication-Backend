package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// countingLimiter permite limit peticiones por clave; si err no es nil, falla.
type countingLimiter struct {
	mu     sync.Mutex
	err    error
	counts map[string]int
	keys   []string
}

func newCounting() *countingLimiter { return &countingLimiter{counts: map[string]int{}} }

func (c *countingLimiter) Allow(_ context.Context, key string, limit int, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = append(c.keys, key)
	if c.err != nil {
		return false, c.err
	}
	c.counts[key]++
	return c.counts[key] <= limit, nil
}

func (c *countingLimiter) setErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func request(h http.Handler, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimitPerIP(t *testing.T) {
	primary := newCounting()
	cfg := RateLimitConfig{Limit: 2, Window: 90 * time.Second}
	h := RequestID(RateLimit(primary, newCounting(), cfg, slog.New(slog.DiscardHandler))(okHandler()))

	for i := 1; i <= 2; i++ {
		if rec := request(h, "203.0.113.7:5000"); rec.Code != http.StatusOK {
			t.Fatalf("petición %d = %d, want 200", i, rec.Code)
		}
	}
	rec := request(h, "203.0.113.7:6000") // otro puerto, misma IP
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("tercera petición = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "90" {
		t.Errorf("Retry-After = %q, want 90", rec.Header().Get("Retry-After"))
	}
	var env struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"requestId"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("429 sin envoltorio JSON: %v", err)
	}
	if env.Error.Code != "rate_limited" || env.Error.Message == "" || env.Error.RequestID == "" {
		t.Errorf("error = %+v", env.Error)
	}
	if rec := request(h, "198.51.100.1:5000"); rec.Code != http.StatusOK {
		t.Errorf("otra IP tiene su propio límite, status = %d", rec.Code)
	}
	if primary.keys[0] != "ip:203.0.113.7" {
		t.Errorf("clave = %q, want ip:203.0.113.7", primary.keys[0])
	}
}

func TestRateLimitIgnoresForwardedFor(t *testing.T) {
	primary := newCounting()
	h := RateLimit(primary, nil, RateLimitConfig{Limit: 1, Window: time.Second}, slog.New(slog.DiscardHandler))(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.7:1"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if primary.keys[0] != "ip:203.0.113.7" {
		t.Errorf("clave = %q: no debe confiar en X-Forwarded-For", primary.keys[0])
	}
}

func TestRateLimitPerUser(t *testing.T) {
	primary := newCounting()
	cfg := RateLimitConfig{Limit: 5, Window: time.Second, UserID: func(r *http.Request) (string, bool) {
		id := r.Header.Get("X-Test-User")
		return id, id != ""
	}}
	h := RateLimit(primary, nil, cfg, slog.New(slog.DiscardHandler))(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Test-User", "u-42")
	h.ServeHTTP(httptest.NewRecorder(), req)
	request(h, "203.0.113.7:1") // sin usuario: por IP

	if primary.keys[0] != "user:u-42" || primary.keys[1] != "ip:203.0.113.7" {
		t.Errorf("claves = %v", primary.keys)
	}
}

func TestRateLimitFallsBackAndLogsOnce(t *testing.T) {
	primary, fallback := newCounting(), newCounting()
	primary.setErr(errors.New("redis: connection refused"))
	var logs bytes.Buffer
	cfg := RateLimitConfig{Limit: 2, Window: time.Second}
	h := RateLimit(primary, fallback, cfg, slog.New(slog.NewTextHandler(&logs, nil)))(okHandler())

	codes := []int{}
	for i := 0; i < 3; i++ {
		codes = append(codes, request(h, "203.0.113.7:1").Code)
	}
	// El limitador local sigue aplicando el límite (CA-S5: la API no se cae).
	if codes[0] != 200 || codes[1] != 200 || codes[2] != http.StatusTooManyRequests {
		t.Errorf("códigos = %v", codes)
	}
	if n := strings.Count(logs.String(), "límite de tasa degradado"); n != 1 {
		t.Errorf("aviso de degradación registrado %d veces, want 1:\n%s", n, logs.String())
	}

	// Al volver Redis se registra la recuperación una vez.
	primary.setErr(nil)
	request(h, "198.51.100.1:1")
	request(h, "198.51.100.1:1")
	if n := strings.Count(logs.String(), "restablecido"); n != 1 {
		t.Errorf("recuperación registrada %d veces, want 1", n)
	}
}

func TestRateLimitWithoutAnyLimiterLetsThrough(t *testing.T) {
	primary := newCounting()
	primary.setErr(errors.New("caído"))
	broken := newCounting()
	broken.setErr(errors.New("también caído"))

	for _, fb := range []*countingLimiter{nil, broken} {
		var fallback interface {
			Allow(context.Context, string, int, time.Duration) (bool, error)
		}
		if fb != nil {
			fallback = fb
		}
		h := RateLimit(primary, fallback, RateLimitConfig{Limit: 1, Window: time.Second}, slog.New(slog.DiscardHandler))(okHandler())
		if rec := request(h, "203.0.113.7:1"); rec.Code != http.StatusOK {
			t.Errorf("sin limitador disponible debe dejar pasar, status = %d", rec.Code)
		}
	}
}

func TestLimitKeyWithoutPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "pipe"
	if got := limitKey(req, nil); got != "ip:pipe" {
		t.Errorf("limitKey = %q", got)
	}
}
