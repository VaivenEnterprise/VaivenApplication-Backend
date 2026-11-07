//go:build integration

package mongo

import (
	"context"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

func TestIntegrationRefreshRepo(t *testing.T) {
	ctx := context.Background()
	repo := NewRefreshRepo(newDB(t))
	for i := 0; i < 2; i++ {
		if err := repo.EnsureIndexes(ctx); err != nil {
			t.Fatalf("EnsureIndexes (vez %d): %v", i+1, err)
		}
	}

	specs, err := repo.col.Indexes().ListSpecifications(ctx)
	if err != nil {
		t.Fatalf("ListSpecifications: %v", err)
	}
	byName := map[string]bool{}
	for _, s := range specs {
		switch {
		case s.Name == "tokenHash_unique" && s.Unique != nil && *s.Unique:
			byName[s.Name] = true
		case s.Name == "expiresAt_ttl" && s.ExpireAfterSeconds != nil && *s.ExpireAfterSeconds == 0:
			byName[s.Name] = true
		case s.Name == "familyId" || s.Name == "userId":
			byName[s.Name] = true
		}
	}
	for _, n := range []string{"tokenHash_unique", "expiresAt_ttl", "familyId", "userId"} {
		if !byName[n] {
			t.Errorf("falta el índice %s", n)
		}
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	tok := func(hash, family, user string) domain.RefreshToken {
		return domain.RefreshToken{UserID: user, FamilyID: family, TokenHash: hash, ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	}
	for _, rt := range []domain.RefreshToken{tok("h1", "f1", "u1"), tok("h2", "f1", "u1"), tok("h3", "f2", "u1"), tok("h4", "f3", "u2")} {
		if err := repo.Create(ctx, rt); err != nil {
			t.Fatalf("Create %s: %v", rt.TokenHash, err)
		}
	}
	if err := repo.Create(ctx, tok("h1", "f9", "u9")); err == nil {
		t.Error("Create con hash repetido debe fallar (índice único)")
	}

	got, err := repo.ByHash(ctx, "h1")
	if err != nil {
		t.Fatalf("ByHash: %v", err)
	}
	if got.FamilyID != "f1" || got.UserID != "u1" || !got.Usable(now) || !got.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Errorf("ByHash = %+v", got)
	}
	if _, err := repo.ByHash(ctx, "no-existe"); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("ByHash inexistente = %v, want ErrInvalidSession", err)
	}

	// Marcar usado es atómico: la segunda vez no modifica.
	ok, err := repo.MarkUsed(ctx, got.ID, now)
	if err != nil || !ok {
		t.Fatalf("MarkUsed = %v, %v; want true", ok, err)
	}
	if ok, _ := repo.MarkUsed(ctx, got.ID, now); ok {
		t.Error("MarkUsed dos veces devolvió true")
	}
	if _, err := repo.MarkUsed(ctx, "no-es-id", now); err == nil {
		t.Error("MarkUsed con id inválido debe fallar")
	}
	used, _ := repo.ByHash(ctx, "h1")
	if !used.Reused() || used.Usable(now) {
		t.Errorf("token usado: Reused=%v Usable=%v", used.Reused(), used.Usable(now))
	}

	// Revocar la familia f1 no toca f2 ni a u2.
	if err := repo.RevokeFamily(ctx, "f1", now); err != nil {
		t.Fatalf("RevokeFamily: %v", err)
	}
	for hash, revoked := range map[string]bool{"h1": true, "h2": true, "h3": false, "h4": false} {
		rt, _ := repo.ByHash(ctx, hash)
		if (rt.RevokedAt != nil) != revoked {
			t.Errorf("%s revocado = %v, want %v", hash, rt.RevokedAt != nil, revoked)
		}
	}
	// Un token revocado no se puede marcar como usado.
	h2, _ := repo.ByHash(ctx, "h2")
	if ok, _ := repo.MarkUsed(ctx, h2.ID, now); ok {
		t.Error("MarkUsed sobre un token revocado devolvió true")
	}

	if err := repo.RevokeAllForUser(ctx, "u1", now); err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	h3, _ := repo.ByHash(ctx, "h3")
	h4, _ := repo.ByHash(ctx, "h4")
	if h3.RevokedAt == nil || h4.RevokedAt != nil {
		t.Errorf("RevokeAllForUser: h3 revocado=%v (want true), h4 revocado=%v (want false)", h3.RevokedAt != nil, h4.RevokedAt != nil)
	}
}
