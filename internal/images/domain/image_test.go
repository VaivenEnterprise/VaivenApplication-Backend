package domain

import "testing"

func TestPurposeAndDelivery(t *testing.T) {
	if !PurposeItemPhoto.Valid() || !PurposeEvidence.Valid() || Purpose("avatar").Valid() || Purpose("").Valid() {
		t.Error("Valid")
	}
	if PurposeItemPhoto.Delivery() != DeliveryPublic || PurposeEvidence.Delivery() != DeliveryAuthenticated {
		t.Error("Delivery por propósito (P-07)")
	}
	if DeliveryPublic.CloudinaryType() != "upload" || DeliveryAuthenticated.CloudinaryType() != "authenticated" {
		t.Error("CloudinaryType (RF-02)")
	}
}

func TestAllowedFormat(t *testing.T) {
	for _, f := range []string{"jpg", "jpeg", "png", "webp"} {
		if !AllowedFormat(f) {
			t.Errorf("%s debe estar permitido", f)
		}
	}
	for _, f := range []string{"gif", "heic", "JPG", "", "jpg,png", "svg"} {
		if AllowedFormat(f) {
			t.Errorf("%q no debe estar permitido", f)
		}
	}
}

func TestPublicIDAndOwnership(t *testing.T) {
	const base = "vaiven/staging"
	id := NewPublicID(base+"/", PurposeItemPhoto, "u1", "abc")
	if id != "vaiven/staging/item_photo/u1/abc" {
		t.Fatalf("NewPublicID = %q", id)
	}
	tests := []struct {
		name     string
		publicID string
		purpose  Purpose
		user     string
		want     bool
	}{
		{"propio", id, PurposeItemPhoto, "u1", true},
		{"de otra persona", id, PurposeItemPhoto, "u2", false},
		{"prefijo de usuario parecido", "vaiven/staging/item_photo/u10/abc", PurposeItemPhoto, "u1", false},
		{"otro propósito", id, PurposeEvidence, "u1", false},
		{"otra base", "vaiven/production/item_photo/u1/abc", PurposeItemPhoto, "u1", false},
		{"sin archivo", "vaiven/staging/item_photo/u1/", PurposeItemPhoto, "u1", false},
		{"subcarpeta", "vaiven/staging/item_photo/u1/x/abc", PurposeItemPhoto, "u1", false},
		{"punto punto", "vaiven/staging/item_photo/u1/..", PurposeItemPhoto, "u1", false},
		{"punto", "vaiven/staging/item_photo/u1/.", PurposeItemPhoto, "u1", false},
		{"barra invertida", "vaiven/staging/item_photo/u1/a\\b", PurposeItemPhoto, "u1", false},
		{"usuario vacío", "vaiven/staging/item_photo//abc", PurposeItemPhoto, "", false},
		{"usuario con barra", "vaiven/staging/item_photo/u1/abc", PurposeItemPhoto, "item_photo/u1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OwnedBy(tt.publicID, base, tt.purpose, tt.user); got != tt.want {
				t.Errorf("OwnedBy = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	if ErrInvalidPurpose.Code != "validation_failed" || len(ErrInvalidPurpose.Details) != 1 {
		t.Errorf("ErrInvalidPurpose = %+v", ErrInvalidPurpose)
	}
	if ErrUnavailable.Code != "service_unavailable" || ErrUnavailable.Message == "" {
		t.Errorf("ErrUnavailable = %+v", ErrUnavailable)
	}
}
