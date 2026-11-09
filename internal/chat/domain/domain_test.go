package domain

import (
	"errors"
	"strings"
	"testing"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

func field(t *testing.T, err error) string {
	t.Helper()
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeValidation {
		t.Fatalf("err = %v", err)
	}
	return de.Details[0].Field
}

func TestNormalizeBody(t *testing.T) {
	if got, err := NormalizeBody("  hola  "); err != nil || got != "hola" {
		t.Errorf("recorte = %q %v", got, err)
	}
	if got, err := NormalizeBody(strings.Repeat("ñ", 1000)); err != nil || len([]rune(got)) != 1000 {
		t.Errorf("1000 runas = %v", err)
	}
	for _, b := range []string{"", "   \n\t", strings.Repeat("a", 1001)} { // CA-05
		if _, err := NormalizeBody(b); field(t, err) != "body" {
			t.Errorf("%q debe fallar", b[:min(len(b), 5)])
		}
	}
	// Texto plano: no se interpreta ni se escapa.
	if got, _ := NormalizeBody("<b>hola</b>"); got != "<b>hola</b>" {
		t.Error("el cuerpo no se transforma")
	}
}

func TestNormalizeClientID(t *testing.T) {
	if id, err := NormalizeClientID(nil); id != nil || err != nil {
		t.Error("nil")
	}
	blank := "  "
	if id, err := NormalizeClientID(&blank); id != nil || err != nil {
		t.Error("vacío")
	}
	ok := " abc "
	if id, err := NormalizeClientID(&ok); err != nil || *id != "abc" {
		t.Error("recorte")
	}
	long := strings.Repeat("x", 65)
	if _, err := NormalizeClientID(&long); field(t, err) != "clientId" {
		t.Error("largo")
	}
}

func TestPreview(t *testing.T) {
	if Preview("corto") != "corto" {
		t.Error("corto")
	}
	p := Preview(strings.Repeat("é", 90))
	if len([]rune(p)) != 81 || !strings.HasSuffix(p, "…") {
		t.Errorf("preview = %d runas", len([]rune(p)))
	}
	if Preview(strings.Repeat("a", 80)) != strings.Repeat("a", 80) {
		t.Error("80 exactos sin recorte")
	}
}

func TestConversation(t *testing.T) {
	c := Conversation{Participants: [2]string{"own", "req"}, Unread: map[string]int{"own": 2}}
	if !c.IsParticipant("own") || !c.IsParticipant("req") || c.IsParticipant("x") || c.IsParticipant("") {
		t.Error("IsParticipant")
	}
	if c.Counterpart("own") != "req" || c.Counterpart("req") != "own" {
		t.Error("Counterpart")
	}
	if c.UnreadFor("own") != 2 || c.UnreadFor("req") != 0 {
		t.Error("UnreadFor")
	}
}

func TestCanWrite(t *testing.T) {
	for st, want := range map[string]bool{
		"pending": true, "accepted": true, "in_progress": true,
		"rejected": false, "expired": false, "cancelled": false, "completed": false, "": false,
	} {
		if CanWrite(st, true) != want {
			t.Errorf("CanWrite(%s) != %v", st, want)
		}
	}
	if CanWrite("accepted", false) { // RF-07
		t.Error("contraparte dada de baja")
	}
	if ErrClosed.Message != "La conversación está cerrada." || ErrNotFound.Code != shared.CodeNotFound || ErrDuplicateClientID.Code != shared.CodeConflict {
		t.Error("errores")
	}
}
