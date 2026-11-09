//go:build integration

package mongo

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/domain"
)

var testClient *mongodb.Client

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "no se pudo iniciar MongoDB con Docker o Podman:", err)
		os.Exit(1)
	}
	uri, err := c.ConnectionString(ctx)
	if err == nil {
		testClient, err = mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "MongoDB:", err)
		_ = c.Terminate(ctx)
		os.Exit(1)
	}
	code := m.Run()
	_ = testClient.Disconnect(ctx)
	_ = c.Terminate(ctx)
	os.Exit(code)
}

func TestIntegrationEnsureIndexesAndCreate(t *testing.T) {
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("sanctions_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := NewRepo(db)
	for i := 0; i < 2; i++ { // idempotente
		if err := repo.EnsureIndexes(ctx); err != nil {
			t.Fatalf("EnsureIndexes #%d: %v", i+1, err)
		}
	}
	cur, err := repo.col.Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var idx []struct {
		Name string `bson:"name"`
	}
	if err := cur.All(ctx, &idx); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, i := range idx {
		found = found || i.Name == "user_createdAt"
	}
	if !found || len(idx) != 2 {
		t.Errorf("índices = %v", idx)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	ends := now.Add(7 * 24 * time.Hour)
	incident := bson.NewObjectID().Hex()
	susp := domain.Sanction{
		ID: repo.NewID(), UserID: "6730f1c2a9b3e4d5f6a7b8c9", Type: domain.TypeSuspension, Reason: "Daño reiterado.",
		EndsAt: &ends, IncidentID: &incident, CreatedByAdminID: "adm", CreatedAt: now,
	}
	got, err := repo.Create(ctx, susp)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != susp.ID || got.Type != domain.TypeSuspension || got.EndsAt == nil || !got.EndsAt.Equal(ends) ||
		got.IncidentID == nil || *got.IncidentID != incident || !got.CreatedAt.Equal(now) {
		t.Errorf("suspensión guardada = %+v", got)
	}

	warn := domain.Sanction{ID: repo.NewID(), UserID: susp.UserID, Type: domain.TypeWarning, Reason: "Llegó tarde.", CreatedByAdminID: "adm", CreatedAt: now.Add(time.Minute)}
	w, err := repo.Create(ctx, warn)
	if err != nil || w.EndsAt != nil || w.IncidentID != nil {
		t.Errorf("advertencia guardada = %+v (%v)", w, err)
	}
	if n, err := repo.col.CountDocuments(ctx, bson.M{"userId": susp.UserID}); err != nil || n != 2 {
		t.Errorf("sanciones de la persona = %d (%v)", n, err)
	}
	if _, err := repo.Create(ctx, domain.Sanction{ID: "malo"}); err == nil {
		t.Error("un id inválido debe fallar")
	}
}
