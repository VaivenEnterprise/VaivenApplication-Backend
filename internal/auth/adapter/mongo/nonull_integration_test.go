//go:build integration

package mongo

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
)

func rawByID(t *testing.T, col *drv.Collection, id string) bson.Raw {
	t.Helper()
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		t.Fatal(err)
	}
	var raw bson.Raw
	if err := col.FindOne(context.Background(), bson.D{{Key: "_id", Value: oid}}).Decode(&raw); err != nil {
		t.Fatalf("FindOne: %v", err)
	}
	return raw
}

// Una cuenta y un refresh nuevos, y las actualizaciones de perfil y de baja,
// no dejan ningún null en MongoDB (spec 021, T003, RF-10).
func TestIntegrationAuthWritesNoNulls(t *testing.T) {
	ctx := context.Background()
	db := newSchemaDB(t)
	repo := NewUserRepo(db)
	now := time.Now().UTC().Truncate(time.Millisecond)

	u, err := repo.Create(ctx, newUser("sin-nulls@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := nullPaths(t, rawByID(t, repo.col, u.ID), ""); len(got) != 0 {
		t.Fatalf("cuenta nueva con null en %v", got)
	}

	phone := "+525512345678"
	if _, err := repo.UpdateProfile(ctx, u.ID, usecase.ProfileChanges{Phone: &phone}, now); err != nil {
		t.Fatal(err)
	}
	got, err := repo.UpdateProfile(ctx, u.ID, usecase.ProfileChanges{ClearPhone: true}, now)
	if err != nil || got.Phone != nil {
		t.Fatalf("ClearPhone = %+v, %v", got, err)
	}
	raw := rawByID(t, repo.col, u.ID)
	if _, err := raw.LookupErr("phone"); err == nil {
		t.Error("ClearPhone debe quitar el campo phone")
	}
	if n := nullPaths(t, raw, ""); len(n) != 0 {
		t.Errorf("tras ClearPhone hay null en %v", n)
	}

	if err := repo.Deactivate(ctx, u.ID, "deleted+x@vaiven.invalid", now); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	raw = rawByID(t, repo.col, u.ID)
	if n := nullPaths(t, raw, ""); len(n) != 0 {
		t.Errorf("tras la baja hay null en %v", n)
	}
	if _, err := raw.LookupErr("phone"); err == nil {
		t.Error("la baja debe dejar phone ausente")
	}

	// Un documento viejo con null se sigue aceptando y leyendo (despliegue A
	// tolerante).
	old := bson.D{
		{Key: "_id", Value: bson.NewObjectID()}, {Key: "email", Value: "viejo@example.com"}, {Key: "passwordHash", Value: "h"},
		{Key: "displayName", Value: "Viejo"}, {Key: "phone", Value: nil}, {Key: "role", Value: "user"}, {Key: "status", Value: "active"},
		{Key: "acceptedTermsAt", Value: now}, {Key: "ratingSum", Value: int64(0)}, {Key: "ratingCount", Value: int64(0)},
		{Key: "suspendedUntil", Value: nil}, {Key: "createdAt", Value: now}, {Key: "updatedAt", Value: now},
		{Key: "version", Value: int64(1)}, {Key: "deactivatedAt", Value: nil},
	}
	if _, err := repo.col.InsertOne(ctx, old); err != nil {
		t.Fatalf("insertar documento viejo con null (el validador debe ser tolerante): %v", err)
	}
	if u, err := repo.ByEmail(ctx, "viejo@example.com"); err != nil || u.Phone != nil || u.SuspendedUntil != nil {
		t.Errorf("leer documento viejo = %+v, %v", u, err)
	}

	// Refresh token nuevo.
	rr := NewRefreshRepo(db)
	tok := domain.RefreshToken{UserID: u.ID, FamilyID: "f1", TokenHash: "h-nonull", ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	if err := rr.Create(ctx, tok); err != nil {
		t.Fatal(err)
	}
	var rawTok bson.Raw
	if err := rr.col.FindOne(ctx, bson.D{{Key: "tokenHash", Value: "h-nonull"}}).Decode(&rawTok); err != nil {
		t.Fatal(err)
	}
	if n := nullPaths(t, rawTok, ""); len(n) != 0 {
		t.Errorf("refresh nuevo con null en %v", n)
	}
}
