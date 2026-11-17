package config

import (
	"strconv"
	"time"
)

// minJWTSecretLen es la longitud mínima de JWT_ACCESS_SECRET (spec 003, P-02).
const minJWTSecretLen = 32

// Auth es la configuración de la autenticación (spec 003, plan §5).
type Auth struct {
	// AccessSecret firma los tokens de acceso (HS256). Obligatorio salvo en test.
	AccessSecret Secret
	// AccessTTL es la vida del token de acceso (15 min por defecto).
	AccessTTL time.Duration
	// RefreshTTL es la vida del refresh token (30 días por defecto).
	RefreshTTL time.Duration
	// RateLimitPerMin es el límite por IP de register, login y refresh (RF-07).
	RateLimitPerMin int
	// LoginAttemptsPerAccount son los intentos de login por cuenta cada
	// 15 min (20 por defecto); complementa el límite por IP.
	LoginAttemptsPerAccount int
}

// loadAuth exige JWT_ACCESS_SECRET (≥ 32 caracteres) en todos los entornos
// salvo test. El valor nunca aparece en los mensajes de error.
func loadAuth(r *reader, appEnv string) Auth {
	a := Auth{
		AccessTTL:               r.duration("JWT_ACCESS_TTL", 15*time.Minute),
		RefreshTTL:              r.duration("REFRESH_TTL", 720*time.Hour),
		RateLimitPerMin:         r.positiveInt("RATE_LIMIT_AUTH_PER_MIN", 10),
		LoginAttemptsPerAccount: r.positiveInt("LOGIN_ATTEMPTS_PER_ACCOUNT", 20),
	}
	var secret string
	if appEnv == EnvTest {
		secret = r.text("JWT_ACCESS_SECRET", "")
	} else {
		secret = r.required("JWT_ACCESS_SECRET")
	}
	if secret != "" && len(secret) < minJWTSecretLen {
		r.fail("JWT_ACCESS_SECRET", "debe tener al menos 32 caracteres")
	}
	a.AccessSecret = Secret(secret)
	return a
}

// positiveInt lee un entero mayor que cero.
func (r *reader) positiveInt(key string, def int) int {
	v, ok := r.value(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		r.fail(key, "debe ser un entero mayor que cero")
		return def
	}
	return n
}
