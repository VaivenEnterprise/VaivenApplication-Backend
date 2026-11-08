// Package internal_test verifica la regla de dependencia de Clean Architecture
// (spec 002, T017; RNF-07; Art. III): domain y usecase no dependen, ni directa
// ni transitivamente, de HTTP, chi ni del driver de MongoDB (bson incluido).
package internal_test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// innerLayers son las capas que deben quedar libres de infraestructura: las
// compartidas y las de cada módulo (internal/<modulo>/{domain,usecase}, Art.
// XII.3), además de los puertos compartidos entre módulos.
var innerLayers = []string{
	"./domain/...", "./usecase/...",
	"./.../domain/...", "./.../usecase/...",
	"./users/...", "./notify/...", "./moderation/...", "./images/port/...",
}

// forbiddenPrefixes cubre net/http (y sus subpaquetes), chi y todo el driver
// de MongoDB, incluido bson.
var forbiddenPrefixes = []string{
	"net/http",
	"github.com/go-chi/",
	"go.mongodb.org/mongo-driver",
}

// forbidden devuelve las dependencias que rompen la regla, sin repetir.
func forbidden(deps []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range deps {
		for _, p := range forbiddenPrefixes {
			if (d == p || strings.HasPrefix(d, p)) && !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

func TestInnerLayersHaveNoInfrastructureDeps(t *testing.T) {
	for _, layer := range innerLayers {
		t.Run(layer, func(t *testing.T) {
			// go list -deps incluye las dependencias transitivas.
			out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", layer).Output()
			if err != nil {
				t.Fatalf("go list -deps %s: %v", layer, err)
			}
			deps := strings.Fields(string(out))
			if len(deps) == 0 {
				t.Fatalf("go list no devolvió paquetes para %s", layer)
			}
			if bad := forbidden(deps); len(bad) > 0 {
				t.Errorf("%s depende de infraestructura (Art. III): %v", layer, bad)
			}
		})
	}
}

func TestForbiddenDetectsViolations(t *testing.T) {
	deps := []string{
		"errors", "fmt", "net/url",
		"net/http", "net/http/httptest",
		"github.com/go-chi/chi/v5",
		"go.mongodb.org/mongo-driver/v2/bson",
		"go.mongodb.org/mongo-driver/v2/mongo",
	}
	want := []string{
		"net/http", "net/http/httptest",
		"github.com/go-chi/chi/v5",
		"go.mongodb.org/mongo-driver/v2/bson",
		"go.mongodb.org/mongo-driver/v2/mongo",
	}
	if got := forbidden(deps); !reflect.DeepEqual(got, want) {
		t.Errorf("forbidden = %v, want %v", got, want)
	}
	if got := forbidden([]string{"errors", "log/slog", "net/url"}); got != nil {
		t.Errorf("solo biblioteca estándar permitida, forbidden = %v", got)
	}
}
