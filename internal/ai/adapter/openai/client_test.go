package openai

import (
	"context"
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

const testKey = "gsk_clave_de_prueba_groq"

type fake struct {
	calls   atomic.Int32
	last    map[string]any
	auth    string
	path    string
	respond func(n int32, w http.ResponseWriter)
}

func newFake(t *testing.T, respond func(n int32, w http.ResponseWriter)) (*fake, *httptest.Server) {
	t.Helper()
	f := &fake{respond: respond}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff\xe0 jpeg de prueba"))
			return
		}
		n := f.calls.Add(1)
		f.auth, f.path = r.Header.Get("Authorization"), r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &f.last)
		f.respond(n, w)
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func reply(content, finish string) func(int32, http.ResponseWriter) {
	return func(_ int32, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "finish_reason": finish}},
		})
	}
}

func client(t *testing.T, srv *httptest.Server, key string) *Client {
	t.Helper()
	c, err := New(Config{BaseURL: srv.URL + "/openai/v1/", Model: "llama-de-prueba", APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	c.sleep = func(context.Context, time.Duration) error { return nil }
	c.allowHTTPImages = true
	return c
}

func planReq() aidomain.Request {
	return aidomain.Request{
		System: "Instrucciones fijas",
		Turns: []aidomain.Turn{
			{Role: aidomain.RoleUser, Text: "quiero pintar"},
			{Role: aidomain.RoleModel, Text: "necesitas rodillo"},
			{Role: aidomain.RoleUser, Text: "ya tengo escalera"},
		},
		Schema: aidomain.PlanSchema(),
	}
}

func TestGenerateTextRequestAndJSONMode(t *testing.T) {
	f, srv := newFake(t, reply(`{"summary":"ok"}`, "stop"))
	out, err := client(t, srv, testKey).Generate(context.Background(), planReq())
	if err != nil || string(out) != `{"summary":"ok"}` {
		t.Fatalf("out=%s err=%v", out, err)
	}
	if f.path != "/openai/v1/chat/completions" || f.auth != "Bearer "+testKey {
		t.Errorf("ruta=%q auth=%q", f.path, f.auth)
	}
	msgs := f.last["messages"].([]any)
	sys := msgs[0].(map[string]any)
	if sys["role"] != "system" || !strings.HasPrefix(sys["content"].(string), "Instrucciones fijas") ||
		!strings.Contains(sys["content"].(string), `"needs"`) {
		t.Errorf("system = %v", sys)
	}
	roles := []string{}
	for _, m := range msgs[1:] {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	if strings.Join(roles, ",") != "user,assistant,user" {
		t.Errorf("roles = %v", roles)
	}
	if rf, _ := f.last["response_format"].(map[string]any); rf["type"] != "json_object" || f.last["model"] != "llama-de-prueba" {
		t.Errorf("modo JSON o modelo: %v", f.last)
	}
}

func TestGenerateWithImageUsesDataURLAndNoJSONMode(t *testing.T) {
	f, srv := newFake(t, reply("```json\n{\"title\":\"Taladro\",\"category\":\"herramientas_electricas\"}\n```", "stop"))
	req := aidomain.Request{
		Turns: []aidomain.Turn{{Role: aidomain.RoleUser, Text: "sugiere"}}, ImageURL: srv.URL + "/m.jpg",
		Schema: aidomain.SuggestionSchema(),
	}
	out, err := client(t, srv, testKey).Generate(context.Background(), req)
	if err != nil || string(out) != `{"title":"Taladro","category":"herramientas_electricas"}` {
		t.Fatalf("el JSON se extrae del bloque de código: %s %v", out, err)
	}
	last := f.last["messages"].([]any)[1].(map[string]any)
	parts := last["content"].([]any)
	img := parts[1].(map[string]any)["image_url"].(map[string]any)["url"].(string)
	if !strings.HasPrefix(img, "data:image/jpeg;base64,") {
		t.Errorf("imagen = %.40s", img)
	}
	if _, ok := f.last["response_format"]; ok {
		t.Error("con imagen no se pide response_format")
	}
}

func TestOllamaWithoutKeySendsNoAuthorization(t *testing.T) {
	f, srv := newFake(t, reply(`{"summary":"ok"}`, "stop"))
	if _, err := client(t, srv, "").Generate(context.Background(), planReq()); err != nil {
		t.Fatal(err)
	}
	if f.auth != "" {
		t.Errorf("sin clave no debe enviar Authorization: %q", f.auth)
	}
}

func TestRetriesAndErrors(t *testing.T) {
	f, srv := newFake(t, func(n int32, w http.ResponseWriter) {
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		reply(`{"summary":"segundo"}`, "stop")(n, w)
	})
	if out, err := client(t, srv, testKey).Generate(context.Background(), planReq()); err != nil || !strings.Contains(string(out), "segundo") || f.calls.Load() != 2 {
		t.Errorf("reintento: %s %v %d", out, err, f.calls.Load())
	}

	f, srv = newFake(t, func(_ int32, w http.ResponseWriter) { w.WriteHeader(http.StatusBadGateway) })
	if _, err := client(t, srv, testKey).Generate(context.Background(), planReq()); !errors.Is(err, aidomain.ErrUnavailable) || f.calls.Load() != 2 {
		t.Errorf("dos fallos: %v %d", err, f.calls.Load())
	}

	f, srv = newFake(t, func(_ int32, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"quiero pintar ` + testKey + `"}`))
	})
	_, err := client(t, srv, testKey).Generate(context.Background(), planReq())
	if !errors.Is(err, aidomain.ErrUnavailable) || f.calls.Load() != 1 || strings.Contains(err.Error(), "pintar") || strings.Contains(err.Error(), testKey) {
		t.Errorf("4xx sin reintento ni contenido: %v %d", err, f.calls.Load())
	}
}

func TestBlockedAndEmpty(t *testing.T) {
	cases := map[string]struct {
		h    func(int32, http.ResponseWriter)
		want error
	}{
		"filtro":       {reply("", "content_filter"), aidomain.ErrBlocked},
		"sin JSON":     {reply("no sé", "stop"), aidomain.ErrUnavailable},
		"sin opciones": {func(_ int32, w http.ResponseWriter) { _, _ = w.Write([]byte(`{"choices":[]}`)) }, aidomain.ErrUnavailable},
		"no es JSON":   {func(_ int32, w http.ResponseWriter) { _, _ = w.Write([]byte(`<html>`)) }, aidomain.ErrUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, srv := newFake(t, tc.h)
			if _, err := client(t, srv, testKey).Generate(context.Background(), planReq()); !errors.Is(err, tc.want) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestNewCancelAndValidation(t *testing.T) {
	if _, err := New(Config{BaseURL: "https://x"}); err == nil {
		t.Error("sin modelo debe fallar")
	}
	_, srv := newFake(t, reply(`{}`, "stop"))
	c := client(t, srv, testKey)
	if _, err := c.Generate(context.Background(), aidomain.Request{}); err == nil {
		t.Error("sin mensajes debe fallar")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Generate(ctx, planReq()); !errors.Is(err, aidomain.ErrUnavailable) {
		t.Errorf("cancelado: %v", err)
	}
	bad := aidomain.Request{Turns: []aidomain.Turn{{Text: "x"}}, ImageURL: "http://no-permitida/x.jpg"}
	c.allowHTTPImages = false
	if _, err := c.Generate(context.Background(), bad); err == nil {
		t.Error("una miniatura sin https debe fallar")
	}
}

func TestExtractJSON(t *testing.T) {
	for in, want := range map[string]string{
		`{"a":1}`: `{"a":1}`,
		"Aquí va:\n```json\n{\"a\":{\"b\":2}}\n```": `{"a":{"b":2}}`,
		"nada":         "",
		"} al revés {": "",
	} {
		if got := extractJSON(in); got != want {
			t.Errorf("extractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}
