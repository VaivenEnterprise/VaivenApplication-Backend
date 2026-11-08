package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

// AccountLoginWindow es la ventana del límite de intentos de login por cuenta.
const AccountLoginWindow = 15 * time.Minute

// accountLimiter limita los intentos de login por cuenta (auditoría 2026-11-07,
// hallazgo 2): complementa el límite por IP contra el relleno de credenciales
// desde muchas IP. Cuenta todos los intentos al correo, existan o no, para no
// revelar qué correos tienen cuenta. La clave es el SHA-256 del correo
// normalizado: Redis nunca guarda el correo (Art. VI.5). Si Redis falla usa el
// respaldo local y, sin ninguno, deja pasar (el límite protege, no corta el
// servicio; Art. XIV.2).
type accountLimiter struct {
	primary, fallback ratelimit.Limiter
	limit             int
}

// allow indica si el correo aún tiene intentos en la ventana.
func (a accountLimiter) allow(ctx context.Context, email string) bool {
	if a.primary == nil || a.limit <= 0 {
		return true
	}
	sum := sha256.Sum256([]byte(domain.NormalizeEmail(email)))
	key := "login-account:" + hex.EncodeToString(sum[:])
	ok, err := a.primary.Allow(ctx, key, a.limit, AccountLoginWindow)
	if err == nil {
		return ok
	}
	if a.fallback != nil {
		if ok, err := a.fallback.Allow(ctx, key, a.limit, AccountLoginWindow); err == nil {
			return ok
		}
	}
	return true
}
