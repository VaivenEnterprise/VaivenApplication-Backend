package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// Spec 009: Booking incluye las etapas, el retraso, los ajustes y la nota del
// pago, sin exponer nunca el hash del token del QR (RNF-01).
func TestToBookingIncludesHandoverData(t *testing.T) {
	at := time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)
	note, aff, by := "Transferencia", "b-tardia", "owner"
	b := domain.Booking{
		ID: "b1", Start: domain.MustDate("2026-11-20"), End: domain.MustDate("2026-11-22"),
		Payments: []domain.BookingPayment{{Stage: domain.StageDelivery, AmountCents: 9000, Status: domain.PaymentReported, ReportedAt: &at, ReportedNote: &note}},
		Delivery: &domain.Handover{
			RecordedAt: &at, RecordedByID: &by, ConditionNotes: "Sin daños", EvidenceVersion: 2,
			Photos: []images.Image{{PublicID: "ev1", Width: 10, Height: 20, Format: "jpg", Bytes: 5, Delivery: images.DeliveryAuthenticated}},
			QR:     &domain.QR{TokenHash: "NO-DEBE-SALIR", ExpiresAt: at.Add(15 * time.Minute), EvidenceVersion: 2},
		},
		IsLate:      true,
		Adjustments: []domain.Adjustment{{Type: domain.AdjustmentLateReturn, LateDays: 2, AmountCents: 6000, Status: domain.AdjustmentRecorded}},
		AffectedBy:  &aff,
	}
	exp := at.Add(15 * time.Minute)
	v := usecase.BookingView{
		Booking: b,
		Delivery: &usecase.HandoverView{
			Stage: *b.Delivery, URLs: []images.URLs{{URL: "https://img/ev1", ThumbURL: "https://img/t/ev1"}},
			QR: domain.QRActive, QRExpiresAt: &exp,
		},
	}
	raw, err := json.Marshal(toBooking(v))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "NO-DEBE-SALIR") || strings.Contains(string(raw), "tokenHash") {
		t.Fatal("el hash del token no puede salir en la respuesta")
	}
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	del := got["delivery"].(map[string]any)
	photos := del["photos"].([]any)
	if del["evidenceVersion"] != float64(2) || del["conditionNotes"] != "Sin daños" || len(photos) != 1 ||
		photos[0].(map[string]any)["url"] != "https://img/ev1" ||
		del["qr"].(map[string]any)["state"] != "active" || del["qr"].(map[string]any)["expiresAt"] != "2026-11-20T18:15:00Z" {
		t.Errorf("delivery = %v", del)
	}
	if got["return"] != nil {
		t.Errorf("return = %v, quiero null", got["return"])
	}
	adj := got["adjustments"].([]any)
	if got["isLate"] != true || len(adj) != 1 || adj[0].(map[string]any)["amountCents"] != float64(6000) ||
		adj[0].(map[string]any)["status"] != "recorded" || got["affectedBy"] != "b-tardia" {
		t.Errorf("retraso = %v / %v / %v", got["isLate"], adj, got["affectedBy"])
	}
	pay := got["payments"].([]any)[0].(map[string]any)
	if pay["status"] != "reported" || pay["note"] != "Transferencia" {
		t.Errorf("payments[0] = %v", pay)
	}
}

func TestToBookingWithoutHandoverData(t *testing.T) {
	raw, _ := json.Marshal(toBooking(usecase.BookingView{Booking: domain.Booking{ID: "b1"}}))
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	if got["delivery"] != nil || got["return"] != nil || got["isLate"] != false || got["affectedBy"] != nil {
		t.Errorf("sin datos de 009: %s", raw)
	}
	if adj, ok := got["adjustments"].([]any); !ok || len(adj) != 0 {
		t.Errorf("adjustments debe ser [] y no null: %s", raw)
	}
}
