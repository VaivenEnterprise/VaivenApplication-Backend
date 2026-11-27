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

func TestReportDocWithoutOptionalFieldsHasNoNull(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	noNull(t, reportDoc{ID: bson.NewObjectID(), ReporterID: "r", TargetType: "item", TargetID: "i", Reason: "x", Status: "open", CreatedAt: now}, "details", "resolution")
	old, _ := bson.Marshal(bson.D{{Key: "status", Value: "open"}, {Key: "details", Value: nil}, {Key: "resolution", Value: nil}})
	var d reportDoc
	if err := bson.Unmarshal(old, &d); err != nil {
		t.Fatal(err)
	}
	if r := d.toDomain(); r.Details != nil || r.Resolution != nil {
		t.Error("los opcionales de un documento viejo con null deben leerse como nil")
	}
}
