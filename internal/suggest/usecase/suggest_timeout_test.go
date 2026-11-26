package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

// deadlineAI registra el plazo del contexto que recibe del caso de uso.
type deadlineAI struct {
	fakeAI
	remaining time.Duration
}

func (d *deadlineAI) Generate(ctx context.Context, req aidomain.Request) ([]byte, error) {
	if dl, ok := ctx.Deadline(); ok {
		d.remaining = time.Until(dl)
	}
	return d.fakeAI.Generate(ctx, req)
}

func TestSuggestHonorsConfiguredTimeout(t *testing.T) { // RF-06, T018
	cases := []struct {
		name string
		cfg  time.Duration
		want time.Duration
	}{
		{"por defecto", 0, 25 * time.Second},
		{"configurado", 40 * time.Second, 40 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ai := &deadlineAI{fakeAI: fakeAI{out: `{"title":"Taladro","category":"herramientas_electricas"}`}}
			svc := New(Config{Enabled: true, DailyLimit: 2, BaseFolder: base, Timeout: c.cfg},
				Deps{AI: ai, Images: fakeImages{}, Usage: &fakeUsage{left: 2}})
			if _, err := svc.Suggest(context.Background(), owner, ownPhoto()); err != nil {
				t.Fatal(err)
			}
			if ai.remaining <= c.want-2*time.Second || ai.remaining > c.want {
				t.Errorf("plazo = %v, esperado cerca de %v", ai.remaining, c.want)
			}
		})
	}
}

func TestSuggestMalformedOutputKeepsCause(t *testing.T) {
	e := newEnv(t, nil, fakeImages{}, true)
	e.ai.out = `{"title":""}`
	_, err := e.svc.Suggest(context.Background(), owner, ownPhoto())
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeUnavailable {
		t.Fatalf("err = %v", err)
	}
	if cause, _, ok := aidomain.CauseOf(err); !ok || cause != aidomain.CauseMalformedJSON {
		t.Errorf("causa = %v %v", cause, ok)
	}
}
