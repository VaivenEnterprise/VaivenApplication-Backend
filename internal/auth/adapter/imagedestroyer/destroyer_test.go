package imagedestroyer

import (
	"context"
	"errors"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
)

type fake struct {
	typ, id string
	err     error
}

func (f *fake) Destroy(_ context.Context, typ, id string) error {
	f.typ, f.id = typ, id
	return f.err
}

func TestDestroyDelegates(t *testing.T) {
	f := &fake{}
	d, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Destroy(context.Background(), usecase.ImageRef{PublicID: "a/b", Delivery: "authenticated"}); err != nil {
		t.Fatal(err)
	}
	if f.typ != "authenticated" || f.id != "a/b" {
		t.Errorf("llamada = %q %q", f.typ, f.id)
	}
	// Sin tipo de entrega se usa «upload».
	if err := d.Destroy(context.Background(), usecase.ImageRef{PublicID: "c"}); err != nil || f.typ != "upload" {
		t.Errorf("sin entrega: %q, %v", f.typ, err)
	}
}

func TestDestroyPropagatesError(t *testing.T) {
	boom := errors.New("caído")
	d, _ := New(&fake{err: boom})
	if err := d.Destroy(context.Background(), usecase.ImageRef{PublicID: "x"}); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewRequiresDestroyer(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) debe fallar")
	}
}
