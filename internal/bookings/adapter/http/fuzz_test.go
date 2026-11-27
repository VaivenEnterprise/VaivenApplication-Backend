package http

import (
	"encoding/base64"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/validate"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
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

// FuzzDecodeCreateRequest (spec 020, RF-10 b): el cuerpo de POST /bookings se
// lee y valida sin pánico; un cuerpo inválido da validation_failed y uno
// aceptado trae todos los campos obligatorios presentes y no negativos.
func FuzzDecodeCreateRequest(f *testing.F) {
	f.Add(`{"itemId":"6731a2b3c4d5e6f7a8b9c0d1","startDate":"2026-11-10","endDate":"2026-11-12","paymentMode":"split_50_50","acceptedTotalCents":9000}`)
	for _, b := range []string{``, `{}`, `[]`, `null`, `{"itemId":null}`, `{"acceptedTotalCents":-1}`, `{"acceptedTotalCents":1e400}`,
		`{"itemId":"  ","startDate":"x","endDate":"y","paymentMode":"z","acceptedTotalCents":0}`, `{} {}`, "\xff"} {
		f.Add(b)
	}
	v := validate.New()
	f.Fuzz(func(t *testing.T, body string) {
		var req createRequest
		r := httptest.NewRequest("POST", "/bookings", strings.NewReader(body))
		err := decode(r, &req, false)
		if err == nil {
			err = v.Struct(&req, domain.MessageInvalidData)
		}
		if err != nil {
			var de *shared.Error
			if !errors.As(err, &de) || de.Code != shared.CodeValidation {
				t.Fatalf("un cuerpo inválido debe dar validation_failed, no %T: %v", err, err)
			}
			return
		}
		if req.ItemID == nil || req.StartDate == nil || req.EndDate == nil || req.PaymentMode == nil ||
			req.AcceptedTotalCents == nil || *req.AcceptedTotalCents < 0 || strings.TrimSpace(*req.ItemID) == "" {
			t.Fatalf("se aceptó un cuerpo incompleto: %+v", req)
		}
	})
}
