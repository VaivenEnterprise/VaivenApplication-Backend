package domain

import "testing"

func TestMessageRefHasParticipant(t *testing.T) {
	m := MessageRef{Participants: [2]string{"a", "b"}}
	for id, want := range map[string]bool{"a": true, "b": true, "c": false, "": false} {
		if got := m.HasParticipant(id); got != want {
			t.Errorf("HasParticipant(%q) = %v, quiero %v", id, got, want)
		}
	}
	if (MessageRef{}).HasParticipant("") {
		t.Error("una referencia vacía no tiene participantes")
	}
}
