package cloudinary

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const testSecret = "secreto-de-prueba"

func TestSignKnownVector(t *testing.T) {
	// Vector de la documentación de Cloudinary (Authentication signatures):
	// cadena «public_id=sample_image&timestamp=1315060510abcd».
	got := Sign(map[string]string{"public_id": "sample_image", "timestamp": "1315060510"}, "abcd")
	if want := "e3c44b54e67a3ecc918f5d7236ca5faa36250ea8a8cd6cbabfd2d6bb2453acac"; got != want {
		t.Fatalf("Sign = %s, want %s", got, want)
	}
}

func TestSignOrderAndEmptyValues(t *testing.T) {
	a := Sign(map[string]string{"timestamp": "1315060510", "public_id": "sample_image", "folder": ""}, "abcd")
	b := Sign(map[string]string{"public_id": "sample_image", "timestamp": "1315060510"}, "abcd")
	if a != b {
		t.Fatalf("los valores vacíos deben omitirse y el orden no debe importar: %s != %s", a, b)
	}
	if Sign(map[string]string{"public_id": "x"}, "abcd") == Sign(map[string]string{"public_id": "y"}, "abcd") {
		t.Fatal("firmas iguales para parámetros distintos")
	}
}

func TestNewSignerErrors(t *testing.T) {
	cases := []struct{ cloud, key, secret string }{
		{"", "clave", testSecret},
		{"nube", "", testSecret},
		{"nube", "clave", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		s, err := NewSigner(c.cloud, c.key, c.secret)
		if err == nil || s != nil {
			t.Fatalf("NewSigner(%q,%q,secreto) debe fallar", c.cloud, c.key)
		}
		if !errors.Is(err, errMissingConfig) {
			t.Fatalf("err = %v, want errMissingConfig", err)
		}
		if c.secret != "" && strings.Contains(err.Error(), c.secret) {
			t.Fatalf("el error contiene el secreto: %v", err)
		}
	}
}

func TestSigner(t *testing.T) {
	s, err := NewSigner("nube-demo", "clave-publica", testSecret)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := s.UploadURL(), "https://api.cloudinary.com/v1_1/nube-demo/image/upload"; got != want {
		t.Fatalf("UploadURL = %s, want %s", got, want)
	}
	if s.APIKey() != "clave-publica" {
		t.Fatalf("APIKey = %s", s.APIKey())
	}
	p := map[string]string{"public_id": "a/b", "timestamp": "1"}
	if s.SignUpload(p) != Sign(p, testSecret) {
		t.Fatal("SignUpload debe usar el secreto del Signer")
	}
}

func TestSignerDoesNotPrintSecret(t *testing.T) {
	s, err := NewSigner("nube-demo", "clave-publica", testSecret)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"%v", "%+v", "%#v", "%s"} {
		out := fmt.Sprintf(f, s)
		if strings.Contains(out, testSecret) {
			t.Fatalf("%s muestra el secreto: %s", f, out)
		}
		if !strings.Contains(out, "REDACTED") {
			t.Fatalf("%s no usa String/GoString: %s", f, out)
		}
	}
}
