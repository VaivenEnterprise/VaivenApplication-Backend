package http

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
)

var fuzzAt = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

// FuzzDecodeCursor (spec 020, RF-10 a, RF-11): un cursor hostil nunca provoca
// un pánico; si se acepta, su id es un ObjectID válido y cabe en el límite, y
// codificarlo y decodificarlo devuelve el mismo cursor.
func FuzzDecodeCursor(f *testing.F) {
	valid := encodeCursor(&domain.Cursor{CreatedAt: fuzzAt, ID: "6731a2b3c4d5e6f7a8b9c0d1"})
	f.Add(*valid)
	f.Add("")
	f.Add("!!!")
	f.Add(base64.RawURLEncoding.EncodeToString([]byte("0.6731a2b3c4d5e6f7a8b9c0d1")))
	f.Add(base64.RawURLEncoding.EncodeToString([]byte("-1.6731a2b3c4d5e6f7a8b9c0d1")))
	f.Add(base64.RawURLEncoding.EncodeToString([]byte("9223372036854775807.6731a2b3c4d5e6f7a8b9c0d1")))
	f.Add(base64.RawURLEncoding.EncodeToString([]byte("1.2.6731a2b3c4d5e6f7a8b9c0d1")))
	f.Add(base64.RawURLEncoding.EncodeToString([]byte("1.ZZZZZZZZZZZZZZZZZZZZZZZZ")))
	f.Add(base64.RawURLEncoding.EncodeToString([]byte("+5.6731a2b3c4d5e6f7a8b9c0d1")))
	f.Fuzz(func(t *testing.T, s string) {
		c, err := decodeCursor(s)
		if err != nil {
			return
		}
		if len(s) > 128 {
			t.Fatalf("aceptó un cursor de %d caracteres", len(s))
		}
		if !isObjectIDHex(c.ID) {
			t.Fatalf("aceptó un id inválido: %q", c.ID)
		}
		again, err := decodeCursor(*encodeCursor(&c))
		if err != nil || again != c {
			t.Fatalf("ida y vuelta inestable: %v -> %v (%v)", c, again, err)
		}
	})
}
