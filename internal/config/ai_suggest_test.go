package config

import (
	"strings"
	"testing"
	"time"
)

// T018 (spec 014): SUGGEST_TIMEOUT y SUGGEST_REASONING.
func TestSuggestTimeoutAndReasoning(t *testing.T) {
	cases := []struct {
		name          string
		set           map[string]string
		wantErr       string
		wantTimeout   time.Duration
		wantReasoning string
	}{
		{"por defecto (Groq)", nil, "", 25 * time.Second, SuggestReasoningDefault},
		{"timeout propio", map[string]string{"SUGGEST_TIMEOUT": "40s"}, "", 40 * time.Second, SuggestReasoningDefault},
		{"mínimo", map[string]string{"SUGGEST_TIMEOUT": "5s"}, "", 5 * time.Second, SuggestReasoningDefault},
		{"máximo", map[string]string{"SUGGEST_TIMEOUT": "1m"}, "", time.Minute, SuggestReasoningDefault},
		{"muy corto", map[string]string{"SUGGEST_TIMEOUT": "4s"}, "SUGGEST_TIMEOUT", 0, ""},
		{"muy largo", map[string]string{"SUGGEST_TIMEOUT": "61s"}, "SUGGEST_TIMEOUT", 0, ""},
		{"inválido", map[string]string{"SUGGEST_TIMEOUT": "mucho"}, "SUGGEST_TIMEOUT", 0, ""},
		{"OpenRouter apaga el razonamiento", map[string]string{"SUGGEST_BASE_URL": "https://openrouter.ai/api/v1"}, "", 25 * time.Second, SuggestReasoningOff},
		{"OpenRouter heredado de AI_BASE_URL", map[string]string{"AI_BASE_URL": "https://openrouter.ai/api/v1"}, "", 25 * time.Second, SuggestReasoningOff},
		{"OpenRouter con default explícito", map[string]string{"SUGGEST_BASE_URL": "https://openrouter.ai/api/v1", "SUGGEST_REASONING": "default"}, "", 25 * time.Second, SuggestReasoningDefault},
		{"Groq con off explícito", map[string]string{"SUGGEST_REASONING": "off"}, "", 25 * time.Second, SuggestReasoningOff},
		{"host parecido no cuenta", map[string]string{"SUGGEST_BASE_URL": "https://openrouter.ai.otro.com/v1"}, "", 25 * time.Second, SuggestReasoningDefault},
		{"razonamiento inválido", map[string]string{"SUGGEST_REASONING": "on"}, "SUGGEST_REASONING", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := valid()
			for k, v := range c.set {
				e[k] = v
			}
			cfg, err := load(e.lookup)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("debe fallar con %s: %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.AI.SuggestTimeout != c.wantTimeout || cfg.AI.SuggestReasoning != c.wantReasoning {
				t.Errorf("timeout=%v reasoning=%q", cfg.AI.SuggestTimeout, cfg.AI.SuggestReasoning)
			}
		})
	}
}
