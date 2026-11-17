//go:build integration

package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// randomSecret genera un secreto de JWT ficticio para la prueba (64 hex).
func randomSecret(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// SEC-002: comportamiento real al rotar JWT_ACCESS_SECRET. Se simula con dos
// API sobre la MISMA base: la vieja (secreto anterior) y la reiniciada con el
// secreto nuevo.
//
//   - Los tokens de acceso firmados con el secreto anterior dejan de valer de
//     inmediato (no hay que esperar los 15 min de vida).
//   - Los refresh tokens NO dependen del secreto (son opacos, guardados como
//     SHA-256): siguen valiendo y emiten un acceso firmado con el secreto nuevo.
//     La rotación, por sí sola, no cierra sesiones.
//   - Una instancia que siga con el secreto anterior rechaza los tokens nuevos:
//     todas las réplicas se deben reiniciar con el mismo secreto.
func TestIntegrationJWTRotation(t *testing.T) {
	db := flowClient.Database(fmt.Sprintf("rot_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	oldSecret, newSecret := randomSecret(t), randomSecret(t)
	before := newAPIOn(t, db, oldSecret, 15*time.Minute)

	var reg authBody
	if st, code := before.call(http.MethodPost, "/api/v1/auth/register", fmt.Sprintf(regBody, "rota@example.com"), "", &reg); st != http.StatusCreated {
		t.Fatalf("registro = %d %s", st, code)
	}
	if st, _ := before.call(http.MethodGet, "/api/v1/me", "", reg.Tokens.AccessToken, nil); st != http.StatusOK {
		t.Fatalf("/me antes de rotar = %d", st)
	}

	after := newAPIOn(t, db, newSecret, 15*time.Minute) // reinicio con el secreto rotado

	// 1. El acceso viejo (aún vigente por tiempo) se rechaza de inmediato.
	if st, code := after.call(http.MethodGet, "/api/v1/me", "", reg.Tokens.AccessToken, nil); st != http.StatusUnauthorized || code != "unauthorized" {
		t.Errorf("acceso firmado con el secreto anterior = %d %s, want 401", st, code)
	}

	// 2. El refresh token emitido antes de rotar sigue valiendo.
	var ref authBody
	if st, code := after.call(http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"`+reg.Tokens.RefreshToken+`"}`, "", &ref); st != http.StatusOK || ref.Tokens.AccessToken == "" {
		t.Fatalf("refresh tras rotar = %d %s, want 200 (los refresh no dependen del secreto)", st, code)
	}
	if st, _ := after.call(http.MethodGet, "/api/v1/me", "", ref.Tokens.AccessToken, nil); st != http.StatusOK {
		t.Errorf("/me con el acceso nuevo = %d, want 200", st)
	}

	// 3. Una réplica sin reiniciar (secreto anterior) rechaza el acceso nuevo.
	if st, _ := before.call(http.MethodGet, "/api/v1/me", "", ref.Tokens.AccessToken, nil); st != http.StatusUnauthorized {
		t.Errorf("réplica con el secreto anterior aceptó un token nuevo = %d, want 401", st)
	}

	// 4. El inicio de sesión normal funciona con el secreto nuevo.
	var login authBody
	if st, _ := after.call(http.MethodPost, "/api/v1/auth/login", `{"email":"rota@example.com","password":"una-contrasena-larga"}`, "", &login); st != http.StatusOK {
		t.Fatalf("login tras rotar = %d", st)
	}
	if st, _ := after.call(http.MethodGet, "/api/v1/me", "", login.Tokens.AccessToken, nil); st != http.StatusOK {
		t.Errorf("/me tras login = %d", st)
	}

	// 5. La rotación de refresh tokens (CA-03) sigue intacta: reutilizar el
	//    refresh ya canjeado revoca la familia, también después de rotar.
	if st, _ := after.call(http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"`+reg.Tokens.RefreshToken+`"}`, "", nil); st != http.StatusUnauthorized {
		t.Errorf("reutilizar el refresh canjeado = %d, want 401", st)
	}
}
