package redis

import (
	"context"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/revocation"
)

var (
	_ revocation.Store = (*Revocations)(nil)
	_ revocation.Store = (*MemoryRevocations)(nil)
)

func TestMemoryRevocations(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	m := NewMemoryRevocations()
	m.now = func() time.Time { return now }

	if r, _ := m.IsRevoked(ctx, "u1", "j1", now); r {
		t.Fatal("un token nuevo no está revocado")
	}
	// Logout: solo ese jti.
	_ = m.RevokeToken(ctx, "j1", now.Add(15*time.Minute))
	if r, _ := m.IsRevoked(ctx, "u1", "j1", now); !r {
		t.Error("jti revocado")
	}
	if r, _ := m.IsRevoked(ctx, "u1", "j2", now); r {
		t.Error("otra sesión del mismo usuario sigue válida")
	}
	// Baja de cuenta: todo lo emitido hasta ahora.
	_ = m.RevokeUserBefore(ctx, "u2", now, 15*time.Minute)
	if r, _ := m.IsRevoked(ctx, "u2", "x", now.Add(-time.Minute)); !r {
		t.Error("token anterior al corte")
	}
	if r, _ := m.IsRevoked(ctx, "u2", "y", now.Add(time.Second)); r {
		t.Error("token posterior al corte sigue válido")
	}
	if r, _ := m.IsRevoked(ctx, "u3", "x", now.Add(-time.Minute)); r {
		t.Error("no afecta a otros usuarios")
	}
	// Un corte viejo no baja uno nuevo.
	_ = m.RevokeUserBefore(ctx, "u2", now.Add(-time.Hour), time.Hour)
	if r, _ := m.IsRevoked(ctx, "u2", "x", now.Add(-time.Minute)); !r {
		t.Error("el corte no debe bajar")
	}
	// Vencido: se olvida.
	now = now.Add(2 * time.Hour)
	if r, _ := m.IsRevoked(ctx, "u1", "j1", now.Add(-3*time.Hour)); r {
		t.Error("lo vencido se olvida")
	}
	if r, _ := m.IsRevoked(ctx, "u2", "x", now.Add(-3*time.Hour)); r {
		t.Error("corte vencido")
	}
	// Token ya vencido: no se guarda.
	_ = m.RevokeToken(ctx, "old", now.Add(-time.Second))
	if len(m.jti) != 0 {
		t.Error("no se guarda lo ya vencido")
	}
	if m.RevokeToken(ctx, "", now) == nil || m.RevokeUserBefore(ctx, "", now, time.Minute) == nil || m.RevokeUserBefore(ctx, "u", now, 0) == nil {
		t.Error("entradas inválidas")
	}
}

func TestRevokedFrom(t *testing.T) {
	iat := time.Unix(1000, 0)
	cases := []struct {
		jti    bool
		cutoff any
		want   bool
		err    bool
	}{
		{true, nil, true, false},
		{false, nil, false, false},
		{false, "1000", true, false}, // emitido en el mismo segundo del corte
		{false, "999", false, false},
		{false, "x", false, true},
		{false, 5, false, true},
	}
	for _, c := range cases {
		got, err := revokedFrom(c.jti, c.cutoff, iat)
		if got != c.want || (err != nil) != c.err {
			t.Errorf("revokedFrom(%v, %v) = %v, %v", c.jti, c.cutoff, got, err)
		}
	}
}
