//go:build integration

package mongo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMessageReaderReference(t *testing.T) { // spec 011, RF-08 / CA-06
	db := testClient.Database(fmt.Sprintf("chat_ref_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	ctx := context.Background()

	convID, msgID, orphanID := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	now := time.Now().UTC()
	if _, err := db.Collection(ConversationsCollection).InsertOne(ctx, bson.M{
		"_id": convID, "bookingId": "b1", "participantIds": bson.A{"u-owner", "u-req"}, "createdAt": now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection(MessagesCollection).InsertMany(ctx, []any{
		bson.M{"_id": msgID, "conversationId": convID.Hex(), "senderId": "u-req", "seq": int64(1), "body": "Hola", "createdAt": now},
		bson.M{"_id": orphanID, "conversationId": bson.NewObjectID().Hex(), "senderId": "u-req", "seq": int64(1), "body": "Huérfano", "createdAt": now},
	}); err != nil {
		t.Fatal(err)
	}

	r := NewMessageReader(db)
	ref, found, err := r.Reference(ctx, msgID.Hex())
	if err != nil || !found {
		t.Fatalf("Reference: found=%v err=%v", found, err)
	}
	if ref.ID != msgID.Hex() || ref.SenderID != "u-req" || ref.Body != "Hola" || !ref.HasParticipant("u-owner") || !ref.HasParticipant("u-req") || ref.HasParticipant("intruso") {
		t.Errorf("referencia = %+v", ref)
	}
	for name, id := range map[string]string{"inexistente": bson.NewObjectID().Hex(), "id malo": "nope", "sin conversación": orphanID.Hex()} {
		if _, found, err := r.Reference(ctx, id); found || err != nil {
			t.Errorf("%s: found=%v err=%v; quiero false, nil", name, found, err)
		}
	}
}
