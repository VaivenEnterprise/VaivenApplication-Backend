package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, nil))
}

func TestLoggingRecordsRequest(t *testing.T) {
	var buf bytes.Buffer
	h := RequestID(Logging(newTestLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/things?token=secreto", strings.NewReader(`{"password":"p4ss"}`))
	req.Header.Set("Authorization", "Bearer token-secreto")
	req.Header.Set("Cookie", "sesion=secreta")
	req.Header.Set(HeaderRequestID, "req-42")
	h.ServeHTTP(httptest.NewRecorder(), req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("el log no es JSON: %v (%q)", err, buf.String())
	}
	want := map[string]any{
		"method":    "POST",
		"path":      "/api/v1/things",
		"status":    float64(http.StatusTeapot),
		"requestId": "req-42",
	}
	for k, v := range want {
		if entry[k] != v {
			t.Errorf("%s = %v, want %v", k, entry[k], v)
		}
	}
	if d, ok := entry["durationMs"].(float64); !ok || d < 0 {
		t.Errorf("durationMs = %v, want número >= 0", entry["durationMs"])
	}
	for _, leak := range []string{"secreto", "p4ss", "Bearer", "token="} {
		if strings.Contains(buf.String(), leak) {
			t.Errorf("el log contiene %q: %s", leak, buf.String())
		}
	}
}

func TestLoggingDefaultStatusIs200(t *testing.T) {
	var buf bytes.Buffer
	h := Logging(newTestLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want 200", entry["status"])
	}
}

func TestLoggingKeepsFirstStatus(t *testing.T) {
	var buf bytes.Buffer
	h := Logging(newTestLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.WriteHeader(http.StatusInternalServerError) // net/http lo ignora
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["status"] != float64(http.StatusCreated) {
		t.Errorf("status = %v, want 201", entry["status"])
	}
}

func TestStatusRecorderUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec}
	if sr.Unwrap() != http.ResponseWriter(rec) {
		t.Error("Unwrap no devuelve el ResponseWriter original")
	}
}
