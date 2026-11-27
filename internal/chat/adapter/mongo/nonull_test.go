package mongo

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Una conversación sin mensajes no guarda lastMessage como null (spec 021,
// T006) y los lectores aceptan el documento nuevo y el viejo con null.
func TestConversationDocWithoutLastMessageHasNoNull(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	d := conversationDoc{
		ID: bson.NewObjectID(), BookingID: "b1", ParticipantIDs: []string{"a", "b"},
		Unread: map[string]int{}, LastMessageAt: now, CreatedAt: now,
	}
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bson.Raw(b).LookupErr("lastMessage"); err == nil {
		t.Error("lastMessage debe estar ausente sin mensajes")
	}
	if v := bson.Raw(b).Lookup("unread"); v.Type != bson.TypeEmbeddedDocument {
		t.Errorf("unread = %v, want documento vacío (nunca null)", v.Type)
	}

	oldDoc := bson.D{{Key: "bookingId", Value: "b1"}, {Key: "participantIds", Value: bson.A{"a", "b"}}, {Key: "unread", Value: bson.D{}}, {Key: "lastMessage", Value: nil}}
	oldRaw, err := bson.Marshal(oldDoc)
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string][]byte{"nuevo sin campo": b, "viejo con null": oldRaw} {
		var got conversationDoc
		if err := bson.Unmarshal(src, &got); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c := got.toDomain(); c.LastMessage != nil || c.Unread == nil {
			t.Errorf("%s: LastMessage=%v Unread=%v", name, c.LastMessage, c.Unread)
		}
	}
}

func TestConversationDocWithLastMessageKeepsIt(t *testing.T) {
	d := conversationDoc{LastMessage: &lastMessageDoc{Preview: "hola", At: time.Now(), SenderID: "a"}}
	b, _ := bson.Marshal(d)
	if _, err := bson.Raw(b).LookupErr("lastMessage", "preview"); err != nil {
		t.Errorf("lastMessage.preview ausente: %v", err)
	}
}
