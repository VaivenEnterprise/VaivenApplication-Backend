package http

import (
	"errors"
	"strings"
	"testing"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

var authFuzzBodies = []string{
	`{"email":"ana@example.com","password":"Contrasena-larga-1","displayName":"Ana","acceptTerms":true,"isAdult":true}`,
	`{}`, `[]`, `null`, ``, `{"email":null}`, `{"email":1}`, `{"email":"a"}{"email":"b"}`,
	`{"phone":null}`, `{"phone":"+525555555555"}`, `{"phone":5}`, `{"displayName":null,"phone":"x"}`,
	`{"refreshToken":"` + strings.Repeat("a", 200) + `"}`, "\xff\xfe", `{"password":"` + strings.Repeat("ñ", 100) + `"}`,
}

// FuzzDecodeAuthBodies (spec 020, RF-10 b): los decodificadores de cuerpos de
// cuenta nunca entran en pánico; un cuerpo inválido da validation_failed y las
// conversiones de un cuerpo aceptado tampoco fallan.
func FuzzDecodeAuthBodies(f *testing.F) {
	for _, b := range authFuzzBodies {
		f.Add(b)
	}
	d := newDecoder()
	f.Fuzz(func(t *testing.T, body string) {
		check := func(err error) bool {
			if err == nil {
				return true
			}
			var de *shared.Error
			if !errors.As(err, &de) || de.Code != shared.CodeValidation {
				t.Fatalf("un cuerpo inválido debe dar validation_failed, no %T: %v", err, err)
			}
			return false
		}
		var reg registerRequest
		check(d.decode(strings.NewReader(body), &reg))
		var login loginRequest
		check(d.decode(strings.NewReader(body), &login))
		var refresh refreshRequest
		check(d.decode(strings.NewReader(body), &refresh))
		var del deleteMeRequest
		check(d.decode(strings.NewReader(body), &del))
		var upd updateMeRequest
		if check(d.decode(strings.NewReader(body), &upd)) {
			_, err := upd.changes()
			check(err)
		}
	})
}
