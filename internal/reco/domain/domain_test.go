package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

const oid = "6730f1c2a9b3e4d5f6a7b8c9"

func TestPriceBand(t *testing.T) {
	for _, tc := range []struct {
		modality string
		cents    int64
		want     string
	}{
		{"loan", 9_999, BandLoan},
		{"rent", 0, BandLoan},
		{"rent", 1, BandLe50},
		{"rent", 5_000, BandLe50},
		{"rent", 5_001, BandLe200},
		{"rent", 20_000, BandLe200},
		{"rent", 20_001, BandLe500},
		{"rent", 50_000, BandLe500},
		{"rent", 50_001, BandGt500},
	} {
		if got := PriceBand(tc.modality, tc.cents); got != tc.want {
			t.Errorf("PriceBand(%s, %d) = %s, want %s", tc.modality, tc.cents, got, tc.want)
		}
	}
}

func TestWeights(t *testing.T) {
	if EventView.Weight() != 1 || EventRequest.Weight() != 3 || EventType("otro").Weight() != 0 {
		t.Error("pesos de los tipos")
	}
	if Alpha <= 0 || Alpha >= 1 {
		t.Error("Alpha debe estar entre 0 y 1")
	}
}

func TestNewView(t *testing.T) {
	at := time.Date(2026, 11, 17, 12, 7, 30, 0, time.FixedZone("CST", -6*3600))
	e, err := NewView("u1", oid, "view", at)
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != EventView || e.Weight != WeightView || e.At.Location() != time.UTC || e.ItemID != oid || e.UserID != "u1" {
		t.Errorf("evento = %+v", e)
	}
	if want := time.Date(2026, 11, 17, 18, 0, 0, 0, time.UTC); !e.Bucket().Equal(want) {
		t.Errorf("Bucket = %v, want %v", e.Bucket(), want)
	}
	_, err = NewView("u1", "no-es-id", "request", at)
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeValidation || len(de.Details) != 2 {
		t.Fatalf("inválido: %v", err)
	}
	if de.Details[0].Field != "itemId" || de.Details[1].Field != "type" {
		t.Errorf("detalles = %+v", de.Details)
	}
}

func TestValidObjectID(t *testing.T) {
	for s, want := range map[string]bool{
		oid: true, "": false, strings.ToUpper(oid): false, oid + "0": false, "6730f1c2a9b3e4d5f6a7b8cz": false,
	} {
		if ValidObjectID(s) != want {
			t.Errorf("ValidObjectID(%q) != %v", s, want)
		}
	}
}

func TestParseLimit(t *testing.T) {
	for raw, want := range map[string]int{"": DefaultLimit, " ": DefaultLimit, "1": 1, "20": 20, " 7 ": 7} {
		if got, err := ParseLimit(raw); err != nil || got != want {
			t.Errorf("ParseLimit(%q) = %d, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"0", "21", "-1", "abc", "1e2", "999999999999999999999"} {
		_, err := ParseLimit(raw)
		var de *shared.Error
		if !errors.As(err, &de) || de.Code != shared.CodeValidation || de.Details[0].Field != "limit" {
			t.Errorf("ParseLimit(%q): %v", raw, err)
		}
	}
}

func TestCleanMunicipality(t *testing.T) {
	for raw, want := range map[string]string{"": "", "   ": "", "  San   Pedro  ": "San Pedro", "Tula": "Tula"} {
		if got, err := CleanMunicipality(raw); err != nil || got != want {
			t.Errorf("CleanMunicipality(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"A", strings.Repeat("ñ", 61)} {
		var de *shared.Error
		if _, err := CleanMunicipality(raw); !errors.As(err, &de) || de.Details[0].Field != "municipality" {
			t.Errorf("CleanMunicipality(%q): %v", raw, err)
		}
	}
}

func TestReasonsAndErrors(t *testing.T) {
	r := SimilarTo(oid, "Sierra")
	if r.Code != ReasonSimilarTo || r.ItemID != oid || r.ItemTitle != "Sierra" {
		t.Errorf("SimilarTo = %+v", r)
	}
	if ErrItemNotFound.Code != shared.CodeNotFound || ErrDisabled.Code != shared.CodeUnavailable {
		t.Error("códigos de error")
	}
	for _, c := range []ReasonCode{ReasonSimilarTo, ReasonPopularInCategory, ReasonPopularNearby, ReasonPopular} {
		if c == "" {
			t.Error("motivo vacío")
		}
	}
}
