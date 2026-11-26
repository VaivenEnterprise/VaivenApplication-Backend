package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestAIDefaultsAreOffAndSafe(t *testing.T) {
	cfg, err := load(valid().lookup)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := cfg.AI
	if a.SuggestionsEnabled || a.AssistantEnabled || a.Enabled() {
		t.Errorf("las funciones deben nacer apagadas: %+v", a)
	}
	if a.Provider != AIProviderOpenAI || a.BaseURL != DefaultOpenAIBaseURL || a.Model != DefaultOpenAIModel {
		t.Errorf("proveedor por defecto (Groq) = %+v", a)
	}
	if a.DailyLimit != 20 || a.AssistantDailyLimit != 30 || a.AssistantRadiusKm != 10 {
		t.Errorf("límites por defecto = %+v", a)
	}
}

func TestAIEnabledNeedsKey(t *testing.T) {
	for _, flag := range []string{"AI_ENABLED", "ASSISTANT_ENABLED"} {
		e := valid()
		e[flag] = "true"
		if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "AI_API_KEY") {
			t.Errorf("%s sin clave: err = %v", flag, err)
		}
		e["AI_API_KEY"] = "clave-de-prueba"
		cfg, err := load(e.lookup)
		if err != nil {
			t.Fatalf("%s con clave: %v", flag, err)
		}
		if !cfg.AI.Enabled() {
			t.Errorf("%s: Enabled() = false", flag)
		}
	}
}

func TestAIKeyIsSecret(t *testing.T) {
	e := valid()
	e["AI_API_KEY"] = "muy-secreta"
	cfg, err := load(e.lookup)
	if err != nil {
		t.Fatal(err)
	}
	if s := fmt.Sprintf("%v %+v %#v", cfg.AI.APIKey, cfg.AI, cfg.AI); strings.Contains(s, "muy-secreta") {
		t.Errorf("la clave se imprime: %s", s)
	}
	if cfg.AI.APIKey.Reveal() != "muy-secreta" {
		t.Error("Reveal no devuelve la clave")
	}
}

func TestAIOllamaOnlyInDevelopment(t *testing.T) {
	e := valid()
	e["AI_PROVIDER"] = "ollama"
	e["ASSISTANT_ENABLED"] = "true"
	e["AI_MODEL"] = "llama3.2"
	cfg, err := load(e.lookup)
	if err != nil {
		t.Fatalf("ollama en development: %v", err)
	}
	if cfg.AI.BaseURL != DefaultOllamaBaseURL {
		t.Errorf("BaseURL = %q", cfg.AI.BaseURL)
	}
	for _, env := range []string{EnvTest, EnvStaging, EnvProduction} {
		e := deployedEnv(env)
		e["AI_PROVIDER"] = "ollama"
		if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "ollama solo se permite") {
			t.Errorf("ollama en %s: err = %v", env, err)
		}
	}
}

func TestAIGeminiAlternativeDefaults(t *testing.T) {
	e := valid()
	e["AI_PROVIDER"] = "gemini"
	e["AI_ENABLED"] = "true"
	if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "AI_API_KEY") {
		t.Errorf("gemini sin clave: err = %v", err)
	}
	e["AI_API_KEY"] = "clave"
	cfg, err := load(e.lookup)
	if err != nil || cfg.AI.BaseURL != DefaultGeminiBaseURL || cfg.AI.Model != DefaultGeminiModel {
		t.Errorf("gemini: %+v %v", cfg.AI, err)
	}
}

func TestAIOllamaNeedsModelWhenEnabled(t *testing.T) {
	e := valid()
	e["AI_PROVIDER"] = "ollama"
	e["AI_ENABLED"] = "true"
	if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "AI_MODEL") {
		t.Errorf("ollama sin modelo: err = %v", err)
	}
}

func TestAIBaseURLRules(t *testing.T) {
	e := valid()
	e["AI_BASE_URL"] = "ftp://algo"
	if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "AI_BASE_URL") {
		t.Errorf("esquema inválido: err = %v", err)
	}
	e = deployedEnv(EnvStaging)
	e["AI_BASE_URL"] = "http://inseguro.example"
	if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "https://") {
		t.Errorf("http en staging: err = %v", err)
	}
	e = valid()
	e["AI_BASE_URL"] = "https://proveedor.example/"
	cfg, err := load(e.lookup)
	if err != nil || cfg.AI.BaseURL != "https://proveedor.example" {
		t.Errorf("barra final: %q, %v", cfg.AI.BaseURL, err)
	}
}

