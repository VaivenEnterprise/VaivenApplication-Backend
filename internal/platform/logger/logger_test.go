package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestProductionWritesJSON(t *testing.T) {
	var buf bytes.Buffer
	log, err := newWithWriter(&buf, "info", "production")
	if err != nil {
		t.Fatalf("newWithWriter: %v", err)
	}

	log.Info("petición atendida", "status", 200, "requestId", "abc123")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("la salida no es JSON válido: %v\n%s", err, buf.String())
	}
	if entry["level"] != "INFO" || entry["msg"] != "petición atendida" || entry["requestId"] != "abc123" {
		t.Errorf("entrada JSON inesperada: %v", entry)
	}
	if _, ok := entry["time"]; !ok {
		t.Error("falta el campo time")
	}
}

func TestOtherEnvsWriteText(t *testing.T) {
	for _, env := range []string{"development", "test", ""} {
		t.Run("env="+env, func(t *testing.T) {
			var buf bytes.Buffer
			log, err := newWithWriter(&buf, "info", env)
			if err != nil {
				t.Fatalf("newWithWriter: %v", err)
			}

			log.Info("hola", "status", 200)

			out := buf.String()
			if json.Valid(bytes.TrimSpace(buf.Bytes())) {
				t.Errorf("fuera de producción se esperaba texto, no JSON: %s", out)
			}
			for _, want := range []string{"level=INFO", "msg=hola", "status=200"} {
				if !strings.Contains(out, want) {
					t.Errorf("salida %q sin %q", out, want)
				}
			}
		})
	}
}

func TestLevelFiltersMessages(t *testing.T) {
	var buf bytes.Buffer
	log, err := newWithWriter(&buf, "WARN", "development")
	if err != nil {
		t.Fatalf("newWithWriter: %v", err)
	}

	log.Info("no debe salir")
	log.Warn("sí debe salir")

	out := buf.String()
	if strings.Contains(out, "no debe salir") {
		t.Errorf("con nivel warn se registró un info: %s", out)
	}
	if !strings.Contains(out, "sí debe salir") {
		t.Errorf("con nivel warn no se registró el warn: %s", out)
	}
}

func TestInvalidLevel(t *testing.T) {
	for _, level := range []string{"", "verbose", "trace", "info2"} {
		if _, err := newWithWriter(&bytes.Buffer{}, level, "development"); err == nil {
			t.Errorf("nivel %q: se esperaba error", level)
		}
	}
	if _, err := New("verbose", "production"); err == nil {
		t.Error("New: se esperaba error con nivel inválido")
	}
}

func TestNewWritesToStdout(t *testing.T) {
	log, err := New("debug", "development")
	if err != nil || log == nil {
		t.Fatalf("New: logger=%v err=%v", log, err)
	}
	if !log.Enabled(t.Context(), slog.LevelDebug) {
		t.Error("con nivel debug, Debug debe estar habilitado")
	}
}
