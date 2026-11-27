package domain

import (
	"strings"
	"testing"
	"time"
)

func pendingBooking(at time.Time) Booking {
	exp := at.Add(24 * time.Hour)
	return Booking{
		ID: "b1", ItemID: "i1", OwnerID: "owner", RequesterID: "req", Item: ItemSnapshot{Title: "Taladro"},
		Status: StatusPending, ExpiresAt: &exp, Version: 1,
		History: []HistoryEntry{{Status: StatusPending, At: at, By: RoleRequester}},
	}
}

// Spec 021, T015 (CA-10): la solicitante cancela, la dueña rechaza, con motivo
// «cuenta eliminada» y un aviso para la contraparte.
func TestCloseOnAccountDeletion(t *testing.T) {
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

	b := pendingBooking(at)
	n, err := CloseOnAccountDeletion(&b, "req", at)
	if err != nil || b.Status != StatusCancelled || b.CancelReason == nil || *b.CancelReason != AccountDeletedReason ||
		b.CancelledBy == nil || *b.CancelledBy != RoleRequester || b.RejectReason != nil || b.ExpiresAt != nil {
		t.Fatalf("solicitante: %+v, %v", b, err)
	}
	if n.UserID != "owner" || n.Type != "booking.cancelled" || n.Data["bookingId"] != "b1" || n.Data["itemId"] != "i1" {
		t.Errorf("aviso a la dueña = %+v", n)
	}
	if last := b.History[len(b.History)-1]; last.Status != StatusCancelled || last.By != RoleRequester || !last.At.Equal(at) {
		t.Errorf("historial = %+v", last)
	}

	b = pendingBooking(at)
	n, err = CloseOnAccountDeletion(&b, "owner", at)
	if err != nil || b.Status != StatusRejected || b.RejectReason == nil || *b.RejectReason != AccountDeletedReason || b.CancelReason != nil {
		t.Fatalf("dueña: %+v, %v", b, err)
	}
	if n.UserID != "req" || n.Type != "booking.rejected" || !strings.Contains(n.Body, "Taladro") {
		t.Errorf("aviso a la solicitante = %+v", n)
	}
	for _, text := range []string{n.Title, n.Body} {
		if strings.Contains(text, "owner") || strings.Contains(text, "req") && strings.Contains(text, "@") {
			t.Errorf("el aviso no debe llevar datos de la cuenta: %q", text)
		}
	}
}

func TestCloseOnAccountDeletionRejectsWhatCannotClose(t *testing.T) {
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	b := pendingBooking(at)
	if _, err := CloseOnAccountDeletion(&b, "tercero", at); err != ErrForbidden || b.Status != StatusPending {
		t.Errorf("ajena = %v, estado %s", err, b.Status)
	}
	if _, err := CloseOnAccountDeletion(&b, "", at); err != ErrForbidden {
		t.Errorf("sin id = %v", err)
	}
	// Vencida pero aún marcada pending: no se toca (la tarea periódica la cierra).
	if _, err := CloseOnAccountDeletion(&b, "req", at.Add(48*time.Hour)); err != ErrInvalidTransition || b.Status != StatusPending {
		t.Errorf("vencida = %v, estado %s", err, b.Status)
	}
	for _, st := range []Status{StatusAccepted, StatusInProgress, StatusCompleted, StatusRejected, StatusCancelled, StatusExpired} {
		b2 := pendingBooking(at)
		b2.Status = st
		if _, err := CloseOnAccountDeletion(&b2, "req", at); err == nil || b2.Status != st {
			t.Errorf("estado %s: err = %v, estado final %s", st, err, b2.Status)
		}
	}
}
