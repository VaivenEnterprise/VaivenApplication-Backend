package security

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Spec 018, C-11: emisor y audiencia obligatorios; C-12: sesión en el token.

func TestJWTSessionRoundTrip(t *testing.T) {
	j, _ := NewJWT(testSecret, 15*time.Minute)
	tok, err := j.IssueSession("u1", "user", "fam-1")
	if err != nil {
		t.Fatal(err)
	}
	c, err := j.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.SessionID != "fam-1" || c.TokenID == "" || c.IssuedAt.IsZero() || c.UserID != "u1" {
		t.Errorf("claims = %+v", c)
	}
	// Sin sesión: el sid queda vacío y el token sigue siendo válido.
	tok, _ = j.Issue("u1", "user")
	if c, err := j.Parse(tok); err != nil || c.SessionID != "" {
		t.Errorf("sin sesión = %+v %v", c, err)
	}
}

func TestJWTRejectsWrongOrMissingIssuerAudience(t *testing.T) {
	j, _ := NewJWT(testSecret, 15*time.Minute)
	now := time.Now()
	base := func() jwt.MapClaims {
		return jwt.MapClaims{
			"sub": "u1", "role": "user", "jti": "abc", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
			"iss": Issuer, "aud": Audience,
		}
	}
	if _, err := j.Parse(sign(t, jwt.SigningMethodHS256, testSecret, base())); err != nil {
		t.Fatalf("token correcto rechazado: %v", err)
	}
	cases := map[string]func(jwt.MapClaims){
		"sin iss (tokens anteriores a C-11)": func(c jwt.MapClaims) { delete(c, "iss") },
		"sin aud":                            func(c jwt.MapClaims) { delete(c, "aud") },
		"otro emisor":                        func(c jwt.MapClaims) { c["iss"] = "otro-servicio" },
		"otra audiencia":                     func(c jwt.MapClaims) { c["aud"] = "otra-app" },
		"sin jti":                            func(c jwt.MapClaims) { delete(c, "jti") },
		"sin iat":                            func(c jwt.MapClaims) { delete(c, "iat") },
	}
	for name, mut := range cases {
		c := base()
		mut(c)
		if _, err := j.Parse(sign(t, jwt.SigningMethodHS256, testSecret, c)); err != ErrInvalidToken {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
