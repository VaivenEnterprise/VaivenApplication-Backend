package mongo

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

func TestDocRoundTrip(t *testing.T) {
	nb, rules := "Del Carmen", "Limpio"
	now := time.Date(2026, 11, 8, 16, 0, 0, 0, time.UTC)
	in := domain.Item{
		ID: bson.NewObjectID().Hex(), OwnerID: "u1", Title: "Taladro", Description: "x",
		Category: "hogar", Condition: domain.ConditionNew, Modality: domain.ModalityLoan,
		DeclaredValueCents: 5_000, PriceSource: domain.PriceManual, UsageRules: &rules,
		Zone:     domain.Zone{Municipality: "Tlalpan", Neighborhood: &nb},
		Location: &domain.Location{Lat: 19.123, Lng: -99.129},
		Photos:   []images.Image{{PublicID: "p", Width: 1, Height: 2, Format: "png", Bytes: 3, Delivery: images.DeliveryPublic}},
		Status:   domain.StatusPaused, CreatedAt: now, UpdatedAt: now, Version: 3,
	}
	d, err := toDoc(in)
	if err != nil {
		t.Fatal(err)
	}
	if d.Location.Type != "Point" || d.Location.Coordinates[0] != -99.13 || d.Location.Coordinates[1] != 19.12 {
		t.Errorf("location = %+v (GeoJSON es [lng, lat] redondeado)", d.Location)
	}
	out := d.toDomain()
	in.Location = &domain.Location{Lat: 19.12, Lng: -99.13}
	if out.ID != in.ID || *out.Location != *in.Location || out.Photos[0] != in.Photos[0] ||
		*out.Zone.Neighborhood != nb || *out.UsageRules != rules || out.Status != in.Status || out.Version != 3 {
		t.Errorf("ida y vuelta = %+v", out)
	}

	in.Location = nil
	d, _ = toDoc(in)
	if d.Location != nil || d.toDomain().Location != nil {
		t.Error("sin ubicación no debe guardarse location")
	}
	if _, err := toDoc(domain.Item{ID: "zz"}); err == nil {
		t.Error("un id inválido debe fallar")
	}
}

// hasNull indica si el documento BSON tiene un null en cualquier nivel.
func hasNull(t *testing.T, raw bson.Raw) bool {
	t.Helper()
	elems, err := raw.Elements()
	if err != nil {
		t.Fatalf("BSON inválido: %v", err)
	}
	var walk func(v bson.RawValue) bool
	walk = func(v bson.RawValue) bool {
		switch v.Type {
		case bson.TypeNull:
			return true
		case bson.TypeEmbeddedDocument:
			return hasNull(t, v.Document())
		case bson.TypeArray:
			vals, _ := v.Array().Values()
			for _, it := range vals {
				if walk(it) {
					return true
				}
			}
		}
		return false
	}
	for _, el := range elems {
		if walk(el.Value()) {
			return true
		}
	}
	return false
}

// Un objeto sin reglas, barrio ni ubicación no guarda ningún null (spec 021,
// T005) y los lectores aceptan tanto el documento nuevo como el viejo con null.
func TestDocWithoutOptionalFieldsHasNoNull(t *testing.T) {
	now := time.Date(2026, 11, 8, 16, 0, 0, 0, time.UTC)
	in := domain.Item{
		ID: bson.NewObjectID().Hex(), OwnerID: "u1", Title: "Taladro", Description: "x",
		Category: "hogar", Condition: domain.ConditionNew, Modality: domain.ModalityLoan,
		DeclaredValueCents: 5_000, PriceSource: domain.PriceManual,
		Zone:   domain.Zone{Municipality: "Tlalpan"},
		Photos: []images.Image{{PublicID: "p"}}, Status: domain.StatusActive, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	d, err := toDoc(in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	raw := bson.Raw(b)
	if hasNull(t, raw) {
		t.Errorf("el objeto nuevo guarda null: %s", raw)
	}
	if _, err := raw.LookupErr("usageRules"); err == nil {
		t.Error("usageRules debe estar ausente")
	}
	if _, err := raw.LookupErr("zone", "neighborhood"); err == nil {
		t.Error("zone.neighborhood debe estar ausente")
	}
	if _, err := raw.LookupErr("location"); err == nil {
		t.Error("location debe estar ausente")
	}

	var old bson.D
	if err := bson.Unmarshal(b, &old); err != nil {
		t.Fatal(err)
	}
	old = append(old, bson.E{Key: "usageRules", Value: nil})
	for i := range old {
		if old[i].Key == "zone" {
			old[i].Value = bson.D{{Key: "municipality", Value: "Tlalpan"}, {Key: "neighborhood", Value: nil}}
		}
	}
	oldRaw, err := bson.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string][]byte{"nuevo sin campo": b, "viejo con null": oldRaw} {
		var got itemDoc
		if err := bson.Unmarshal(src, &got); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		it := got.toDomain()
		if it.UsageRules != nil || it.Zone.Neighborhood != nil || it.Location != nil {
			t.Errorf("%s: los opcionales deben leerse como nil: %+v", name, it)
		}
	}
}
