package mongo

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
)

// nullPaths devuelve las rutas con un valor BSON null en cualquier nivel
// (spec 021, RF-10). Los arreglos se anotan con "[]".
func nullPaths(t *testing.T, raw bson.Raw, prefix string) []string {
	t.Helper()
	elems, err := raw.Elements()
	if err != nil {
		t.Fatalf("BSON inválido: %v", err)
	}
	var out []string
	for _, el := range elems {
		out = append(out, nullPathsOfValue(t, el.Value(), prefix+"."+el.Key())...)
	}
	return out
}

func nullPathsOfValue(t *testing.T, v bson.RawValue, path string) []string {
	t.Helper()
	switch v.Type {
	case bson.TypeNull:
		return []string{path}
	case bson.TypeEmbeddedDocument:
		return nullPaths(t, v.Document(), path)
	case bson.TypeArray:
		vals, err := v.Array().Values()
		if err != nil {
			t.Fatalf("arreglo inválido: %v", err)
		}
		var out []string
		for _, it := range vals {
			out = append(out, nullPathsOfValue(t, it, path+"[]")...)
		}
		return out
	}
	return nil
}

func sampleBooking() domain.Booking {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	return domain.Booking{
		ID: bson.NewObjectID().Hex(), ItemID: "i1", OwnerID: "o1", RequesterID: "r1",
		Start: domain.MustDate("2026-12-01"), End: domain.MustDate("2026-12-03"), Days: 3,
		Item:   domain.ItemSnapshot{Title: "Taladro", Modality: domain.Modality("rent")},
		Price:  domain.PriceSnapshot{Modality: domain.Modality("rent"), PricePerDayCents: 1000, TotalCents: 3000, Currency: "MXN"},
		Status: domain.Status("pending"), CreatedAt: now, UpdatedAt: now, Version: 1,
	}
}

// Una reserva sin opcionales (todos nil) no guarda ningún null (spec 021, T004).
func TestBookingDocWithoutOptionalFieldsHasNoNull(t *testing.T) {
	d, err := toDoc(sampleBooking())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if got := nullPaths(t, raw, ""); len(got) != 0 {
		t.Errorf("la reserva guarda null en %v", got)
	}
	for _, k := range []string{"expiresAt", "meetingPoint", "cancelReason", "cancelledBy", "rejectReason", "conversationId", "delivery", "return", "lateAt", "affectedBy"} {
		if _, err := bson.Raw(raw).LookupErr(k); err == nil {
			t.Errorf("el campo opcional %q debe estar ausente", k)
		}
	}
	if _, err := bson.Raw(raw).LookupErr("itemSnapshot", "firstPhoto"); err == nil {
		t.Error("itemSnapshot.firstPhoto debe estar ausente sin foto")
	}
	if _, err := bson.Raw(raw).LookupErr("itemSnapshot", "usageRules"); err == nil {
		t.Error("itemSnapshot.usageRules debe estar ausente sin reglas")
	}
}

// Una entrega parcial (solo registrada, sin QR ni confirmación) y un pago sin
// reporte tampoco guardan null en los subdocumentos.
func TestBookingDocNestedOptionalsHaveNoNull(t *testing.T) {
	b := sampleBooking()
	rec := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	b.Delivery = &domain.Handover{RecordedAt: &rec, ConditionNotes: "ok"}
	b.Payments = []domain.BookingPayment{{Stage: domain.Stage("deposit"), AmountCents: 500, Status: domain.PaymentStatus("pending")}}
	d, err := toDoc(b)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if got := nullPaths(t, raw, ""); len(got) != 0 {
		t.Errorf("la reserva guarda null en %v", got)
	}
	for _, path := range [][]string{{"delivery", "qr"}, {"delivery", "confirmedAt"}, {"delivery", "recordedById"}} {
		if _, err := bson.Raw(raw).LookupErr(path...); err == nil {
			t.Errorf("%v debe estar ausente", path)
		}
	}
	qr := EncodeQR(domain.QR{TokenHash: "h", ExpiresAt: rec})
	qraw, _ := bson.Marshal(qr)
	if got := nullPaths(t, qraw, ""); len(got) != 0 {
		t.Errorf("QR guarda null en %v", got)
	}
}

// Los lectores aceptan una reserva vieja con null y una nueva sin los campos.
func TestBookingDocReadsOldNullAndAbsentFields(t *testing.T) {
	d, err := toDoc(sampleBooking())
	if err != nil {
		t.Fatal(err)
	}
	newRaw, _ := bson.Marshal(d)
	var old bson.D
	if err := bson.Unmarshal(newRaw, &old); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"expiresAt", "meetingPoint", "cancelReason", "cancelledBy", "rejectReason", "conversationId", "delivery", "return", "lateAt", "affectedBy"} {
		old = append(old, bson.E{Key: k, Value: nil})
	}
	oldRaw, err := bson.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"viejo con null": oldRaw, "nuevo sin campo": newRaw} {
		var got bookingDoc
		if err := bson.Unmarshal(raw, &got); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b := got.toDomain()
		if b.ExpiresAt != nil || b.MeetingPoint != nil || b.CancelledBy != nil || b.Delivery != nil || b.Return != nil || b.LateAt != nil || b.AffectedBy != nil || b.ConversationID != nil {
			t.Errorf("%s: los opcionales deben leerse como nil: %+v", name, b)
		}
	}
}
