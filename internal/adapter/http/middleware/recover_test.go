package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func panicHandler(v any) http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(v) })
}

func TestRecoverRespondsInternalError(t *testing.T) {
	var buf bytes.Buffer
	h := RequestID(Recover(newTestLogger(&buf))(panicHandler("fallo con mongodb://user:clave@host")))

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	req.Header.Set(HeaderRequestID, "req-7")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var env struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("cuerpo no es JSON: %v", err)
	}
	if env.Error["code"] != "internal_error" || env.Error["requestId"] != "req-7" {
		t.Errorf("error = %v", env.Error)
	}
	if strings.Contains(rec.Body.String(), "clave") || strings.Contains(rec.Body.String(), "fallo") {
		t.Errorf("la respuesta expone el panic: %s", rec.Body.String())
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("el log no es JSON: %v", err)
	}
	if entry["level"] != "ERROR" || entry["requestId"] != "req-7" {
		t.Errorf("log = %v", entry)
	}
	if p, _ := entry["panic"].(string); !strings.Contains(p, "fallo") {
		t.Errorf("el log no registra el panic: %v", entry["panic"])
	}
	if s, _ := entry["stack"].(string); s == "" {
		t.Error("el log no incluye la pila")
	}
}

func TestRecoverKeepsServerAlive(t *testing.T) {
	var buf bytes.Buffer
	mux := http.NewServeMux()
	mux.Handle("/boom", panicHandler("x"))
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	srv := httptest.NewServer(Recover(newTestLogger(&buf))(mux))
	defer srv.Close()

	for path, want := range map[string]int{"/boom": 500, "/ok": 204} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("GET %s = %d, want %d", path, resp.StatusCode, want)
		}
	}
}

func TestRecoverRepanicsOnAbortHandler(t *testing.T) {
	var buf bytes.Buffer
	h := Recover(newTestLogger(&buf))(panicHandler(http.ErrAbortHandler))

	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recover() = %v, want http.ErrAbortHandler", v)
		}
		if buf.Len() != 0 {
			t.Errorf("no debe registrar nada: %s", buf.String())
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestRecoverPassesThroughWithoutPanic(t *testing.T) {
	var buf bytes.Buffer
	h := Recover(newTestLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusAccepted || buf.Len() != 0 {
		t.Errorf("status = %d, log = %q", rec.Code, buf.String())
	}
}
