package domain

import "testing"

// FuzzParsePayload (spec 020, RF-10 d, RF-11): el texto leído de un QR nunca
// provoca un pánico; si se acepta, trae reserva, etapa válida y token, y volver
// a armar el texto y leerlo da lo mismo.
func FuzzParsePayload(f *testing.F) {
	f.Add(Payload("6731a2b3c4d5e6f7a8b9c0d1", KindDelivery, "dG9rZW4"))
	f.Add(Payload("6731a2b3c4d5e6f7a8b9c0d1", KindReturn, "a+b/c=="))
	for _, s := range []string{"", "vaiven://qr?", "vaiven://qr?b=1&s=delivery", "vaiven://qr?b=1&s=x&t=y",
		"vaiven://qr?b=1&s=delivery&t=y&t=z", "vaiven://qr?%zz", "vaiven://qr?b=%00&s=return&t=%ff",
		"https://evil.example/qr?b=1&s=delivery&t=y", "VAIVEN://qr?b=1&s=delivery&t=y", "vaiven://qr?b=1;s=delivery;t=y"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		b, k, tok, ok := ParsePayload(s)
		if !ok {
			if b != "" || k != "" || tok != "" {
				t.Fatalf("rechazó pero devolvió datos: %q %q %q", b, k, tok)
			}
			return
		}
		if b == "" || tok == "" || !k.Valid() {
			t.Fatalf("aceptó un QR incompleto: %q %q %q", b, k, tok)
		}
		b2, k2, tok2, ok2 := ParsePayload(Payload(b, k, tok))
		if !ok2 || b2 != b || k2 != k || tok2 != tok {
			t.Fatalf("ida y vuelta inestable: (%q,%q,%q) -> (%q,%q,%q,%v)", b, k, tok, b2, k2, tok2, ok2)
		}
	})
}
