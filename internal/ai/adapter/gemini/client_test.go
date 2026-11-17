package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
)

const testKey = "clave-de-prueba-gemini"

// fakeGemini simula la API. respond decide la respuesta de cada intento.
type fakeGemini struct {
	calls   atomic.Int32
	last    generateRequest
	path    string
	keyHdr  string
	query   string
	respond func(n int32, w http.ResponseWriter)
}

func newFake(t *testing.T, respond func(n int32, w http.ResponseWriter)) (*fakeGemini, *httptest.Server) {
	t.Helper()
	f := &fakeGemini{respond: respond}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { // miniatura
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff\xe0 jpeg de prueba \xff\xd9"))
			return
		}
		n := f.calls.Add(1)
		f.path, f.keyHdr, f.query = r.URL.Path, r.Header.Get("x-goog-api-key"), r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &f.last)
		f.respond(n, w)
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func okText(text string) func(int32, http.ResponseWriter) {
	return func(_ int32, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []any{map[string]any{
				"content":      map[string]any{"parts": []any{map[string]any{"text": text}}},
				"finishReason": "STOP",
			}},
		})
	}
}

func newClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(Config{BaseURL: srv.URL + "/", Model: "gemini-test", APIKey: testKey})
	if err != nil {
		t.Fatal(err)
	}
	c.sleep = func(context.Context, time.Duration) error { return nil }
	c.allowHTTPImages = true
	return c
}

func planRequest() aidomain.Request {
	return aidomain.Request{
		System: "Instrucciones fijas",
		Turns: []aidomain.Turn{
			{Role: aidomain.RoleUser, Text: "quiero pintar mi cuarto"},
			{Role: aidomain.RoleModel, Text: `{"summary":"..."}`},
			{Role: aidomain.RoleUser, Text: "ya tengo escalera"},
		},
		Schema: aidomain.PlanSchema(),
	}
}

func TestGenerateSendsRequestAndReturnsJSON(t *testing.T) {
	f, srv := newFake(t, okText(`{"summary":"ok"}`))
	out, err := newClient(t, srv).Generate(context.Background(), planRequest())
	if err != nil || string(out) != `{"summary":"ok"}` {
		t.Fatalf("out=%s err=%v", out, err)
	}
	if f.path != "/v1beta/models/gemini-test:generateContent" {
		t.Errorf("ruta = %q", f.path)
	}
	if f.keyHdr != testKey || strings.Contains(f.query, testKey) {
		t.Errorf("la clave debe ir en la cabecera y nunca en la URL: hdr=%q query=%q", f.keyHdr, f.query)
	}
	if f.last.SystemInstruction == nil || f.last.SystemInstruction.Parts[0].Text != "Instrucciones fijas" {
		t.Errorf("instrucciones = %+v", f.last.SystemInstruction)
	}
	if len(f.last.Contents) != 3 || f.last.Contents[0].Role != "user" || f.last.Contents[1].Role != "model" {
		t.Errorf("turnos = %+v", f.last.Contents)
	}
	gc := f.last.GenerationConfig
	if gc.ResponseMIMEType != "application/json" || gc.ResponseSchema == nil || gc.ResponseSchema.Properties["needs"] == nil {
		t.Errorf("configuración de salida = %+v", gc)
	}
	if enum := gc.ResponseSchema.Properties["needs"].Items.Properties["category"].Enum; len(enum) != 7 {
		t.Errorf("las categorías del esquema deben ser las 7 de POL-14: %v", enum)
	}
}

func TestGenerateAttachesImageToLastTurn(t *testing.T) {
	f, srv := newFake(t, okText(`{"title":"Taladro","category":"herramientas_electricas"}`))
	req := aidomain.Request{
		Turns:    []aidomain.Turn{{Role: aidomain.RoleUser, Text: "sugiere"}},
		ImageURL: srv.URL + "/miniatura.jpg",
		Schema:   aidomain.SuggestionSchema(),
	}
	if _, err := newClient(t, srv).Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	parts := f.last.Contents[0].Parts
	if len(parts) != 2 || parts[1].InlineData == nil || parts[1].InlineData.MIMEType != "image/jpeg" {
		t.Fatalf("partes = %+v", parts)
	}
	if data, _ := base64.StdEncoding.DecodeString(parts[1].InlineData.Data); !strings.Contains(string(data), "jpeg de prueba") {
		t.Error("la imagen no viajó en base64")
	}
}

