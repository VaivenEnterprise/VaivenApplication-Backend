package config

import (
	"strings"
	"testing"
)

func TestRecoOffByDefault(t *testing.T) {
	cfg, err := load(valid().lookup)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Reco.Enabled {
		t.Error("las recomendaciones deben nacer apagadas")
	}
}

func TestRecoEnabledAndInvalid(t *testing.T) {
	e := valid()
	e["RECO_ENABLED"] = "true"
	cfg, err := load(e.lookup)
	if err != nil || !cfg.Reco.Enabled {
		t.Fatalf("RECO_ENABLED=true: %+v %v", cfg.Reco, err)
	}
	e["RECO_ENABLED"] = "quizá"
	if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), "RECO_ENABLED") {
		t.Errorf("valor inválido: %v", err)
	}
}
