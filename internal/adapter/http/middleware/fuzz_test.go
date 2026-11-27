package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

// FuzzClientIP (spec 020, RF-10 e, RF-11): ni la dirección de la conexión ni
// X-Forwarded-For provocan un pánico; una conexión que no viene de un proxy de
// confianza nunca cambia de IP; y la IP elegida del encabezado es válida y
// nunca de una red de confianza (no se puede falsificar anteponiendo valores).
func FuzzClientIP(f *testing.F) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("fd00::/8")}
	f.Add("10.0.0.5:443", "203.0.113.7")
	f.Add("10.0.0.5:443", "198.51.100.1, 203.0.113.7, 10.0.0.9")
	f.Add("203.0.113.9:1234", "10.0.0.1")
	f.Add("[fd00::1]:80", "2001:db8::1")
	f.Add("10.0.0.5", "::ffff:203.0.113.7")
	f.Add("10.0.0.5:1", "garbage, 10.0.0.1")
	f.Add("", "")
	f.Add("10.0.0.5:1", ",,,,")
	f.Add("10.0.0.5:1", "1.2.3.4%eth0")
	f.Fuzz(func(t *testing.T, remote, xff string) {
		var got string
		h2 := ClientIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.RemoteAddr }))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remote
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		h2.ServeHTTP(httptest.NewRecorder(), req)
		if got == remote {
			return
		}
		peer, ok := addrOf(remote)
		if !ok || !isTrusted(trusted, peer) {
			t.Fatalf("cambió la IP de una conexión que no es de confianza: %q -> %q", remote, got)
		}
		host, _, err := net.SplitHostPort(got)
		if err != nil {
			t.Fatalf("IP elegida mal formada %q: %v", got, err)
		}
		a, err := netip.ParseAddr(host)
		if err != nil || isTrusted(trusted, a) {
			t.Fatalf("IP elegida inválida o de confianza: %q (%v)", got, err)
		}
	})
}

// TestClientIPIgnoresIPv6Zone es la regresión del hallazgo del fuzzing: una
// dirección con zona («::%[») dejaba RemoteAddr mal formado y evadía la
// comprobación de proxies de confianza.
func TestClientIPIgnoresIPv6Zone(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("fd00::/8")}
	for xff, want := range map[string]string{
		"::%[":                  "[::]:0",
		"2001:db8::1%a":         "[2001:db8::1]:0",
		"1.1.1.1, fd00::5%eth0": "1.1.1.1:0",
	} {
		var got string
		h := ClientIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.RemoteAddr }))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.5:443"
		req.Header.Set("X-Forwarded-For", xff)
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != want {
			t.Errorf("XFF %q: RemoteAddr = %q, quería %q", xff, got, want)
		}
	}
}
