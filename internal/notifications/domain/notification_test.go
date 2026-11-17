package domain

import (
	"errors"
	"strings"
	"testing"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

func TestNormalizeToken(t *testing.T) {
	good := []string{"ExponentPushToken[abc-123_XYZ]", "ExpoPushToken[xxxxxxxxxxxxxxxxxxxxxx]", "  ExponentPushToken[abc]  "}
	for _, in := range good {
		got, err := NormalizeToken(in)
		if err != nil || got != strings.TrimSpace(in) {
			t.Errorf("NormalizeToken(%q) = %q, %v", in, got, err)
		}
	}
	bad := []string{"", "   ", "abc", "ExponentPushToken[]", "ExponentPushToken[abc", "ExponentPushToken[a b]",
		"ExponentPushToken[a[b]]", "Other[abc]", "ExponentPushToken[" + strings.Repeat("a", 200) + "]"}
	for _, in := range bad {
		_, err := NormalizeToken(in)
		var de *shared.Error
		if !errors.As(err, &de) || de.Code != shared.CodeValidation {
			t.Errorf("NormalizeToken(%q) debe ser validation_failed, fue %v", in, err)
		}
	}
	if !IsExpoToken("ExpoPushToken[a]") || IsExpoToken("x") {
		t.Error("IsExpoToken")
	}
}

func TestValidatePlatform(t *testing.T) {
	for _, p := range []string{"android", "ios"} {
		if err := ValidatePlatform(p); err != nil {
			t.Errorf("ValidatePlatform(%q): %v", p, err)
		}
	}
	for _, p := range []string{"", "web", "Android"} {
		if err := ValidatePlatform(p); err == nil {
			t.Errorf("ValidatePlatform(%q) debe fallar", p)
		}
	}
}

func TestIsObjectID(t *testing.T) {
	if !IsObjectID("6731e0000000000000000001") {
		t.Error("id válido")
	}
	for _, s := range []string{"", "xyz", "6731E0000000000000000001", "6731e000000000000000000", "6731e00000000000000000011"} {
		if IsObjectID(s) {
			t.Errorf("IsObjectID(%q) debe ser false", s)
		}
	}
}