func TestGenerateRejectsNonHTTPSImageInProduction(t *testing.T) {
	_, srv := newFake(t, okText(`{}`))
	c := newClient(t, srv)
	c.allowHTTPImages = false
	req := aidomain.Request{Turns: []aidomain.Turn{{Text: "x"}}, ImageURL: srv.URL + "/a.jpg"}
	if _, err := c.Generate(context.Background(), req); err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("err = %v", err)
	}
}

func TestGenerateRetriesOnceOn503(t *testing.T) {
	f, srv := newFake(t, func(n int32, w http.ResponseWriter) {
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		okText(`{"summary":"segundo intento"}`)(n, w)
	})
	out, err := newClient(t, srv).Generate(context.Background(), planRequest())
	if err != nil || !strings.Contains(string(out), "segundo intento") || f.calls.Load() != 2 {
		t.Fatalf("out=%s err=%v intentos=%d", out, err, f.calls.Load())
	}
}

func TestGenerateGivesUpAfterSecondFailure(t *testing.T) {
	f, srv := newFake(t, func(_ int32, w http.ResponseWriter) { w.WriteHeader(http.StatusTooManyRequests) })
	if _, err := newClient(t, srv).Generate(context.Background(), planRequest()); !errors.Is(err, aidomain.ErrUnavailable) || f.calls.Load() != 2 {
		t.Errorf("err=%v intentos=%d", err, f.calls.Load())
	}
}

func TestGenerateDoesNotRetryClientErrorsNorLeakBody(t *testing.T) {
	f, srv := newFake(t, func(_ int32, w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"quiero pintar mi cuarto ` + testKey + `"}}`))
	})
	_, err := newClient(t, srv).Generate(context.Background(), planRequest())
	if !errors.Is(err, aidomain.ErrUnavailable) || f.calls.Load() != 1 {
		t.Fatalf("err=%v intentos=%d", err, f.calls.Load())
	}
	if strings.Contains(err.Error(), "pintar") || strings.Contains(err.Error(), testKey) {
		t.Errorf("el error filtra contenido: %v", err)
	}
}

func TestGenerateBlockedAndEmpty(t *testing.T) {
	cases := map[string]struct {
		body string
		want error
	}{
		"bloqueo de la petición": {`{"promptFeedback":{"blockReason":"SAFETY"}}`, aidomain.ErrBlocked},
		"respuesta bloqueada":    {`{"candidates":[{"finishReason":"SAFETY","content":{"parts":[]}}]}`, aidomain.ErrBlocked},
		"sin candidatos":         {`{"candidates":[]}`, aidomain.ErrUnavailable},
		"texto vacío":            {`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"  "}]}}]}`, aidomain.ErrUnavailable},
		"no es JSON":             {`<html>`, aidomain.ErrUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, srv := newFake(t, func(_ int32, w http.ResponseWriter) { _, _ = w.Write([]byte(tc.body)) })
			if _, err := newClient(t, srv).Generate(context.Background(), planRequest()); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, se esperaba %v", err, tc.want)
			}
		})
	}
}

func TestGenerateRespectsCanceledContext(t *testing.T) {
	_, srv := newFake(t, okText(`{}`))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newClient(t, srv).Generate(ctx, planRequest()); !errors.Is(err, aidomain.ErrUnavailable) {
		t.Errorf("err = %v", err)
	}
}

func TestNewAndEmptyRequest(t *testing.T) {
	if _, err := New(Config{BaseURL: "https://x", Model: "m"}); err == nil {
		t.Error("sin clave debe fallar")
	}
	_, srv := newFake(t, okText(`{}`))
	if _, err := newClient(t, srv).Generate(context.Background(), aidomain.Request{}); err == nil {
		t.Error("sin mensajes debe fallar")
	}
}

func TestClientNeverPrintsKey(t *testing.T) {
	c, _ := New(Config{BaseURL: "https://x", Model: "m", APIKey: testKey})
	_ = c
	// La clave solo se usa en la cabecera; Config no se registra en ningún lado.
	// Esta prueba documenta la regla: ningún mensaje de error de New la incluye.
	if _, err := New(Config{APIKey: testKey}); err == nil || strings.Contains(err.Error(), testKey) {
		t.Errorf("err = %v", err)
	}
}
