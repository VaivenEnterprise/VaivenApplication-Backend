package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/revocation"
)

// RevocationChecker consulta si un token de acceso fue revocado antes de
// vencer (spec 018, C-12); lo implementa internal/platform/redis.
type RevocationChecker interface {
	IsAccessRevoked(ctx context.Context, a revocation.Access) (bool, error)
}

// errRevocationUnavailable es el 503 cuando no se puede saber si el token fue
// revocado: la ruta protegida FALLA CERRADA (spec 018, C-12).
// revocationTimeout acota la consulta: con Redis caído se responde 503 rápido
// en lugar de esperar los reintentos de conexión.
const revocationTimeout = 500 * time.Millisecond

var errRevocationUnavailable = shared.NewUnavailable("El servicio no está disponible por ahora. Intenta de nuevo en unos minutos.")

// TokenParser valida un token de acceso. *security.JWT lo cumple.
type TokenParser interface {
	Parse(token string) (security.AccessClaims, error)
}

// UserLookup lee una cuenta por ID. *usecase.Service (Me) lo cumple.
type UserLookup func(ctx context.Context, userID string) (domain.User, error)

// Authenticate exige «Authorization: Bearer <token>» válido y guarda la
// identidad en el contexto (auth.UserID, auth.Role). Sin token, o con uno
// inválido o vencido, responde 401 unauthorized (CA-04).
func Authenticate(tokens TokenParser) func(http.Handler) http.Handler {
	return AuthenticateWith(tokens, nil, nil)
}

// AuthenticateWith es Authenticate con revocación inmediata: un token
// revocado (logout, baja, reutilización de refresh) responde 401 aunque no
// haya vencido, y si el almacén de revocaciones no responde la ruta responde
// 503 en lugar de aceptar el token. revoked nil no consulta nada.
func AuthenticateWith(tokens TokenParser, revoked RevocationChecker, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
				respond.Error(w, r, domain.ErrInvalidSession)
				return
			}
			c, err := tokens.Parse(strings.TrimSpace(token))
			if err != nil {
				respond.Error(w, r, domain.ErrInvalidSession)
				return
			}
			if revoked != nil {
				qctx, cancel := context.WithTimeout(r.Context(), revocationTimeout)
				isRevoked, err := revoked.IsAccessRevoked(qctx, revocation.Access{
					UserID: c.UserID, SessionID: c.SessionID, TokenID: c.TokenID, IssuedAt: c.IssuedAt,
				})
				cancel()
				if err != nil {
					middleware.LogSecurityEvent(r.Context(), log, "auth.revocation_unavailable", "blocked", "high", "auth", "user:"+c.UserID)
					respond.Error(w, r, errRevocationUnavailable)
					return
				}
				if isRevoked {
					middleware.LogSecurityEvent(r.Context(), log, "auth.revoked_token_used", "denied", "medium", "auth", "user:"+c.UserID)
					respond.Error(w, r, domain.ErrInvalidSession)
					return
				}
			}
			ctx := auth.WithIdentity(r.Context(), auth.Identity{UserID: c.UserID, Role: c.Role})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole deja pasar solo a la identidad con el rol indicado; las demás
// reciben 403 forbidden (CA-07). Va después de Authenticate.
//
// El rol del token no basta (spec 012, AUD-017, CA-06): también se lee la
// cuenta con lookup y debe tener ese rol en la base y estar activa. Así, un
// token firmado con un secreto filtrado, o emitido antes de quitarle el rol a
// la cuenta, no abre las rutas de administración. La lectura solo ocurre si
// el token ya trae el rol pedido, así que las demás peticiones no tocan la
// base. lookup es obligatorio.
func RequireRole(role domain.Role, lookup UserLookup, log *slog.Logger) func(http.Handler) http.Handler {
	if lookup == nil {
		panic("authhttp.RequireRole: lookup es obligatorio (AUD-017)")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := auth.UserID(r.Context())
			if !ok {
				respond.Error(w, r, domain.ErrInvalidSession)
				return
			}
			if auth.Role(r.Context()) != string(role) {
				middleware.LogSecurityEvent(r.Context(), log, "authz.forbidden", "denied", "medium", "authz", "user:"+id)
				respond.Error(w, r, domain.ErrForbiddenRole)
				return
			}
			u, err := lookup(r.Context(), id)
			if err != nil {
				if domain.Is(err, domain.ErrUserNotFound) {
					respond.Error(w, r, domain.ErrInvalidSession)
					return
				}
				respond.Error(w, r, err)
				return
			}
			if u.Status != domain.StatusActive {
				respond.Error(w, r, domain.ErrInvalidSession)
				return
			}
			if u.Role != role {
				// El token dice un rol que la base ya no confirma (AUD-017).
				middleware.LogSecurityEvent(r.Context(), log, "authz.forbidden", "denied", "high", "authz", "user:"+id)
				respond.Error(w, r, domain.ErrForbiddenRole)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireActive bloquea con 403 forbidden a una cuenta suspendida (RF-13). Las
// specs 005 y 007 lo montan en las rutas que crean objetos o solicitudes,
// después de Authenticate. now nil = time.Now.
func RequireActive(lookup UserLookup, now func() time.Time) func(http.Handler) http.Handler {
	if now == nil {
		now = time.Now
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := auth.UserID(r.Context())
			if !ok {
				respond.Error(w, r, domain.ErrInvalidSession)
				return
			}
			u, err := lookup(r.Context(), id)
			if err != nil {
				respond.Error(w, r, err)
				return
			}
			if u.IsSuspended(now()) {
				respond.Error(w, r, domain.ErrSuspended)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
