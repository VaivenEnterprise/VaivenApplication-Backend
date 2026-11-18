// Package contract verifica que las respuestas reales de la API cumplan el
// contrato OpenAPI 3.1 (spec 016, T008; RF-08, CA-04; Art. IV y VII).
//
// Solo lo importan las pruebas (P-03): envuelve el router o el handler de una
// prueba de integración (P-04) y, por cada petición, valida la respuesta
// contra el contrato empaquetado (internal/adapter/http/docs/openapi.yaml,
// que la CI mantiene igual a api/). No valida la petición: las pruebas mandan
// a propósito cuerpos inválidos para comprobar los 400, y esas respuestas de
// error también deben cumplir el contrato.
//
// Cobertura: si la variable CONTRACT_COVERAGE apunta a un archivo, cada
// operación ejercida («GET /api/v1/items/{id}») se anexa ahí; Missing compara
// ese archivo con todas las operaciones del contrato.
package contract

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/paths"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// CoverageEnv es la variable con la ruta del archivo de cobertura.
const CoverageEnv = "CONTRACT_COVERAGE"

// TB es la parte de testing.TB que usa el paquete.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
}

// Checker valida respuestas contra el contrato. Es seguro para uso concurrente.
type Checker struct {
	v     validator.Validator
	model *v3.Document
	mu    sync.Mutex
}

var (
	loadOnce sync.Once
	loaded   *Checker
	loadErr  error
)

// SpecPath es la ruta del contrato empaquetado, relativa a este archivo.
func SpecPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "docs", "openapi.yaml")
}

// Load lee y compila el contrato una sola vez por proceso.
func Load() (*Checker, error) {
	loadOnce.Do(func() {
		raw, err := os.ReadFile(SpecPath())
		if err != nil {
			loadErr = fmt.Errorf("contract: leer el contrato: %w", err)
			return
		}
		loaded, loadErr = New(raw)
	})
	return loaded, loadErr
}

// New compila un contrato OpenAPI 3.1.
func New(spec []byte) (*Checker, error) {
	doc, err := libopenapi.NewDocument(spec)
	if err != nil {
		return nil, fmt.Errorf("contract: documento: %w", err)
	}
	m, err := doc.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("contract: modelo: %w", err)
	}
	v, errs := validator.NewValidator(doc)
	if len(errs) > 0 {
		return nil, fmt.Errorf("contract: validador: %w", errors.Join(errs...))
	}
	return &Checker{v: v, model: &m.Model}, nil
}

// Operations devuelve todas las operaciones del contrato, ordenadas.
func (c *Checker) Operations() []string {
	var out []string
	for p, item := range c.model.Paths.PathItems.FromOldest() {
		for method := range item.GetOperations().FromOldest() {
			out = append(out, strings.ToUpper(method)+" "+p)
		}
	}
	sort.Strings(out)
	return out
}

// Check valida una respuesta ya recibida para la petición req (con la ruta
// completa, /api/v1 incluido). Devuelve la operación encontrada y los
// problemas, con texto legible.
func (c *Checker) Check(req *http.Request, res *http.Response) (string, []string) {
	c.mu.Lock() // el validador guarda cachés internas
	defer c.mu.Unlock()
	item, _, tmpl := paths.FindPath(req, c.model, nil)
	if item == nil {
		return "", []string{"la ruta no está en el contrato"}
	}
	op := req.Method + " " + tmpl
	ok, verrs := c.v.ValidateHttpResponse(req, res)
	if ok {
		return op, nil
	}
	issues := make([]string, 0, len(verrs))
	for _, e := range verrs {
		msg := e.Message
		if e.Reason != "" {
			msg += ": " + e.Reason
		}
		for _, f := range e.SchemaValidationErrors {
			msg += " [" + f.Reason + " en " + f.FieldPath + "]"
		}
		issues = append(issues, msg)
	}
	return op, issues
}

// Wrap devuelve h con la verificación de contrato: cada respuesta se valida y
// cada incumplimiento es un t.Errorf. prefix es lo que el router de la prueba
// no monta (por ejemplo "/api/v1" cuando la prueba registra el módulo en un
// chi.Router sin prefijo); vacío si la prueba usa el router real. skip son
// rutas (completas) que la prueba agrega a propósito fuera del contrato, como
// una sonda; no se validan.
func Wrap(t TB, h http.Handler, prefix string, skip ...string) http.Handler {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Errorf("contract: %v", err)
		return h
	}
	return c.Wrap(t, h, prefix, skip...)
}

// Wrap es Wrap con un Checker dado.
func (c *Checker) Wrap(t TB, h http.Handler, prefix string, skip ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if slices.Contains(skip, prefix+r.URL.Path) {
			copyResponse(w, rec)
			return
		}
		res := rec.Result()
		body := rec.Body.Bytes()

		copyResponse(w, rec)

		full := r.Clone(r.Context())
		full.URL.Path = prefix + r.URL.Path
		full.URL.RawPath = ""
		full.Body = http.NoBody
		res.Body = io.NopCloser(bytes.NewReader(body))
		op, issues := c.Check(full, res)
		if op != "" {
			record(op)
		}
		for _, is := range issues {
			t.Errorf("contrato: %s %s → %d: %s", r.Method, full.URL.Path, res.StatusCode, is)
		}
	})
}

// copyResponse entrega al cliente de la prueba la respuesta grabada, intacta.
func copyResponse(w http.ResponseWriter, rec *httptest.ResponseRecorder) {
	for k, vs := range rec.Result().Header {
		w.Header()[k] = vs
	}
	w.WriteHeader(rec.Code)
	_, _ = w.Write(rec.Body.Bytes())
}

var recordMu sync.Mutex

// record anexa la operación al archivo de cobertura, si está configurado.
func record(op string) {
	path := os.Getenv(CoverageEnv)
	if path == "" {
		return
	}
	recordMu.Lock()
	defer recordMu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 G703 -- ruta elegida por quien corre las pruebas (solo pruebas)
	if err != nil {
		return
	}
	_, _ = f.WriteString(op + "\n")
	_ = f.Close()
}

// Missing lee un archivo de cobertura y devuelve las operaciones del
// contrato que ninguna prueba ejerció, y cuántas sí.
func (c *Checker) Missing(coverageFile string) (missing []string, covered int, err error) {
	raw, err := os.ReadFile(coverageFile) // #nosec G304 G703 -- ruta elegida por quien corre las pruebas (solo pruebas)
	if err != nil {
		return nil, 0, err
	}
	hit := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r", ""), "\n") {
		if line != "" {
			hit[line] = true
		}
	}
	for _, op := range c.Operations() {
		if hit[op] {
			covered++
		} else {
			missing = append(missing, op)
		}
	}
	return missing, covered, nil
}
