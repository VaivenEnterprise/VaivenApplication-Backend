package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

var t0 = time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC) // 12:00 en México

func item() ItemForBooking {
	return ItemForBooking{ID: "i1", OwnerID: "own", Status: "active", Title: "Taladro", Modality: ModalityRent,
		PricePerDayCents: 3000, FirstPhoto: &images.Image{PublicID: "p"}}
}

func pending(t *testing.T, start, end string) Booking {
	t.Helper()
	q, err := NewQuote(rentInput(start, end, PaymentSplit5050))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewRequest("b1", "req", item(), q, t0, mustLoc(t))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNewRequest(t *testing.T) {
	b := pending(t, "2026-11-20", "2026-11-22")
	if b.Status != StatusPending || b.Version != 1 || b.Price.TotalCents != 9000 || b.Price.Currency != "MXN" ||
		b.Item.Title != "Taladro" || len(b.Payments) != 2 || b.Payments[0].Status != PaymentPending ||
		len(b.History) != 1 || b.History[0].By != RoleRequester {
		t.Errorf("b = %+v", b)
	}
	// Vence a las 48 h (el inicio está lejos).
	if !b.ExpiresAt.Equal(t0.Add(48 * time.Hour)) {
		t.Errorf("ExpiresAt = %v", b.ExpiresAt)
	}
	// Inicio mañana: vence al empezar ese día en México (antes de 48 h).
	b = pending(t, "2026-11-09", "2026-11-09")
	if !b.ExpiresAt.Equal(time.Date(2026, 11, 9, 6, 0, 0, 0, time.UTC)) {
		t.Errorf("ExpiresAt inicio = %v", b.ExpiresAt)
	}
	q, _ := NewQuote(rentInput("2026-11-10", "2026-11-11", PaymentSplit5050))
	own := item()
	own.OwnerID = "req"
	if _, err := NewRequest("x", "req", own, q, t0, mustLoc(t)); !errors.Is(err, ErrOwnItem) { // CA-04
		t.Errorf("propio = %v", err)
	}
	paused := item()
	paused.Status = "paused"
	if _, err := NewRequest("x", "req", paused, q, t0, mustLoc(t)); !errors.Is(err, ErrItemUnavailable) {
		t.Errorf("pausado = %v", err)
	}
}

func TestTransitionsTable(t *testing.T) {
	all := []Status{StatusPending, StatusAccepted, StatusRejected, StatusExpired, StatusCancelled, StatusInProgress, StatusCompleted}
	roles := []Role{RoleOwner, RoleRequester, RoleSystem, RoleAdmin}
	valid := map[string]bool{
		"pending>accepted>owner": true, "pending>rejected>owner": true, "pending>cancelled>requester": true,
		"pending>expired>system": true, "accepted>cancelled>owner": true, "accepted>cancelled>requester": true,
		"accepted>in_progress>system": true, "in_progress>completed>system": true,
	}
	for _, from := range all {
		for _, to := range all {
			for _, by := range roles {
				key := fmt.Sprintf("%s>%s>%s", from, to, by)
				if got := CanTransition(from, to, by); got != valid[key] {
					t.Errorf("%s = %v", key, got)
				}
			}
		}
	}
}

func TestTransition(t *testing.T) {
	b := pending(t, "2026-11-20", "2026-11-22")
	if err := b.Transition(StatusAccepted, RoleRequester, t0); !errors.Is(err, ErrForbidden) {
		t.Errorf("solicitante acepta = %v", err)
	}
	if err := b.Transition(StatusCompleted, RoleOwner, t0); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("pending → completed = %v", err)
	}
	if err := b.Transition(StatusAccepted, RoleOwner, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if b.Status != StatusAccepted || b.ExpiresAt != nil || len(b.History) != 2 || !b.UpdatedAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("aceptada = %+v", b)
	}
	if err := b.Transition(StatusRejected, RoleOwner, t0); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("accepted → rejected = %v", err)
	}
	if err := b.Transition(StatusCancelled, RoleOwner, t0); err != nil {
		t.Errorf("propietario cancela aceptada = %v", err)
	}
	for _, to := range []Status{StatusAccepted, StatusPending, StatusCancelled} {
		if err := b.Transition(to, RoleOwner, t0); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("cancelled → %s = %v", to, err)
		}
	}
}

