package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
)

// Pruebas de la T018 (spec 014): razonamiento, reintento con tiempo y causas.

func causeOf(t *testing.T, err error) (aidomain.Cause, int) {
	t.Helper()
	c, s, ok := aidomain.CauseOf(err)
	if !ok {
		t.Fatalf("el error no tiene causa: %v", err)
	}
	return c, s
}

func TestReasoningOnlyWhenConfigured(t *testing.T) {
	cases := []struct {
		name    string
		disable bool
		want    bool
	}{
		{"sin configurar (Groq, asistente)", false, false},
		{"desactivado (OpenRouter)", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, srv := newFake(t, reply(`{"summary":"ok"}`, "stop"))
			c := client(t, srv, testKey)
			c.cfg.DisableReasoning = tc.disable
			if _, err := c.Generate(context.Background(), planReq()); err != nil {
				t.Fatal(err)
			}
			r, has := f.last["reasoning"].(map[string]any)
			if has != tc.want {
				t.Fatalf("reasoning presente = %v", has)
			}
			if tc.want && (r["enabled"] != false || r["exclude"] != true) {
				t.Errorf("reasoning = %v", r)
			}
		})
	}
}

func TestRequestJSONUnchangedWithoutReasoning(t *testing.T) {
	_, srv := newFake(t, reply(`{}`, "stop"))
	body, err := client(t, srv, testKey).buildBody(context.Background(), planReq())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(m))
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != "max_tokens,messages,model,response_format,temperature" {
		t.Errorf("campos del JSON para Groq = %v", got)
	}
}

func TestEmptyContentRetriesOnceThenTypedError(t *testing.T) {
	cases := []struct {
		name    string
		h       func(int32, http.ResponseWriter)
		calls   int32
		wantErr aidomain.Cause // vacío = éxito
	}{
		{"vacío y luego bien", func(n int32, w http.ResponseWriter) {
			if n == 1 {
				reply("", "stop")(n, w)
				return
			}
			reply(`{"title":"x"}`, "stop")(n, w)
		}, 2, ""},
		{"truncado y luego bien", func(n int32, w http.ResponseWriter) {
			if n == 1 {
				reply("  \n", "length")(n, w)
				return
			}
			reply(`{"title":"x"}`, "stop")(n, w)
		}, 2, ""},
		{"vacío dos veces", reply("", "stop"), 2, aidomain.CauseEmptyResponse},
		{"truncado dos veces", reply(`{"title":`, "length"), 2, aidomain.CauseEmptyResponse},
		{"texto sin JSON no se reintenta", reply("no sé", "stop"), 1, aidomain.CauseMalformedJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, srv := newFake(t, tc.h)
			_, err := client(t, srv, testKey).Generate(context.Background(), planReq())
			if f.calls.Load() != tc.calls {
				t.Errorf("llamadas = %d, esperadas %d", f.calls.Load(), tc.calls)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, aidomain.ErrUnavailable) {
				t.Fatalf("err = %v", err)
			}
			if cause, _ := causeOf(t, err); cause != tc.wantErr {
				t.Errorf("causa = %v", cause)
			}
		})
	}
}

