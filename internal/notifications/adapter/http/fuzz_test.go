package http

import (
	"encoding/base64"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
)

// FuzzDecodeCursor (spec 020, RF-10 a, RF-11): el cursor de avisos es el id
// codificado; si se acepta, es un ObjectID válido y la ida y vuelta es estable.
func FuzzDecodeCursor(f *testing.F) {
	id := "6731a2b3c4d5e6f7a8b9c0d1"
	f.Add(*encodeCursor(&id))
	f.Add("")
	f.Add("!!!")
	f.Add(base64.RawURLEncoding.EncodeToString([]byte("6731A2B3C4D5E6F7A8B9C0D1")))
	f.Add(base64.RawURLEncoding.EncodeToString([]byte("6731a2b3c4d5e6f7a8b9c0d1\x00")))
	f.Add(base64.RawURLEncoding.EncodeToString([]byte("../../etc/passwd")))
	f.Fuzz(func(t *testing.T, s string) {
		got, err := decodeCursor(s)
		if err != nil {
			return
		}
		if len(s) > 64 {
			t.Fatalf("aceptó un cursor de %d caracteres", len(s))
		}
		if !domain.IsObjectID(got) {
			t.Fatalf("aceptó un id inválido: %q", got)
		}
		again, err := decodeCursor(*encodeCursor(&got))
		if err != nil || again != got {
			t.Fatalf("ida y vuelta inestable: %q -> %q (%v)", got, again, err)
		}
	})
}
