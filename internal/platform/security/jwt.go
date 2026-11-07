package security

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MinSecretLen es la longitud mínima del secreto de JWT (plan P-02).
const MinSecretLen = 32

// ErrInvalidToken indica un token de acceso ausente, mal formado, con firma o
// algoritmo incorrectos o vencido. No distingue el motivo a propósito.
var ErrInvalidToken = errors.New("security: token de acceso inválido")

// AccessClaims es la identidad que lleva un token de acceso válido.
type AccessClaims struct {
	UserID    string
	Role      string
	ExpiresAt time.Time
}

type claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// JWT emite y valida tokens de acceso HS256 (plan P-02) con los claims sub,
// role, exp, iat y jti.
type JWT struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewJWT crea el emisor. secret debe tener al menos MinSecretLen caracteres.
func NewJWT(secret string, ttl time.Duration) (*JWT, error) {
	if len(secret) < MinSecretLen {
		return nil, fmt.Errorf("security: el secreto de JWT debe tener al menos %d caracteres", MinSecretLen)
	}
	if ttl <= 0 {
		return nil, errors.New("security: la vida del token de acceso debe ser positiva")
	}
	return &JWT{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

// TTL es la vida de los tokens de acceso.
func (j *JWT) TTL() time.Duration { return j.ttl }

// Issue firma un token de acceso para userID con su rol.
func (j *JWT) Issue(userID, role string) (string, error) {
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", fmt.Errorf("security: generar jti: %w", err)
	}
	now := j.now().UTC()
	c := claims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.ttl)),
			ID:        hex.EncodeToString(jti),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(j.secret)
	if err != nil {
		return "", fmt.Errorf("security: firmar token: %w", err)
	}
	return s, nil
}

// Parse valida firma, algoritmo (solo HS256) y vencimiento, y devuelve la
// identidad. Cualquier problema devuelve ErrInvalidToken.
func (j *JWT) Parse(token string) (AccessClaims, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c,
		func(*jwt.Token) (any, error) { return j.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(j.now),
	)
	if err != nil || c.Subject == "" || c.Role == "" {
		return AccessClaims{}, ErrInvalidToken
	}
	return AccessClaims{UserID: c.Subject, Role: c.Role, ExpiresAt: c.ExpiresAt.Time}, nil
}
