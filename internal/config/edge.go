package config

import "net/netip"

// Edge configura lo que depende de cómo llega el tráfico a la API (spec 018,
// propuesta no aprobada; C-08 y prototipo de balanceo). Por omisión todo queda
// como hoy: sin compresión y sin confiar en ningún proxy.
type Edge struct {
	// CompressionEnabled activa gzip en las respuestas elegibles
	// (HTTP_COMPRESSION_ENABLED). Antes de encenderlo en un entorno real hay
	// que coordinar con la app móvil (spec 018, T014).
	CompressionEnabled bool
	// TrustedProxies son las redes (CIDR) de los proxies cuyo
	// X-Forwarded-For se acepta (TRUSTED_PROXIES). Vacío: la IP del cliente
	// es siempre la de la conexión (Art. XIV.4: hoy no hay proxy).
	TrustedProxies []netip.Prefix
	// GlobalRateLimitPerMin es el límite global por cliente
	// (RATE_LIMIT_GLOBAL_PER_MIN, 120 por omisión). Solo se puede cambiar en
	// development y test (por ejemplo, para el análisis ZAP aislado de la
	// spec 018, C-09); en staging y producción queda fijo en 120.
	GlobalRateLimitPerMin int
}

// DefaultGlobalRateLimitPerMin es el límite global del Art. XIV.2.
const DefaultGlobalRateLimitPerMin = 120

func loadEdge(r *reader, appEnv string) Edge {
	e := Edge{
		CompressionEnabled:    r.boolean("HTTP_COMPRESSION_ENABLED", false),
		GlobalRateLimitPerMin: r.positiveInt("RATE_LIMIT_GLOBAL_PER_MIN", DefaultGlobalRateLimitPerMin),
	}
	if e.GlobalRateLimitPerMin != DefaultGlobalRateLimitPerMin && appEnv != EnvDevelopment && appEnv != EnvTest {
		r.fail("RATE_LIMIT_GLOBAL_PER_MIN", "solo se puede cambiar en development y test")
		e.GlobalRateLimitPerMin = DefaultGlobalRateLimitPerMin
	}
	for _, s := range r.list("TRUSTED_PROXIES") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			r.fail("TRUSTED_PROXIES", "cada valor debe ser una red CIDR, por ejemplo 10.0.0.0/8")
			continue
		}
		e.TrustedProxies = append(e.TrustedProxies, p.Masked())
	}
	return e
}
