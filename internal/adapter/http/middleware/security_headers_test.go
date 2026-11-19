package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func headersFor(t *testing.T, cfg SecurityHeadersConfig, h http.Handler, mutate func(*http.Request)) http.Header {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/v1/items", nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	SecurityHeaders(cfg)(h).ServeHTTP(rec, req)
	return rec.Result().Header
}

var plainOK = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	for _, status := range []int{200, 404, 500} {
		h := headersFor(t, SecurityHeadersConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }), nil)
		for k, want := range map[string]string{
			"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer",
			"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		} {
			if got := h.Values(k); len(got) != 1 || got[0] != want {
				t.Errorf("%d %s = %v", status, k, got)
			}
		}
		if h.Get("Cache-Control") != "" || h.Get("Strict-Transport-Security") != "" {
			t.Errorf("%d: sin sesión ni TLS no hay no-store ni HSTS: %v", status, h)
		}
	}
}

func TestSecurityHeadersNoStoreWhenAuthenticated(t *testing.T) {
	h := headersFor(t, SecurityHeadersConfig{}, plainOK, func(r *http.Request) { r.Header.Set("Authorization", "Bearer x") })
	if h.Get("Cache-Control") != "no-store" {
		t.Errorf("autenticada = %v", h)
	}
	// El handler que ya pone no-store (handover) no lo duplica.
	dup := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(201)
	})
	h = headersFor(t, SecurityHeadersConfig{}, dup, func(r *http.Request) { r.Header.Set("Authorization", "Bearer x") })
	if got := h.Values("Cache-Control"); len(got) != 1 {
		t.Errorf("Cache-Control duplicada: %v", got)
	}
}

func TestSecurityHeadersHSTSOnlyOverTLSWhenConfigured(t *testing.T) {
	tlsReq := func(r *http.Request) { r.TLS = &tls.ConnectionState{} }
	h := headersFor(t, SecurityHeadersConfig{HSTSMaxAge: HSTSStagingMaxAge}, plainOK, tlsReq)
	if h.Get("Strict-Transport-Security") != "max-age=86400" {
		t.Errorf("HSTS = %q (sin includeSubDomains ni preload)", h.Get("Strict-Transport-Security"))
	}
	if h := headersFor(t, SecurityHeadersConfig{HSTSMaxAge: time.Hour}, plainOK, nil); h.Get("Strict-Transport-Security") != "" {
		t.Error("nunca HSTS por HTTP")
	}
	if h := headersFor(t, SecurityHeadersConfig{}, plainOK, tlsReq); h.Get("Strict-Transport-Security") != "" {
		t.Error("sin configurar (producción) no hay HSTS")
	}
}

func TestSecurityHeadersHandlerCanOverride(t *testing.T) {
	docs := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.WriteHeader(200)
	})
	if got := headersFor(t, SecurityHeadersConfig{}, docs, nil).Values("Content-Security-Policy"); len(got) != 1 || got[0] != "default-src 'self'" {
		t.Errorf("CSP de /docs = %v", got)
	}
}

func TestClientSubject(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "198.51.100.77:1234"
	if got := ClientSubject(r); got != "ip:198.51.100.0/24" {
		t.Errorf("ClientSubject = %q", got)
	}
	r.RemoteAddr = "sin-puerto"
	if got := ClientSubject(r); got != "ip:invalid" {
		t.Errorf("ClientSubject = %q", got)
	}
}
