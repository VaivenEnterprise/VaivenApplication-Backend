//go:build integration

package mongo

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
)

func count(t *testing.T, e *env, col string, user string) int64 {
	t.Helper()
	n, err := e.db.Collection(col).CountDocuments(context.Background(), bson.D{{Key: "userId", Value: user}})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Spec 021, T018 (CA-01, POL-17): se borran los avisos y los tokens de push de
// la persona y solo los suyos; el worker de push no encuentra tokens que enviar.
func TestIntegrationEraserDeletesNotificationsAndTokens(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	er := NewEraser(e.db)
	for i := 0; i < 4; i++ {
		save(t, e, "ana", "booking_requested")
	}
	save(t, e, "beto", "booking_requested")
	now := time.Now().UTC()
	for _, tk := range []domain.PushToken{
		{Token: "ExponentPushToken[ana-1]", UserID: "ana", Platform: "ios", LastSeenAt: now, CreatedAt: now},
		{Token: "ExponentPushToken[ana-2]", UserID: "ana", Platform: "android", LastSeenAt: now, CreatedAt: now},
		{Token: "ExponentPushToken[beto-1]", UserID: "beto", Platform: "ios", LastSeenAt: now, CreatedAt: now},
	} {
		if err := e.tokens.Upsert(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}

	res, err := er.Erase(ctx, "ana", now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts["notifications.deleted"] != 4 || res.Counts["push_tokens.deleted"] != 2 || len(res.Notices)+len(res.Images) != 0 {
		t.Fatalf("resultado = %+v", res)
	}
	if count(t, e, NotificationsCollection, "ana") != 0 || count(t, e, PushTokensCollection, "ana") != 0 {
		t.Error("no debe quedar nada de Ana")
	}
	if count(t, e, NotificationsCollection, "beto") != 1 || count(t, e, PushTokensCollection, "beto") != 1 {
		t.Error("los de otra persona no se tocan")
	}
	// El worker de push ve una cuenta sin tokens.
	if tokens, err := e.tokens.TokensOf(ctx, "ana"); err != nil || len(tokens) != 0 {
		t.Errorf("TokensOf(ana) = %v, %v", tokens, err)
	}
	if page, err := e.notifs.List(ctx, "ana", "", 10); err != nil || len(page) != 0 {
		t.Errorf("List(ana) = %v, %v", page, err)
	}

	again, err := er.Erase(ctx, "ana", now)
	if err != nil || again.Counts["notifications.deleted"] != 0 || again.Counts["push_tokens.deleted"] != 0 {
		t.Errorf("segunda ejecución = %+v, %v", again, err)
	}
	if er.Name() != "notifications" {
		t.Errorf("Name = %q", er.Name())
	}
}
