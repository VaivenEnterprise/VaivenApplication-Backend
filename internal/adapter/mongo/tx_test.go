package mongo

import (
	"context"
	"testing"
)

func TestTxRunnerNeedsFunction(t *testing.T) {
	// Sin función no se abre ninguna sesión: no necesita servidor.
	if err := (&TxRunner{}).Do(context.Background(), nil); err == nil {
		t.Fatal("Do(nil) debe fallar")
	}
}