func TestAILimitsValidation(t *testing.T) {
	for key, bad := range map[string]string{
		"AI_DAILY_LIMIT":        "0",
		"ASSISTANT_DAILY_LIMIT": "-3",
		"ASSISTANT_RADIUS_KM":   "51",
		"AI_PROVIDER":           "proveedor-desconocido",
		"ASSISTANT_ENABLED":     "quizá",
	} {
		e := valid()
		e[key] = bad
		if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%s: err = %v", key, bad, err)
		}
	}
}

// deployedEnv devuelve un entorno válido para staging, production o test con
// lo que esos entornos exigen (HTTPS y Redis).
func deployedEnv(appEnv string) env {
	e := valid()
	e["APP_ENV"] = appEnv
	e["TLS_CERT_FILE"] = "/certs/fullchain.pem"
	e["TLS_KEY_FILE"] = "/certs/privkey.pem"
	e["REDIS_URL"] = "redis://:clave@redis:6379/0"
	return e
}

func TestSuggestProviderOverridesAndFallback(t *testing.T) {
	// Sin SUGGEST_*: las sugerencias usan el mismo proveedor que el asistente.
	e := valid()
	e["AI_API_KEY"] = "gsk_groq"
	cfg, err := load(e.lookup)
	if err != nil || cfg.AI.Suggest != cfg.AI.Assistant() {
		t.Fatalf("herencia: %+v %v", cfg.AI.Suggest, err)
	}
	// Con SUGGEST_*: OpenRouter para la parte A y Groq para la B.
	e = valid()
	e["AI_ENABLED"], e["ASSISTANT_ENABLED"] = "true", "true"
	e["AI_API_KEY"] = "gsk_groq"
	e["SUGGEST_BASE_URL"] = "https://openrouter.ai/api/v1/"
	e["SUGGEST_MODEL"] = "google/gemma-4-31b-it:free"
	e["SUGGEST_API_KEY"] = "sk-or-clave"
	cfg, err = load(e.lookup)
	if err != nil {
		t.Fatal(err)
	}
	s, a := cfg.AI.Suggest, cfg.AI.Assistant()
	if s.Name != AIProviderOpenAI || s.BaseURL != "https://openrouter.ai/api/v1" || s.Model != "google/gemma-4-31b-it:free" || s.APIKey.Reveal() != "sk-or-clave" {
		t.Errorf("sugerencias = %+v", s)
	}
	if a.BaseURL != DefaultOpenAIBaseURL || a.APIKey.Reveal() != "gsk_groq" {
		t.Errorf("el asistente no cambia: %+v", a)
	}
	if strings.Contains(fmt.Sprintf("%+v", cfg.AI), "sk-or-clave") {
		t.Error("la clave de las sugerencias se imprime")
	}
}

func TestSuggestOnlyEnabledNeedsOnlyItsKey(t *testing.T) {
	e := valid()
	e["AI_ENABLED"] = "true"
	e["SUGGEST_API_KEY"] = "sk-or-clave"
	if _, err := load(e.lookup); err != nil {
		t.Errorf("solo sugerencias con su clave, sin AI_API_KEY: %v", err)
	}
	e = valid()
	e["ASSISTANT_ENABLED"] = "true"
	e["SUGGEST_API_KEY"] = "sk-or-clave"
	if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "AI_API_KEY") {
		t.Errorf("el asistente necesita AI_API_KEY: %v", err)
	}
}

func TestSuggestProviderRules(t *testing.T) {
	for key, bad := range map[string]string{"SUGGEST_PROVIDER": "otro", "SUGGEST_BASE_URL": "ftp://x"} {
		e := valid()
		e[key] = bad
		if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%s: %v", key, bad, err)
		}
	}
	e := deployedEnv(EnvStaging)
	e["SUGGEST_BASE_URL"] = "http://inseguro"
	if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "SUGGEST_BASE_URL") {
		t.Errorf("http en staging: %v", err)
	}
	e = deployedEnv(EnvStaging)
	e["SUGGEST_PROVIDER"] = "ollama"
	if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "SUGGEST_PROVIDER") {
		t.Errorf("ollama en staging: %v", err)
	}
	// Ollama en local sin modelo propio ni heredado: el error nombra AI_MODEL.
	e = valid()
	e["AI_PROVIDER"] = "ollama"
	e["AI_ENABLED"] = "true"
	if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "AI_MODEL") {
		t.Errorf("sin modelo: %v", err)
	}
}
