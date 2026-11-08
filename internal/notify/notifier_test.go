package notify

import (
	"context"
	"testing"
)

func TestNoopNeverFails(t *testing.T) {
	var n Notifier = Noop{}
	if err := n.Notify(context.Background(), "u1", Event{Type: "test", Data: map[string]string{"k": "v"}}); err != nil {
		t.Fatalf("Notify = %v, want nil", err)
	}
	if err := n.Notify(context.Background(), "", Event{}); err != nil {
		t.Fatalf("Notify vacío = %v, want nil", err)
	}
}
