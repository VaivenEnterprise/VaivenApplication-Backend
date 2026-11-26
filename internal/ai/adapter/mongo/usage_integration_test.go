//go:build integration

package mongo

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

func testDB(t *testing.T) *drv.Database {
	t.Helper()
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatalf("no se pudo iniciar MongoDB con Docker o Podman: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	uri, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	return client.Database("vaiven_ai_test")
}

func TestIntegrationUsageLimitIsAtomic(t *testing.T) {
	db := testDB(t)
	repo := NewUsageRepo(db)
	ctx := context.Background()
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC)

	// 50 peticiones simultáneas con límite 20: exactamente 20 pasan.
	const limit, workers = 20, 50
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.Take(ctx, "u1", "assistant", "2026-11-16", limit, now)
			if err != nil {
				t.Errorf("Take: %v", err)
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != limit {
		t.Fatalf("permitidas = %d, want %d", allowed.Load(), limit)
	}

	var doc bson.M
	if err := db.Collection(UsageCollection).FindOne(ctx, bson.D{{Key: "userId", Value: "u1"}}).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["count"] != int32(limit) || doc["createdAt"] == nil || doc["updatedAt"] == nil {
		t.Errorf("documento = %v", doc)
	}
	if n, _ := db.Collection(UsageCollection).CountDocuments(ctx, bson.D{}); n != 1 {
		t.Errorf("documentos = %d, want 1 (sin duplicados)", n)
	}
}

func TestIntegrationUsageIsPerUserFeatureAndDay(t *testing.T) {
	db := testDB(t)
	repo := NewUsageRepo(db)
	ctx := context.Background()
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	take := func(user, feature, day string) bool {
		ok, err := repo.Take(ctx, user, feature, day, 1, now)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !take("u1", "suggest", "2026-11-16") || take("u1", "suggest", "2026-11-16") {
		t.Fatal("el segundo uso del mismo día debe rechazarse con límite 1")
	}
	if !take("u1", "assistant", "2026-11-16") || !take("u2", "suggest", "2026-11-16") || !take("u1", "suggest", "2026-11-17") {
		t.Error("otra función, otra persona u otro día tienen su propio cupo")
	}
	if ok, err := repo.Take(ctx, "u3", "suggest", "2026-11-16", 0, now); ok || err != nil {
		t.Errorf("límite 0: ok=%v err=%v", ok, err)
	}
}

func TestIntegrationUsageIndexes(t *testing.T) {
	db := testDB(t)
	repo := NewUsageRepo(db)
	ctx := context.Background()
	for i := 0; i < 2; i++ { // idempotente
		if err := repo.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
	}
	cur, err := db.Collection(UsageCollection).Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var idx []bson.M
	if err := cur.All(ctx, &idx); err != nil {
		t.Fatal(err)
	}
	found := map[string]bson.M{}
	for _, i := range idx {
		found[i["name"].(string)] = i
	}
	if u := found["user_feature_day_unique"]; u == nil || u["unique"] != true {
		t.Errorf("falta el índice único: %v", found)
	}
	if ttl := found["created_ttl"]; ttl == nil || ttl["expireAfterSeconds"] != int32(UsageRetentionSeconds) {
		t.Errorf("falta el TTL de 3 días: %v", found)
	}
}
