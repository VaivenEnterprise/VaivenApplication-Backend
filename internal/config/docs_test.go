package config

import (
	"errors"
	"strings"
	"testing"
)

func TestLoadDocs(t *testing.T) {
	cases := []struct {
		name    string
		appEnv  string
		value   string // "" = sin DOCS_ENABLED
		want    bool
		wantErr bool
	}{
		{"development por defecto", EnvDevelopment, "", true, false},
		{"staging por defecto", EnvStaging, "", true, false},
		{"test por defecto", EnvTest, "", false, false},
		{"production por defecto", EnvProduction, "", false, false},
		{"staging apagado", EnvStaging, "false", false, false},
		{"development con 0", EnvDevelopment, "0", false, false},
		{"production apagado explícito", EnvProduction, "false", false, false},
		{"production encendido", EnvProduction, "true", false, true},
		{"test encendido", EnvTest, "1", false, true},
		{"valor inválido", EnvStaging, "quizá", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := env{"APP_ENV": tc.appEnv}
			if tc.value != "" {
				e["DOCS_ENABLED"] = tc.value
			}
			r := reader{lookup: e.lookup}
			got := loadDocs(&r, tc.appEnv)
			if got != tc.want {
				t.Errorf("loadDocs = %v, want %v", got, tc.want)
			}
			if hasErr := len(r.problems) > 0; hasErr != tc.wantErr {
				t.Errorf("problemas = %v, wantErr %v", r.problems, tc.wantErr)
			}
		})
	}
}

func TestLoadRejectsDocsInProduction(t *testing.T) {
	e := deployed()
	e["APP_ENV"] = EnvProduction
	e["DOCS_ENABLED"] = "true"
	_, err := load(e.lookup)
	var cfgErr *Error
	if !errors.As(err, &cfgErr) || !strings.Contains(err.Error(), "DOCS_ENABLED") {
		t.Fatalf("load: se esperaba error de DOCS_ENABLED, got %v", err)
	}
}
