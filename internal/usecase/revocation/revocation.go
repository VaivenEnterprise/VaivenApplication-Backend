// Package revocation declara el puerto de revocación de tokens de acceso
// (spec 018, C-12; propuesta no aprobada). Las implementaciones viven en
// internal/platform/redis.
//
// Estado: PREPARADO. Ningún módulo lo usa todavía: conectarlo al middleware
// de autenticación toca internal/auth y requiere el visto bueno de BE-A
// (Art. XII.1). Ver docs/seguridad/revocacion-jwt.md.
//
// Política de fallo: si el almacén no responde, IsRevoked devuelve error y
// quien lo usa debe FALLAR CERRADO en las rutas protegidas (401 o 503), nunca
// aceptar el token. Las rutas públicas no consultan el almacén.
package revocation

import (
	"context"
	"time"
)

// Store guarda revocaciones con vencimiento. Nunca guarda el token completo:
// solo su jti (identificador aleatorio) o un corte por usuario.
type Store interface {
	// RevokeToken revoca un token por su jti hasta expiresAt (cuando el token
	// vence ya no hace falta recordarlo).
	RevokeToken(ctx context.Context, jti string, expiresAt time.Time) error
	// RevokeUserBefore revoca todos los tokens del usuario emitidos en o antes
	// de cutoff, durante ttl (la vida máxima de un token de acceso). Solo sube
	// el corte: una revocación vieja no deshace una nueva.
	RevokeUserBefore(ctx context.Context, userID string, cutoff time.Time, ttl time.Duration) error
	// IsRevoked dice si el token (usuario, jti, emitido en issuedAt) está
	// revocado, en una sola consulta.
	IsRevoked(ctx context.Context, userID, jti string, issuedAt time.Time) (bool, error)
}
