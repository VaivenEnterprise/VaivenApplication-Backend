package contract

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTB junta los errores en lugar de fallar la prueba.
type fakeTB struct{ errs []string }

func (f *fakeTB) Helper() {}
func (f *fakeTB) Errorf(format string, args ...any) {
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}

func jsonHandler(status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func serve(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestLoadRealContract(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ops := c.Operations()
	if len(ops) < 62 { // 62 rutas en el contrato 0.11.0; varias con más de un método
		t.Fatalf("operaciones = %d", len(ops))
	}
	for _, want := range []string{"GET /healthz", "POST /api/v1/bookings", "POST /api/v1/bookings/{id}/meeting-point"} {
		found := false
		for _, op := range ops {
			found = found || op == want
		}
		if !found {
			t.Errorf("falta %q en %v", want, ops)
		}
	}
}

func TestWrapAcceptsConformingResponse(t *testing.T) {
	tb := &fakeTB{}
	rec := serve(t, Wrap(tb, jsonHandler(200, `{"status":"ok"}`), ""), http.MethodGet, "/healthz")
	if len(tb.errs) != 0 {
		t.Fatalf("errores: %v", tb.errs)
	}
	// La respuesta llega intacta al cliente de la prueba.
	if rec.Code != 200 || rec.Body.String() != `{"status":"ok"}` || rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("respuesta alterada: %d %q", rec.Code, rec.Body.String())
	}
}

func TestWrapReportsSchemaViolation(t *testing.T) {
	tb := &fakeTB{}
	serve(t, Wrap(tb, jsonHandler(200, `{"status":"tal vez"}`), ""), http.MethodGet, "/healthz")
	if len(tb.errs) == 0 || !strings.Contains(tb.errs[0], "GET /healthz → 200") {
		t.Fatalf("se esperaba un incumplimiento, errores: %v", tb.errs)
	}
}

func TestWrapReportsUndocumentedStatusAndPath(t *testing.T) {
	tb := &fakeTB{}
	serve(t, Wrap(tb, jsonHandler(418, `{}`), ""), http.MethodGet, "/healthz")
	serve(t, Wrap(tb, jsonHandler(200, `{}`), ""), http.MethodGet, "/no-existe")
	if len(tb.errs) < 2 {
		t.Fatalf("errores = %v", tb.errs)
	}
	if !strings.Contains(strings.Join(tb.errs, "\n"), "la ruta no está en el contrato") {
		t.Errorf("sin aviso de ruta: %v", tb.errs)
	}
}

func TestWrapPrefixAndCoverage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cov.txt")
	t.Setenv(CoverageEnv, file)
	tb := &fakeTB{}
	body := `{"error":{"code":"not_found","message":"No encontramos lo que buscas.","requestId":"r1"}}`
	// El router de la prueba no monta /api/v1: el prefijo lo completa.
	serve(t, Wrap(tb, jsonHandler(404, body), "/api/v1"), http.MethodGet, "/incidents/abc")
	if len(tb.errs) != 0 {
		t.Fatalf("errores: %v", tb.errs)
	}
	c, _ := Load()
	missing, covered, err := c.Missing(file)
	if err != nil || covered != 1 || len(missing) != len(c.Operations())-1 {
		t.Fatalf("Missing = %d faltantes, %d cubiertas, %v", len(missing), covered, err)
	}
	if _, _, err := c.Missing(filepath.Join(t.TempDir(), "nada.txt")); err == nil {
		t.Error("archivo inexistente sin error")
	}
}

func TestNewRejectsBadDocuments(t *testing.T) {
	if _, err := New([]byte("no es yaml: [")); err == nil {
		t.Error("documento inválido aceptado")
	}
	if _, err := New(nil); err == nil {
		t.Error("documento vacío aceptado")
	}
}

func TestRecordWithoutEnvIsNoop(t *testing.T) {
	t.Setenv(CoverageEnv, "")
	record("GET /healthz") // no escribe nada ni falla
	t.Setenv(CoverageEnv, filepath.Join(t.TempDir(), "no", "existe", "cov.txt"))
	record("GET /healthz") // directorio inexistente: se ignora
	if _, err := os.Stat(os.Getenv(CoverageEnv)); err == nil {
		t.Error("se creó un archivo en un directorio inexistente")
	}
}

// TestCoverageReport resume la cobertura de una corrida completa. Solo corre
// si CONTRACT_COVERAGE_REPORT apunta al archivo que llenaron las pruebas de
// integración (ver docs/calidad/pruebas.md).
func TestCoverageReport(t *testing.T) {
	file := os.Getenv("CONTRACT_COVERAGE_REPORT")
	if file == "" {
		t.Skip("sin CONTRACT_COVERAGE_REPORT")
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	missing, covered, err := c.Missing(file)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("operaciones ejercidas: %d de %d", covered, covered+len(missing))
	for _, op := range missing {
		t.Logf("sin ejercer: %s", op)
	}
}
