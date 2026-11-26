package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

var proxyNet = []netip.Prefix{netip.MustParsePrefix("172.30.0.0/16")}

func seenIP(t *testing.T, trusted []netip.Prefix, remote string, xff ...string) string {
	t.Helper()
	var got string
	h := ClientIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.RemoteAddr
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = remote
	for _, v := range xff {
		req.Header.Add("X-Forwarded-For", v)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestClientIPWithoutTrustedProxiesIgnoresHeader(t *testing.T) {
	if got := seenIP(t, nil, "203.0.113.7:5000", "1.2.3.4"); got != "203.0.113.7:5000" {
		t.Errorf("sin proxies de confianza = %q", got)
	}
}

func TestClientIPFromTrustedProxy(t *testing.T) {
	if got := seenIP(t, proxyNet, "172.30.0.5:4000", "198.51.100.9"); got != "198.51.100.9:0" {
		t.Errorf("vía proxy = %q", got)
	}
}

func TestClientIPSpoofingIsIgnored(t *testing.T) {
	// El cliente antepone una IP falsa; el proxy agrega la real a la derecha.
	if got := seenIP(t, proxyNet, "172.30.0.5:4000", "10.9.9.9, 198.51.100.9"); got != "198.51.100.9:0" {
		t.Errorf("falsificación a la izquierda = %q", got)
	}
	// Directo a la API (sin proxy) con cabecera falsa: se ignora.
	if got := seenIP(t, proxyNet, "198.51.100.9:4000", "1.1.1.1"); got != "198.51.100.9:4000" {
		t.Errorf("directo con cabecera = %q", got)
	}
	// Valor basura a la derecha: no se confía en nada.
	if got := seenIP(t, proxyNet, "172.30.0.5:4000", "1.1.1.1, basura"); got != "172.30.0.5:4000" {
		t.Errorf("basura = %q", got)
	}
	// Solo proxies en la cadena: se queda la conexión.
	if got := seenIP(t, proxyNet, "172.30.0.5:4000", "172.30.0.9"); got != "172.30.0.5:4000" {
		t.Errorf("solo proxies = %q", got)
	}
	// Varias cabeceras: cuenta la última.
	if got := seenIP(t, proxyNet, "172.30.0.5:4000", "1.1.1.1", "198.51.100.10"); got != "198.51.100.10:0" {
		t.Errorf("varias cabeceras = %q", got)
	}
	// IPv4 en IPv6 y dirección sin puerto.
	if got := seenIP(t, proxyNet, "[::ffff:172.30.0.5]:4000", "2001:db8::1"); got != "[2001:db8::1]:0" {
		t.Errorf("ipv6 = %q", got)
	}
	if got := seenIP(t, proxyNet, "no-es-ip", "1.1.1.1"); got != "no-es-ip" {
		t.Errorf("remote inválido = %q", got)
	}
}

type countLimiter struct{ n, max int }

func (c *countLimiter) Allow(context.Context, string, int, time.Duration) (bool, error) {
	c.n++
	return c.n <= c.max, nil
}

func TestRateLimitBlockedEvent(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	h := RequestID(RateLimit(&countLimiter{max: 1}, nil, RateLimitConfig{Limit: 1, Window: time.Minute, KeyPrefix: "auth:"}, log)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })))
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"password":"no-se-registra"}`))
		req.RemoteAddr = "198.51.100.77:1234"
		req.Header.Set("Authorization", "Bearer secreto")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if i == 1 && rec.Code != 429 {
			t.Fatalf("segunda = %d", rec.Code)
		}
	}
	var ev map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &ev); err != nil {
		t.Fatalf("un solo evento JSON: %q", buf.String())
	}
	want := map[string]any{"msg": "security_event", "event": "ratelimit.blocked", "result": "blocked", "severity": "medium", "category": "auth", "subject": "ip:198.51.100.0/24"}
	for k, v := range want {
		if ev[k] != v {
			t.Errorf("%s = %v, quiero %v", k, ev[k], v)
		}
	}
	if ev["requestId"] == "" || ev["time"] == nil {
		t.Error("faltan requestId o time")
	}
	for _, leak := range []string{"198.51.100.77", "secreto", "no-se-registra", "Bearer"} {
		if strings.Contains(buf.String(), leak) {
			t.Errorf("el evento filtra %q", leak)
		}
	}
}

func TestPseudonymize(t *testing.T) {
	for in, want := range map[string]string{
		"user:abc": "user:abc", "ip:2001:db8:1:2::9": "ip:2001:db8:1::/48",
		"ip:::ffff:10.1.2.3": "ip:10.1.2.0/24", "ip:x": "ip:invalid", "otra": "other",
	} {
		if got := pseudonymize(in); got != want {
			t.Errorf("pseudonymize(%q) = %q, quiero %q", in, got, want)
		}
	}
	if limitCategory("") != "global" || limitCategory("ai:assistant:") != "ai:assistant" {
		t.Error("limitCategory")
	}
	securityEvent(httptest.NewRequest("GET", "/", nil), nil, "e", "r", "s", "c", "k") // sin logger: no hace nada
}
