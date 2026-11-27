//go:build integration

package mongo

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
)

// Spec 021, T019 (CA-01): se borran las conversaciones de la persona con el
// asistente y las de nadie más.
func TestIntegrationEraserDeletesConversations(t *testing.T) {
	db := testDB(t)
	repo := NewConversationRepo(db)
	er := NewEraser(db)
	ctx := context.Background()
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 26, 18, 0, 0, 0, time.UTC)
	for _, c := range []struct{ user, text string }{
		{"ana", "quiero pintar mi casa"}, {"ana", "¿y un taladro?"}, {"beto", "necesito una escalera"},
	} {
		if _, err := repo.Create(ctx, c.user, []aidomain.Turn{turn("user", c.text)}, t0); err != nil {
			t.Fatal(err)
		}
	}
	count := func(user string) int64 {
		n, err := db.Collection(ConversationsCollection).CountDocuments(ctx, bson.D{{Key: "userId", Value: user}})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	res, err := er.Erase(ctx, "ana", t0)
	if err != nil || res.Counts["ai_conversations.deleted"] != 2 {
		t.Fatalf("Erase = %+v, %v", res, err)
	}
	if count("ana") != 0 || count("beto") != 1 {
		t.Errorf("ana=%d beto=%d, want 0 y 1", count("ana"), count("beto"))
	}
	if again, err := er.Erase(ctx, "ana", t0); err != nil || again.Counts["ai_conversations.deleted"] != 0 {
		t.Errorf("segunda ejecución = %+v, %v", again, err)
	}
	if er.Name() != "assistant" {
		t.Errorf("Name = %q", er.Name())
	}
}
