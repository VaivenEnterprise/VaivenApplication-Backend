package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// refreshTokenBytes es la entropía de un refresh token (plan P-03).
const refreshTokenBytes = 32

// NewRefreshToken genera un refresh token opaco (32 bytes aleatorios en
// base64url sin relleno) y su hash. Solo el hash se guarda (plan P-03); el
// token se entrega una sola vez al cliente.
func NewRefreshToken() (token, hash string, err error) {
	b := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("security: generar refresh token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken devuelve el SHA-256 en hexadecimal de un refresh token. Basta un
// hash rápido: el token tiene 256 bits de entropía, no es una contraseña.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
