//go:build integration

package mongo

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/assistant/domain"
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
	return client.Database("vaiven_assistant_test")
}

func turn(role, text string) aidomain.Turn { return aidomain.Turn{Role: role, Text: text} }

func TestIntegrationConversations(t *testing.T) {
	db := testDB(t)
	repo := NewConversationRepo(db)
	ctx := context.Background()
	for i := 0; i < 2; i++ { // idempotente
		if err := repo.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
	}
	t0 := time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC)

	id, err := repo.Create(ctx, "u1", []aidomain.Turn{turn("user", "quiero pintar"), turn("model", "necesitas rodillo")}, t0)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("dueño y CA-13", func(t *testing.T) {
		c, err := repo.Get(ctx, id, "u1")
		if err != nil || c.ID != id || c.UserID != "u1" || len(c.Turns) != 2 || c.Turns[1].Text != "necesitas rodillo" {
			t.Fatalf("Get = %+v, %v", c, err)
		}
		for _, call := range []func() error{
			func() error { _, err := repo.Get(ctx, id, "u2"); return err },
			func() error { return repo.Append(ctx, id, "u2", []aidomain.Turn{turn("user", "x")}, t0) },
			func() error { return repo.Delete(ctx, id, "u2") },
			func() error { _, err := repo.Get(ctx, "no-es-un-id", "u1"); return err },
			func() error { _, err := repo.Get(ctx, bson.NewObjectID().Hex(), "u1"); return err },
		} {
			if err := call(); !errors.Is(err, domain.ErrConversationNotFound) {
				t.Errorf("otra persona o id inválido: err = %v", err)
			}
		}
	})

	t.Run("máximo 6 turnos y textos acotados", func(t *testing.T) {
		for i := 0; i < 4; i++ {
			if err := repo.Append(ctx, id, "u1", []aidomain.Turn{turn("user", "m"), turn("model", "r")}, t0.Add(time.Duration(i+1)*time.Minute)); err != nil {
				t.Fatal(err)
			}
		}
		if err := repo.Append(ctx, id, "u1", []aidomain.Turn{turn("model", strings.Repeat("é", 3000))}, t0.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		c, _ := repo.Get(ctx, id, "u1")
		if len(c.Turns) != domain.MaxTurns {
			t.Fatalf("turnos = %d, want %d", len(c.Turns), domain.MaxTurns)
		}
		if n := utf8.RuneCountInString(c.Turns[domain.MaxTurns-1].Text); n != domain.MaxTurnRunes {
			t.Errorf("texto del último turno = %d runas, want %d", n, domain.MaxTurnRunes)
		}
		var doc bson.M
		_ = db.Collection(ConversationsCollection).FindOne(ctx, bson.D{}).Decode(&doc)
		created, _ := doc["createdAt"].(bson.DateTime)
		updated, _ := doc["updatedAt"].(bson.DateTime)
		if created.Time().UTC() != t0 || updated.Time().UTC() != t0.Add(time.Hour) {
			t.Errorf("createdAt=%v updatedAt=%v", created.Time(), updated.Time())
		}
		if _, ok := doc["location"]; ok {
			t.Error("la conversación nunca guarda la ubicación")
		}
	})

	t.Run("crear con más de 6 turnos conserva los últimos", func(t *testing.T) {
		var many []aidomain.Turn
		for i := 0; i < 9; i++ {
			many = append(many, turn("user", string(rune('a'+i))))
		}
		id2, err := repo.Create(ctx, "u1", many, t0)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := repo.Get(ctx, id2, "u1")
		if len(c.Turns) != domain.MaxTurns || c.Turns[0].Text != "d" {
			t.Errorf("turnos = %+v", c.Turns)
		}
	})

	t.Run("borrar", func(t *testing.T) {
		if err := repo.Delete(ctx, id, "u1"); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Get(ctx, id, "u1"); !errors.Is(err, domain.ErrConversationNotFound) {
			t.Errorf("tras borrar: %v", err)
		}
		if err := repo.Delete(ctx, id, "u1"); !errors.Is(err, domain.ErrConversationNotFound) {
			t.Errorf("borrar dos veces: %v", err)
		}
	})

	t.Run("TTL de 24 h sobre updatedAt", func(t *testing.T) {
		cur, err := db.Collection(ConversationsCollection).Indexes().List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var idx []bson.M
		_ = cur.All(ctx, &idx)
		found := false
		for _, i := range idx {
			if i["name"] == "updated_ttl" && i["expireAfterSeconds"] == int32(domain.ConversationTTLSeconds) {
				found = true
			}
		}
		if !found {
			t.Errorf("falta el TTL de 24 h: %v", idx)
		}
	})
}
