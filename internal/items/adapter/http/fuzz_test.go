package http

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
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

func isValidation(t *testing.T, err error) {
	t.Helper()
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeValidation {
		t.Fatalf("un cuerpo inválido debe dar validation_failed, no %T: %v", err, err)
	}
}

var fuzzBodies = []string{
	`{"title":"Taladro","description":"x","category":"hogar","condition":"good","modality":"rent","declaredValueCents":100000,"pricePerDayCents":3000,"priceSource":"manual","zone":{"municipality":"Coyoacán"},"photos":[{"publicId":"p1"}]}`,
	`{}`, `[]`, `null`, ``, `{"title":null}`, `{"title":1}`, `{"photos":[]}`, `{"photos":[null]}`, `{"zone":null}`,
	`{"title":"a"}{"title":"b"}`, `{"declaredValueCents":1e999}`, `{"version":null}`, `{"version":1,"title":null}`,
	`{"approxLocation":{"lat":"x"}}`, "\xff\xfe", `{"photos":[{"publicId":"` + strings.Repeat("a", 1000) + `"}]}`,
}

// FuzzDecodeCreateRequest (spec 020, RF-10 b): el decodificador de cuerpos nunca
// entra en pánico; un cuerpo inválido da validation_failed; uno aceptado se
// puede convertir en entrada del caso de uso sin desreferenciar nada nulo.
func FuzzDecodeCreateRequest(f *testing.F) {
	for _, b := range fuzzBodies {
		f.Add(b)
	}
	d := newDecoder()
	f.Fuzz(func(t *testing.T, body string) {
		var req createRequest
		if err := d.decode(strings.NewReader(body), &req); err != nil {
			isValidation(t, err)
			return
		}
		_ = req.input() // no debe entrar en pánico si la validación pasó
	})
}

// FuzzDecodeUpdateRequest es lo mismo para PATCH /items/{id}.
func FuzzDecodeUpdateRequest(f *testing.F) {
	for _, b := range fuzzBodies {
		f.Add(b)
	}
	d := newDecoder()
	f.Fuzz(func(t *testing.T, body string) {
		var req updateRequest
		if err := d.decode(strings.NewReader(body), &req); err != nil {
			isValidation(t, err)
			return
		}
		_ = req.empty()
		_ = req.nullDetails()
		_, _ = req.input()
	})
}
