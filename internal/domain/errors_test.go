package domain

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConstructorsSetCode(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		code Code
	}{
		{"validación", NewValidation("datos inválidos"), CodeValidation},
		{"no encontrado", NewNotFound("objeto no encontrado"), CodeNotFound},
		{"conflicto", NewConflict("ya existe"), CodeConflict},
		{"no autenticado", NewUnauthorized("inicia sesión"), CodeUnauthorized},
		{"prohibido", NewForbidden("sin permiso"), CodeForbidden},
		{"no disponible", NewUnavailable("intenta más tarde"), CodeUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.code {
				t.Errorf("Code = %q, want %q", tt.err.Code, tt.code)
			}
			if tt.err.Message == "" {
				t.Error("Message vacío")
			}
			if want := string(tt.code) + ": " + tt.err.Message; tt.err.Error() != want {
				t.Errorf("Error() = %q, want %q", tt.err.Error(), want)
			}
		})
	}
}

func TestCodesMatchAPIContract(t *testing.T) {
	// Art. IV.3: los códigos de dominio son un subconjunto de los de la API.
	want := map[Code]string{
		CodeValidation:   "validation_failed",
		CodeNotFound:     "not_found",
		CodeConflict:     "conflict",
		CodeUnauthorized: "unauthorized",
		CodeForbidden:    "forbidden",
		CodeUnavailable:  "service_unavailable",
	}
	for code, s := range want {
		if string(code) != s {
			t.Errorf("código %q, el contrato dice %q", code, s)
		}
	}
}

func TestValidationDetails(t *testing.T) {
	err := NewValidation("revisa los campos",
		Detail{Field: "email", Issue: "formato inválido"},
		Detail{Field: "name", Issue: "obligatorio"},
	)
	if len(err.Details) != 2 || err.Details[0].Field != "email" || err.Details[1].Issue != "obligatorio" {
		t.Errorf("Details = %+v", err.Details)
	}
	if NewNotFound("x").Details != nil {
		t.Error("un error sin detalles debe tener Details nil")
	}
}

func TestErrorsAsThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("caso de uso: %w", NewConflict("el correo ya está registrado"))

	var derr *Error
	if !errors.As(wrapped, &derr) {
		t.Fatal("errors.As no encontró *domain.Error")
	}
	if derr.Code != CodeConflict {
		t.Errorf("Code = %q, want %q", derr.Code, CodeConflict)
	}
}

func TestWrapKeepsCause(t *testing.T) {
	cause := errors.New("duplicate key")
	base := NewConflict("ya existe")
	err := base.Wrap(cause)

	if !errors.Is(err, cause) {
		t.Error("errors.Is no llega a la causa")
	}
	if errors.Unwrap(err) != cause {
		t.Error("Unwrap no devuelve la causa")
	}
	if base.Err != nil {
		t.Error("Wrap no debe modificar el error original")
	}
	if err.Error() != base.Error() {
		t.Errorf("Error() = %q; la causa no debe filtrarse al mensaje", err.Error())
	}
	if NewNotFound("x").Unwrap() != nil {
		t.Error("sin causa, Unwrap debe devolver nil")
	}
}

// TestOnlyStdlibImports aplica el Art. III: domain no importa nada fuera de
// la biblioteca estándar (ni drivers, ni HTTP, ni módulos del proyecto).
func TestOnlyStdlibImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(fset, f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			first := strings.SplitN(path, "/", 2)[0]
			if strings.Contains(first, ".") {
				t.Errorf("%s importa %q, que no es de la biblioteca estándar", f, path)
			}
		}
	}
}