func TestExpiry(t *testing.T) {
	b := pending(t, "2026-11-20", "2026-11-22")
	late := t0.Add(48 * time.Hour)
	if b.EffectiveStatus(late.Add(-time.Second)) != StatusPending || b.EffectiveStatus(late) != StatusExpired {
		t.Error("EffectiveStatus")
	}
	// Vencida: ya no se acepta ni se cancela, pero el sistema la marca expired (CA-06).
	if err := b.Transition(StatusAccepted, RoleOwner, late); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("aceptar vencida = %v", err)
	}
	if err := b.Transition(StatusExpired, RoleSystem, late); err != nil || b.Status != StatusExpired {
		t.Errorf("vencer = %v %s", err, b.Status)
	}
	if b.EffectiveStatus(late) != StatusExpired {
		t.Error("expired se mantiene")
	}
}

func TestHistoryCap(t *testing.T) {
	b := pending(t, "2026-11-20", "2026-11-22")
	for i := range 12 {
		b.History = append(b.History, HistoryEntry{Status: StatusPending, At: t0.Add(time.Duration(i) * time.Minute)})
	}
	_ = b.Transition(StatusAccepted, RoleOwner, t0)
	if len(b.History) != MaxHistory || b.History[MaxHistory-1].Status != StatusAccepted {
		t.Errorf("historial = %d", len(b.History))
	}
}

func TestRoleAndHelpers(t *testing.T) {
	b := pending(t, "2026-11-20", "2026-11-22")
	if r, ok := b.RoleOf("own"); !ok || r != RoleOwner {
		t.Error("dueño")
	}
	if r, ok := b.RoleOf("req"); !ok || r != RoleRequester {
		t.Error("solicitante")
	}
	if _, ok := b.RoleOf(""); ok {
		t.Error("vacío")
	}
	if _, ok := b.RoleOf("otro"); ok {
		t.Error("tercero")
	}
	if !b.Overlaps(MustDate("2026-11-22"), MustDate("2026-11-25")) || b.Overlaps(MustDate("2026-11-23"), MustDate("2026-11-25")) {
		t.Error("Overlaps")
	}
	if !StatusAccepted.Blocks() || !StatusInProgress.Blocks() || StatusPending.Blocks() {
		t.Error("Blocks")
	}
	if !StatusCompleted.Valid() || Status("x").Valid() {
		t.Error("Valid")
	}
	if s, err := NormalizeText("reason", nil, 3, 200); s != nil || err != nil {
		t.Error("nil")
	}
	blank := "   "
	if s, err := NormalizeText("reason", &blank, 3, 200); s != nil || err != nil {
		t.Error("vacío")
	}
	ok := "  hola  "
	if s, err := NormalizeText("reason", &ok, 3, 200); err != nil || *s != "hola" {
		t.Error("recorte")
	}
	for _, c := range []struct {
		v        string
		min      int
		wantText string
	}{{"ab", 3, "entre 3"}, {strings.Repeat("a", 201), 1, "como máximo"}} {
		v := c.v
		_, err := NormalizeText("reason", &v, c.min, 200)
		if f := detailFields(t, err); f[0] != "reason" || !strings.Contains(err.Error(), "Revisa") {
			t.Errorf("NormalizeText(%q) = %v", c.v, err)
		}
	}
	if ErrQuoteChanged.Details[0].Issue != "quote_changed" || ErrReasonRequired.Details[0].Field != "reason" {
		t.Error("errores con detalle")
	}
}
