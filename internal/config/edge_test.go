package config

import (
	"strings"
	"testing"
)

func TestEdgeDefaultsAreOff(t *testing.T) {
	cfg, err := load(valid().lookup)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Edge.CompressionEnabled || len(cfg.Edge.TrustedProxies) != 0 {
		t.Errorf("por omisión todo apagado: %+v", cfg.Edge)
	}
}

func TestEdgeParses(t *testing.T) {
	cfg, err := load(with(map[string]string{"HTTP_COMPRESSION_ENABLED": "true", "TRUSTED_PROXIES": "172.30.0.0/16, 10.1.2.3/32"}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Edge.CompressionEnabled || len(cfg.Edge.TrustedProxies) != 2 || cfg.Edge.TrustedProxies[0].String() != "172.30.0.0/16" {
		t.Errorf("edge = %+v", cfg.Edge)
	}
}

func TestEdgeRejectsBadCIDR(t *testing.T) {
	_, err := load(with(map[string]string{"TRUSTED_PROXIES": "10.0.0.1"}))
	if err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
		t.Errorf("err = %v", err)
	}
}

// with agrega variables al mínimo válido.
func with(extra map[string]string) lookupFunc {
	e := valid()
	for k, v := range extra {
		e[k] = v
	}
	return e.lookup
}

func TestEdgeGlobalRateLimitOnlyInDevelopment(t *testing.T) {
	cfg, err := load(valid().lookup)
	if err != nil || cfg.Edge.GlobalRateLimitPerMin != DefaultGlobalRateLimitPerMin {
		t.Fatalf("por omisión 120: %v %v", cfg.Edge.GlobalRateLimitPerMin, err)
	}
	cfg, err = load(with(map[string]string{"RATE_LIMIT_GLOBAL_PER_MIN": "100000"}))
	if err != nil || cfg.Edge.GlobalRateLimitPerMin != 100000 {
		t.Fatalf("development permite cambiarlo: %v %v", cfg.Edge.GlobalRateLimitPerMin, err)
	}
	_, err = load(with(map[string]string{"APP_ENV": "staging", "RATE_LIMIT_GLOBAL_PER_MIN": "100000", "REDIS_URL": "redis://r:6379", "TLS_CERT_FILE": "c", "TLS_KEY_FILE": "k"}))
	if err == nil || !strings.Contains(err.Error(), "RATE_LIMIT_GLOBAL_PER_MIN") {
		t.Errorf("staging no permite cambiarlo: %v", err)
	}
}
