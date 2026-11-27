//go:build integration

package mongo

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Spec 021, T019 (CA-01): se borra el uso de IA de la persona y de nadie más.
func TestIntegrationEraserDeletesUsage(t *testing.T) {
	db := testDB(t)
	repo := NewUsageRepo(db)
	er := NewEraser(db)
	ctx := context.Background()
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 26, 18, 0, 0, 0, time.UTC)
	for _, c := range []struct{ user, feature, day string }{
		{"ana", "assistant", "2026-11-26"}, {"ana", "suggest", "2026-11-26"}, {"ana", "assistant", "2026-11-25"},
		{"beto", "assistant", "2026-11-26"},
	} {
		if ok, err := repo.Take(ctx, c.user, c.feature, c.day, 20, now); err != nil || !ok {
			t.Fatalf("Take = %v, %v", ok, err)
		}
	}
	count := func(user string) int64 {
		n, err := db.Collection(UsageCollection).CountDocuments(ctx, bson.D{{Key: "userId", Value: user}})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	res, err := er.Erase(ctx, "ana", now)
	if err != nil || res.Counts["ai_usage.deleted"] != 3 {
		t.Fatalf("Erase = %+v, %v", res, err)
	}
	if count("ana") != 0 || count("beto") != 1 {
		t.Errorf("ana=%d beto=%d, want 0 y 1", count("ana"), count("beto"))
	}
	if again, err := er.Erase(ctx, "ana", now); err != nil || again.Counts["ai_usage.deleted"] != 0 {
		t.Errorf("segunda ejecución = %+v, %v", again, err)
	}
	if er.Name() != "ai" {
		t.Errorf("Name = %q", er.Name())
	}
}
