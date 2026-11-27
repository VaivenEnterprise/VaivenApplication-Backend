package mongo

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// noNull comprueba que el documento no guarda ningún null (spec 021, T007,
// RF-10) y que los campos opcionales indicados están ausentes.
func noNull(t *testing.T, doc any, absent ...string) bson.Raw {
	t.Helper()
	b, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	js, err := bson.MarshalExtJSON(doc, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(js), "null") {
		t.Errorf("el documento guarda null: %s", js)
	}
	for _, k := range absent {
		if _, err := bson.Raw(b).LookupErr(k); err == nil {
			t.Errorf("el campo opcional %q debe estar ausente", k)
		}
	}
	return b
}

func TestModelMetaDocWithoutBuiltAtHasNoNull(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	noNull(t, modelMetaDoc{ID: modelMetaID, BuildID: "", LockedUntil: now, CreatedAt: now, UpdatedAt: now}, "builtAt")
	noNull(t, popularDoc{ID: "b:k", BuildID: "b", Key: "k", Items: []string{}, CreatedAt: now, UpdatedAt: now})
	noNull(t, modelDoc{ID: "b:i", BuildID: "b", ItemID: "i", Neighbors: []neighborDoc{}, CreatedAt: now, UpdatedAt: now})
}
