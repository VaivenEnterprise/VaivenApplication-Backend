//go:build integration

package redis

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/revocation"
)

func TestIntegrationRevocationJWTFixture(t *testing.T) {
	ctx := t.Context()
	base := revocationRedis(t)
	first := NewRevocations(base)
	second := newReplica(t, "redis://"+base.rdb.Options().Addr)

	// Las llaves solo existen durante la prueba. El verificador de la API sigue intacto.
	issuer, signingKey := newLabJWT(t)
	otherIssuer, _ := newLabJWT(t)

	// La API todavía no devuelve jti e iat. El fixture los recupera de los
	// mismos bytes ya validados, verificando también su firma en esta lectura.
	verify := func(token string) (security.AccessClaims, jwt.RegisteredClaims, error) {
		identity, err := issuer.Parse(token)
		if err != nil {
			return security.AccessClaims{}, jwt.RegisteredClaims{}, err
		}
		var metadata jwt.RegisteredClaims
		_, err = jwt.ParseWithClaims(token, &metadata, func(*jwt.Token) (any, error) {
			return signingKey, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
		if err == nil && (metadata.ID == "" || metadata.IssuedAt == nil || metadata.Subject != identity.UserID) {
			err = security.ErrInvalidToken
		}
		return identity, metadata, err
	}
	accepted := func(store revocation.Store, token string) (bool, error) {
		identity, metadata, err := verify(token)
		if err != nil {
			return false, err
		}
		revoked, err := store.IsRevoked(ctx, identity.UserID, metadata.ID, metadata.IssuedAt.Time)
		return !revoked && err == nil, err
	}
	issue := func(issuer *security.JWT, subject string) string {
		t.Helper()
		token, err := issuer.Issue(subject, "user")
		if err != nil {
			t.Fatal("no se pudo crear el JWT sintético")
		}
		return token
	}

	const subject = "lab-jwt-revocation"
	token := issue(issuer, subject)
	otherToken := issue(issuer, subject)
	for _, store := range []revocation.Store{first, second} {
		if ok, err := accepted(store, token); !ok || err != nil {
			t.Fatal("ambos clientes deben aceptar el JWT vigente")
		}
	}

	_, metadata, err := verify(token)
	if err != nil {
		t.Fatal("no se pudo comprobar la identidad sintética")
	}
	start := time.Now()
	if err := first.RevokeToken(ctx, metadata.ID, metadata.ExpiresAt.Time); err != nil {
		t.Fatal("la primera réplica no pudo revocar el jti")
	}
	if ok, err := accepted(second, token); ok || err != nil || time.Since(start) > time.Minute {
		t.Fatal("la segunda réplica debe rechazar el jti revocado en ≤ 60 s")
	}
	if ok, err := accepted(second, otherToken); !ok || err != nil {
		t.Fatal("la revocación de un jti no debe revocar otro token")
	}
	if err := second.RevokeUserBefore(ctx, subject, time.Now(), issuer.TTL()); err != nil {
		t.Fatal("la segunda réplica no pudo escribir el corte de cuenta")
	}
	if ok, err := accepted(first, otherToken); ok || err != nil {
		t.Fatal("el corte de cuenta debe alcanzar los tokens previos en ambas réplicas")
	}

	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": subject, "role": "user", "exp": time.Now().Add(-time.Minute).Unix(),
	}).SignedString(signingKey)
	if err != nil {
		t.Fatal("no se pudo crear el JWT sintético vencido")
	}
	for _, invalid := range []string{expired, issue(otherIssuer, subject)} {
		if ok, err := accepted(second, invalid); ok || !errors.Is(err, security.ErrInvalidToken) {
			t.Fatal("el verificador debe rechazar el JWT vencido o firmado con otra llave")
		}
	}

	unreachable, err := NewClient("redis://127.0.0.1:1", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unreachable.Close() })
	if ok, err := accepted(NewRevocations(unreachable), issue(issuer, "lab-jwt-store-down")); ok || err == nil {
		t.Fatal("el fixture debe rechazar un JWT válido si el almacén no responde")
	}
}

func newLabJWT(t *testing.T) (*security.JWT, []byte) {
	t.Helper()
	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		t.Fatal("no se pudo generar la llave efímera")
	}
	key := hex.EncodeToString(entropy[:])
	issuer, err := security.NewJWT(key, 15*time.Minute)
	if err != nil {
		t.Fatal("no se pudo crear el emisor sintético")
	}
	return issuer, []byte(key)
}
