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

func TestSameValidation(t *testing.T) {
	want := bson.M{"$jsonSchema": ItemsSchema()}
	raw := func(v any) bson.Raw {
		b, err := bson.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	ok := raw(bson.M{"validator": want, "validationLevel": "moderate", "validationAction": "error"})
	if !sameValidation(ok, want) {
		t.Error("el mismo validador debe coincidir")
	}
	for name, opts := range map[string]bson.Raw{
		"vacío":      nil,
		"otro nivel": raw(bson.M{"validator": want, "validationLevel": "strict", "validationAction": "error"}),
		"otro":       raw(bson.M{"validator": bson.M{}, "validationLevel": "moderate", "validationAction": "error"}),
	} {
		if sameValidation(opts, want) {
			t.Errorf("%s: no debe coincidir", name)
		}
	}
}
