package auth

import (
	"context"
	"testing"
)

func TestIdentity(t *testing.T) {
	ctx := context.Background()
	if id, ok := UserID(ctx); ok || id != "" || Role(ctx) != "" {
		t.Errorf("sin identidad: UserID = %q, %v; Role = %q", id, ok, Role(ctx))
	}
	ctx = WithIdentity(ctx, Identity{UserID: "u1", Role: "admin"})
	if id, ok := UserID(ctx); !ok || id != "u1" || Role(ctx) != "admin" {
		t.Errorf("con identidad: UserID = %q, %v; Role = %q", id, ok, Role(ctx))
	}
	if _, ok := UserID(WithIdentity(context.Background(), Identity{Role: "user"})); ok {
		t.Error("una identidad sin UserID no cuenta como autenticada")
	}
}
