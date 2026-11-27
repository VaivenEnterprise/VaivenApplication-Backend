package mongo

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Una calificación sin comentario no guarda comment como null (spec 021,
// T006) y los lectores aceptan el documento nuevo y el viejo con null.
func TestRatingDocWithoutCommentHasNoNull(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	b, err := bson.Marshal(ratingDoc{ID: bson.NewObjectID(), BookingID: "b", RaterID: "r", RateeID: "e", RatedRole: "owner", Score: 5, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bson.Raw(b).LookupErr("comment"); err == nil {
		t.Error("comment debe estar ausente sin comentario")
	}
	comment := "Excelente"
	b2, _ := bson.Marshal(ratingDoc{Comment: &comment})
	if _, err := bson.Raw(b2).LookupErr("comment"); err != nil {
		t.Errorf("comment con valor debe guardarse: %v", err)
	}

	oldRaw, err := bson.Marshal(bson.D{{Key: "score", Value: 4}, {Key: "comment", Value: nil}})
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string][]byte{"nuevo sin campo": b, "viejo con null": oldRaw} {
		var got ratingDoc
		if err := bson.Unmarshal(src, &got); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if r := got.toDomain(); r.Comment != nil {
			t.Errorf("%s: Comment = %v, want nil", name, r.Comment)
		}
	}
}
