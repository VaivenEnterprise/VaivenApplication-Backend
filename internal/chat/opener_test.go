package chat

import (
	"context"
	"testing"
)

func TestNoopOpener(t *testing.T) {
	var o Opener = Noop{}
	id, err := o.Open(context.Background(), "b", "o", "r")
	if id != "" || err != nil {
		t.Errorf("Open = %q %v", id, err)
	}
}
