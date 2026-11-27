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

func TestIncidentDocWithoutOptionalFieldsHasNoNull(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	noNull(t, incidentDoc{ID: bson.NewObjectID(), BookingID: "b", ReporterID: "r", ReportedID: "e", Reason: "x", Status: "open", Evidence: []imageDoc{}, Notes: []noteDoc{}, CreatedAt: now, UpdatedAt: now, Version: 1},
		"finalResponse", "closedAt")
	b, _ := bson.Marshal(incidentDoc{Final: &finalDoc{Text: "ok"}, ClosedAt: &now})
	for _, k := range []string{"finalResponse", "closedAt"} {
		if _, err := bson.Raw(b).LookupErr(k); err != nil {
			t.Errorf("%s con valor debe guardarse: %v", k, err)
		}
	}
	old, _ := bson.Marshal(bson.D{{Key: "status", Value: "open"}, {Key: "finalResponse", Value: nil}, {Key: "closedAt", Value: nil}})
	for name, src := range map[string][]byte{"viejo con null": old, "nuevo sin campo": mustMarshal(t, incidentDoc{})} {
		var d incidentDoc
		if err := bson.Unmarshal(src, &d); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if i := d.toDomain(); i.Final != nil || i.ClosedAt != nil {
			t.Errorf("%s: opcionales deben ser nil", name)
		}
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := bson.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
