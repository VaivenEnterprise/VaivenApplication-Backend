// Package auth expone la identidad de la petición a todos los módulos (spec
// 003, plan P-07): UserID y Role son el único modo de leerla. El middleware
// Authenticate (internal/auth/adapter/http) la guarda con WithIdentity. Solo
// usa la biblioteca estándar, así que lo pueden importar domain y usecase.
package auth

import "context"

type identityKey struct{}

// Identity es la persona autenticada de la petición.
type Identity struct {
	UserID string
	Role   string
}

// WithIdentity devuelve una copia de ctx con la identidad.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// UserID devuelve el usuario autenticado y true, o "" y false si la petición
// no está autenticada.
func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	if !ok || id.UserID == "" {
		return "", false
	}
	return id.UserID, true
}

// Role devuelve el rol del usuario autenticado, o "" si no hay identidad.
func Role(ctx context.Context) string {
	id, _ := ctx.Value(identityKey{}).(Identity)
	return id.Role
}