func TestNoRetryWithoutTimeLeft(t *testing.T) {
	for name, h := range map[string]func(int32, http.ResponseWriter){
		"429":   func(_ int32, w http.ResponseWriter) { w.WriteHeader(http.StatusTooManyRequests) },
		"vacío": reply("", "stop"),
	} {
		t.Run(name, func(t *testing.T) {
			f, srv := newFake(t, h)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second) // menos que minRetryBudget
			defer cancel()
			if _, err := client(t, srv, testKey).Generate(ctx, planReq()); !errors.Is(err, aidomain.ErrUnavailable) {
				t.Fatalf("err = %v", err)
			}
			if f.calls.Load() != 1 {
				t.Errorf("sin tiempo no se reintenta: %d llamadas", f.calls.Load())
			}
		})
	}
	// Con tiempo de sobra sí se reintenta.
	f, srv := newFake(t, func(_ int32, w http.ResponseWriter) { w.WriteHeader(http.StatusTooManyRequests) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, _ = client(t, srv, testKey).Generate(ctx, planReq())
	if f.calls.Load() != 2 {
		t.Errorf("con tiempo se reintenta: %d llamadas", f.calls.Load())
	}
}

func TestErrorCauses(t *testing.T) {
	cases := []struct {
		name      string
		h         func(int32, http.ResponseWriter)
		cause     aidomain.Cause
		status    int
		wantBlock bool
		noLeak    string
	}{
		{"429", func(_ int32, w http.ResponseWriter) { w.WriteHeader(429) }, aidomain.CauseHTTPStatus, 429, false, ""},
		{"503", func(_ int32, w http.ResponseWriter) { w.WriteHeader(503) }, aidomain.CauseHTTPStatus, 503, false, ""},
		{"401", func(_ int32, w http.ResponseWriter) {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"secreto-del-cuerpo"}`))
		}, aidomain.CauseHTTPStatus, 401, false, "secreto-del-cuerpo"},
		{"filtro", reply("", "content_filter"), aidomain.CauseContentFilter, 0, true, ""},
		{"sin opciones", func(_ int32, w http.ResponseWriter) { _, _ = w.Write([]byte(`{"choices":[]}`)) }, aidomain.CauseMalformedJSON, 0, false, ""},
		{"no es JSON", func(_ int32, w http.ResponseWriter) { _, _ = w.Write([]byte(`<html>`)) }, aidomain.CauseMalformedJSON, 0, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newFake(t, tc.h)
			_, err := client(t, srv, testKey).Generate(context.Background(), planReq())
			cause, status := causeOf(t, err)
			if cause != tc.cause || status != tc.status {
				t.Errorf("causa = %v %d", cause, status)
			}
			if tc.wantBlock != errors.Is(err, aidomain.ErrBlocked) {
				t.Errorf("ErrBlocked esperado = %v", tc.wantBlock)
			}
			if tc.noLeak != "" && strings.Contains(err.Error(), tc.noLeak) {
				t.Errorf("el error filtra el cuerpo: %v", err)
			}
		})
	}
}

func TestTimeoutAndNetworkCauses(t *testing.T) {
	// El servidor tarda más que el contexto: causa timeout.
	_, srv := newFake(t, func(_ int32, w http.ResponseWriter) {
		time.Sleep(300 * time.Millisecond)
		reply(`{}`, "stop")(0, w)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := client(t, srv, testKey).Generate(ctx, planReq())
	if cause, _ := causeOf(t, err); cause != aidomain.CauseTimeout {
		t.Errorf("causa = %v", cause)
	}
	// Servidor cerrado: fallo de red.
	_, srv2 := newFake(t, reply(`{}`, "stop"))
	c := client(t, srv2, testKey)
	srv2.Close()
	_, err = c.Generate(context.Background(), planReq())
	if cause, _ := causeOf(t, err); cause != aidomain.CauseNetwork {
		t.Errorf("causa = %v", cause)
	}
}

func TestImageFetchCause(t *testing.T) {
	_, srv := newFake(t, reply(`{}`, "stop"))
	c := client(t, srv, testKey)
	c.allowHTTPImages = false
	_, err := c.Generate(context.Background(), aidomain.Request{
		Turns: []aidomain.Turn{{Text: "x"}}, ImageURL: "http://no-permitida/x.jpg"})
	if cause, _ := causeOf(t, err); cause != aidomain.CauseImageFetch {
		t.Errorf("causa = %v", cause)
	}
}

func TestHTTPTimeoutNotShorterThanBudget(t *testing.T) {
	cases := []struct {
		name   string
		budget time.Duration
	}{
		{"sin presupuesto", 0},
		{"asistente (12 s)", 12 * time.Second},
		{"sugerencias por defecto (25 s)", 25 * time.Second},
		{"sugerencias al máximo (60 s)", 60 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(Config{BaseURL: "https://x", Model: "m", Timeout: tc.budget})
			if err != nil {
				t.Fatal(err)
			}
			if c.http.Timeout < tc.budget || c.http.Timeout < httpTimeout {
				t.Errorf("tope HTTP %v menor que el presupuesto %v", c.http.Timeout, tc.budget)
			}
		})
	}
}
