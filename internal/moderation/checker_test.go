package moderation

import (
	"context"
	"testing"
)

func TestAllowAllAllowsEverything(t *testing.T) {
	var c Checker = AllowAll{}
	for _, text := range []string{"", "hola", "texto con groserías de prueba"} {
		res, err := c.Check(context.Background(), text)
		if err != nil {
			t.Fatalf("Check(%q) error = %v", text, err)
		}
		if !res.Allowed || res.Reason != "" {
			t.Errorf("Check(%q) = %+v, want Allowed sin motivo", text, res)
		}
	}
}
