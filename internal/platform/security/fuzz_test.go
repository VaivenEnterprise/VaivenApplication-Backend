package security

import (
	"strings"
	"testing"
	"time"
)

// FuzzJWTParse (spec 020, RF-10 c, RF-11): ninguna cadena provoca un pánico y
// nadie puede entrar sin la firma: solo un token que firmó este emisor se
// acepta, y entonces trae la misma identidad con que se emitió.
func FuzzJWTParse(f *testing.F) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	j, err := NewJWT("0123456789abcdef0123456789abcdef-fuzz-secret", 15*time.Minute)
	if err != nil {
		f.Fatal(err)
	}
	j.now = func() time.Time { return now }
	valid, err := j.IssueSession("6731a2b3c4d5e6f7a8b9c0d1", "user", "sid1")
	if err != nil {
		f.Fatal(err)
	}
	parts := strings.Split(valid, ".")
	f.Add(valid)
	f.Add("")
	f.Add("a.b.c")
	f.Add("..")
	f.Add(parts[0] + "." + parts[1] + ".")
	f.Add("eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." + parts[1] + ".")
	f.Add("eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9." + parts[1] + "." + parts[2])
	f.Add(parts[0] + "." + parts[1] + "." + parts[2][:len(parts[2])-2] + "xx")
	f.Add(strings.Repeat("A", 10000))
	f.Fuzz(func(t *testing.T, token string) {
		c, err := j.Parse(token)
		if err != nil {
			return
		}
		if c.UserID != "6731a2b3c4d5e6f7a8b9c0d1" || c.Role != "user" || c.SessionID != "sid1" || c.TokenID == "" {
			t.Fatalf("aceptó un token con otra identidad: %+v", c)
		}
		if !c.ExpiresAt.After(now) {
			t.Fatalf("aceptó un token vencido: %+v", c)
		}
	})
}
