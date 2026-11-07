package config

import (
	"strings"
	"testing"
)

func TestImagesConfig(t *testing.T) {
	e := valid()
	e["IMAGE_MAX_BYTES"] = "1048576"
	e["IMAGE_SIGNATURE_PER_HOUR"] = "5"
	cfg, err := load(e.lookup)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Images.MaxBytes != 1048576 || cfg.Images.SignaturesPerHour != 5 {
		t.Errorf("Images = %+v", cfg.Images)
	}
	for _, key := range []string{"IMAGE_MAX_BYTES", "IMAGE_SIGNATURE_PER_HOUR"} {
		e := valid()
		e[key] = "-1"
		if _, err := load(e.lookup); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=-1: err = %v", key, err)
		}
	}
}
