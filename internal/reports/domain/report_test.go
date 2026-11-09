package domain

import (
	"errors"
	"strings"
	"testing"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

func isValidation(err error) bool {
	var de *shared.Error
	return errors.As(err, &de) && de.Code == shared.CodeValidation
}

func TestParseEnums(t *testing.T) {
	for _, s := range []string{"item", "message", "user", "moderation_decision"} {
		if got, err := ParseTargetType(s); err != nil || string(got) != s {
			t.Errorf("ParseTargetType(%q) = %q, %v", s, got, err)
		}
	}
	for _, s := range []string{"prohibited_content", "inappropriate", "spam", "other"} {
		if got, err := ParseReason(s); err != nil || string(got) != s {
			t.Errorf("ParseReason(%q) = %q, %v", s, got, err)
		}
	}
	for _, s := range []string{"dismiss", "hide_item", "suspend_user"} {
		if got, err := ParseAction(s); err != nil || string(got) != s {
			t.Errorf("ParseAction(%q) = %q, %v", s, got, err)
		}
	}
	for in, want := range map[string]Status{"": StatusOpen, "open": StatusOpen, "actioned": StatusActioned, "dismissed": StatusDismissed} {
		if got, err := ParseStatus(in); err != nil || got != want {
			t.Errorf("ParseStatus(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseTargetType("booking"); !isValidation(err) {
		t.Error("tipo inválido")
	}
	if _, err := ParseReason("odio"); !isValidation(err) {
		t.Error("motivo inválido")
	}
	if _, err := ParseAction("ban"); !isValidation(err) {
		t.Error("acción inválida")
	}
	if _, err := ParseStatus("closed"); !isValidation(err) {
		t.Error("estado inválido")
	}
}

func TestNormalizeTargetID(t *testing.T) {
	const oid = "6731e0000000000000000001"
	for _, tt := range []TargetType{TargetItem, TargetMessage, TargetUser} {
		if got, err := NormalizeTargetID(tt, " "+oid+" "); err != nil || got != oid {
			t.Errorf("%s: %q, %v", tt, got, err)
		}
		for _, bad := range []string{"", "  ", "abc", strings.ToUpper(oid), oid + "0"} {
			if _, err := NormalizeTargetID(tt, bad); !isValidation(err) {
				t.Errorf("%s %q debe ser inválido", tt, bad)
			}
		}
	}
	// moderation_decision acepta una referencia libre de hasta 64.
	if got, err := NormalizeTargetID(TargetModerationDecision, "rechazo-título-2026"); err != nil || got != "rechazo-título-2026" {
		t.Errorf("referencia libre: %q, %v", got, err)
	}
	for _, bad := range []string{"", strings.Repeat("a", 65), "con espacio", "tab\tx", "del\x7f"} {
		if _, err := NormalizeTargetID(TargetModerationDecision, bad); !isValidation(err) {
			t.Errorf("referencia %q debe ser inválida", bad)
		}
	}
	if _, err := NormalizeTargetID(TargetModerationDecision, strings.Repeat("a", 64)); err != nil {
		t.Error("64 caracteres es válido")
	}
}

func TestNormalizeDetails(t *testing.T) {
	if got, err := NormalizeDetails(nil); got != nil || err != nil {
		t.Error("nil")
	}
	blank := "   "
	if got, err := NormalizeDetails(&blank); got != nil || err != nil {
		t.Error("vacío = nil")
	}
	d := "  Algo grave  "
	if got, err := NormalizeDetails(&d); err != nil || *got != "Algo grave" {
		t.Errorf("recorta: %v %v", got, err)
	}
	ok := strings.Repeat("ñ", 500)
	if _, err := NormalizeDetails(&ok); err != nil {
		t.Error("500 caracteres es válido")
	}
	long := strings.Repeat("ñ", 501)
	if _, err := NormalizeDetails(&long); !isValidation(err) {
		t.Error("501 caracteres es inválido")
	}
}

func TestNormalizeNote(t *testing.T) {
	if got, err := NormalizeNote("  ok! "); err != nil || got != "ok!" {
		t.Errorf("%q %v", got, err)
	}
	for _, bad := range []string{"", "  ab ", strings.Repeat("x", 501)} {
		if _, err := NormalizeNote(bad); !isValidation(err) {
			t.Errorf("%q debe ser inválida", bad)
		}
	}
	if _, err := NormalizeNote(strings.Repeat("x", 500)); err != nil {
		t.Error("500 es válido")
	}
}

func TestCheckAction(t *testing.T) {
	allowed := map[Action][]TargetType{
		ActionDismiss:     {TargetItem, TargetMessage, TargetUser, TargetModerationDecision},
		ActionHideItem:    {TargetItem},
		ActionSuspendUser: {TargetItem, TargetMessage, TargetUser},
	}
	all := []TargetType{TargetItem, TargetMessage, TargetUser, TargetModerationDecision}
	for a, ok := range allowed {
		for _, tt := range all {
			want := false
			for _, x := range ok {
				want = want || x == tt
			}
			err := CheckAction(a, tt)
			if want && err != nil {
				t.Errorf("%s sobre %s debe permitirse: %v", a, tt, err)
			}
			if !want && !errors.Is(err, ErrActionNotAllowed) {
				t.Errorf("%s sobre %s debe ser conflicto: %v", a, tt, err)
			}
		}
	}
}

func TestActionResolve(t *testing.T) {
	if ActionDismiss.Resolve() != StatusDismissed || ActionHideItem.Resolve() != StatusActioned || ActionSuspendUser.Resolve() != StatusActioned {
		t.Error("estado final por acción")
	}
}

func TestSuspensionIsSevenDays(t *testing.T) {
	if SuspensionDays != 7 {
		t.Errorf("plan P-07: 7 días, hay %d", SuspensionDays)
	}
}
