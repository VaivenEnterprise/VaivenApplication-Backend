package usecase

import (
	"errors"
	"sync"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
)

// confirmDelivery lleva la reserva de accepted a in_progress por el camino normal.
func confirmDelivery(t *testing.T, e *env) {
	t.Helper()
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "Todo bien", refs("p1")); err != nil {
		t.Fatal(err)
	}
	q, err := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, q.Token); err != nil {
		t.Fatal(err)
	}
}

func TestDeliveryThenReturnFlow(t *testing.T) { // CA-01 y CA-05
	e := newEnv(t, bookings.StatusAccepted)
	confirmDelivery(t, e)
	b := e.repo.b
	if b.Status != bookings.StatusInProgress || b.Delivery.ConfirmedAt == nil || *b.Delivery.ConfirmedByID != "req" {
		t.Fatalf("tras la entrega: %+v", b)
	}
	if b.LateAt == nil || !b.LateAt.Equal(time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("LateAt = %v", b.LateAt)
	}
	if got := e.notif.events["owner"]; len(got) != 1 || got[0].Type != "delivery_confirmed" || got[0].Data["bookingId"] != "b1" {
		t.Errorf("RF-12: el propietario debe recibir el aviso, got %+v", e.notif.events)
	}
	if _, ok := e.notif.events["req"]; ok {
		t.Error("quien confirma no recibe aviso")
	}

	*e.now = t0.Add(48 * time.Hour)
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"req"}, "b1", domain.KindReturn, "Devuelto limpio", refs("r1")); err != nil {
		t.Fatal(err)
	}
	q, err := e.svc.IssueQR(ctx, Viewer{"req"}, "b1", domain.KindReturn)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"owner"}, "b1", domain.KindReturn, q.Token); err != nil {
		t.Fatal(err)
	}
	if e.repo.b.Status != bookings.StatusCompleted || e.repo.b.Return.ConfirmedAt == nil {
		t.Errorf("tras la devolución: %+v", e.repo.b)
	}
	if got := e.notif.events["req"]; len(got) != 1 || got[0].Type != "return_confirmed" {
		t.Errorf("el solicitante debe recibir el aviso: %+v", e.notif.events["req"])
	}
	if len(e.repo.b.Adjustments) != 0 {
		t.Error("a tiempo no hay ajuste")
	}
}

func TestReturnRecordsLateAdjustment(t *testing.T) { // CA-07 (la cuenta está en payments)
	e := newEnv(t, bookings.StatusAccepted)
	confirmDelivery(t, e)
	adj := &bookings.Adjustment{Type: bookings.AdjustmentLateReturn, LateDays: 2, AmountCents: 6000, Status: bookings.AdjustmentRecorded}
	e.svc.d.Lateness = fakeLateness{adj: adj}
	*e.now = time.Date(2026, 11, 24, 3, 0, 0, 0, time.UTC) // tarde
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"req"}, "b1", domain.KindReturn, "Devuelto", refs("r1")); err != nil {
		t.Fatal(err)
	}
	q, _ := e.svc.IssueQR(ctx, Viewer{"req"}, "b1", domain.KindReturn)
	if err := e.svc.Confirm(ctx, Viewer{"owner"}, "b1", domain.KindReturn, q.Token); err != nil {
		t.Fatal(err)
	}
	if got := e.repo.b.Adjustments; len(got) != 1 || got[0].AmountCents != 6000 {
		t.Errorf("Adjustments = %+v", got)
	}
}

func TestConfirmQRExpiredThenRenew(t *testing.T) { // CA-02
	e := newEnv(t, bookings.StatusAccepted)
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "Todo bien", refs("p1")); err != nil {
		t.Fatal(err)
	}
	q, _ := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery)
	*e.now = t0.Add(16 * time.Minute)
	if err := e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, q.Token); err != domain.ErrQRExpired {
		t.Fatalf("err = %v, quiero ErrQRExpired", err)
	}
	q2, err := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery)
	if err != nil {
		t.Fatalf("renovar: %v", err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, q2.Token); err != nil {
		t.Errorf("el QR renovado debe servir: %v", err)
	}
}

func TestConfirmUsedQR(t *testing.T) { // CA-03
	e := newEnv(t, bookings.StatusAccepted)
	e.repo.b.Delivery = nil
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "Todo bien", refs("p1")); err != nil {
		t.Fatal(err)
	}
	q, _ := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery)
	if err := e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, q.Token); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, q.Token); err == nil {
		t.Error("reintentar con el QR usado debe fallar")
	}
}

func TestConfirmErrors(t *testing.T) {
	e := newEnv(t, bookings.StatusAccepted)
	if err := e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, " "); err != domain.ErrTokenRequired {
		t.Errorf("token vacío: %v", err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"otro"}, "b1", domain.KindDelivery, "x"); err != bookings.ErrNotFound {
		t.Errorf("tercero: %v", err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "x"); err != domain.ErrForbidden {
		t.Errorf("el propietario no confirma su entrega: %v", err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, "x"); err != domain.ErrQRInvalid {
		t.Errorf("sin QR: %v", err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.Kind("x"), "x"); err != bookings.ErrNotFound {
		t.Errorf("etapa inválida: %v", err)
	}
}

func TestConfirmSurvivesNotifierFailure(t *testing.T) {
	e := newEnv(t, bookings.StatusAccepted)
	e.notif.err = errors.New("push caído")
	confirmDelivery(t, e) // el fallo del aviso se registra y no deshace la confirmación
	if e.repo.b.Status != bookings.StatusInProgress {
		t.Error("la confirmación debe sobrevivir al fallo de la notificación")
	}
}

func TestConfirmConcurrentOnlyOneWins(t *testing.T) { // RNF-02
	e := newEnv(t, bookings.StatusAccepted)
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "Todo bien", refs("p1")); err != nil {
		t.Fatal(err)
	}
	q, _ := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery)
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, q.Token)
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Errorf("confirmaciones exitosas = %d, quiero exactamente 1 (%v)", ok, errs)
	}
	if got := len(e.notif.events["owner"]); got != 1 {
		t.Errorf("avisos = %d, quiero 1", got)
	}
}

func TestConfirmLosesRaceToDifferentQR(t *testing.T) {
	e := newEnv(t, bookings.StatusAccepted)
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "Todo bien", refs("p1")); err != nil {
		t.Fatal(err)
	}
	q, _ := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery)
	// Entre la lectura y el guardado la reserva sigue válida para el dominio pero
	// el repositorio no aplica (p. ej. el QR se marcó usado por otra réplica):
	// el caso de uso lo traduce a «QR ya usado».
	racy := &racyRepo{memRepo: e.repo, before: func() {
		e.repo.mu.Lock()
		h := *e.repo.b.Delivery
		qr := *h.QR
		qr.TokenHash = "otro"
		h.QR = &qr
		e.repo.b.Delivery = &h
		e.repo.mu.Unlock()
	}}
	svc, _ := NewService(Deps{Repo: racy, Bookings: racy, Images: e.imgs, Notifier: e.notif, Log: e.svc.d.Log, Now: func() time.Time { return t0 }})
	if err := svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, q.Token); err != domain.ErrQRInvalid {
		t.Errorf("err = %v, quiero ErrQRInvalid", err)
	}
}

func TestGetStage(t *testing.T) { // RF-05, CA-09
	e := newEnv(t, bookings.StatusAccepted)
	v, err := e.svc.GetStage(ctx, Viewer{"req"}, "b1", domain.KindDelivery)
	if err != nil || v.QR != domain.QRNone || len(v.URLs) != 0 || v.Stage.EvidenceVersion != 0 {
		t.Fatalf("etapa vacía: %+v %v", v, err)
	}
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "Todo bien", refs("p1", "p2")); err != nil {
		t.Fatal(err)
	}
	q, _ := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery)
	for _, who := range []string{"owner", "req"} {
		v, err := e.svc.GetStage(ctx, Viewer{who}, "b1", domain.KindDelivery)
		if err != nil || len(v.URLs) != 2 || v.QR != domain.QRActive || v.QRExpiresAt == nil || !v.QRExpiresAt.Equal(q.ExpiresAt) {
			t.Errorf("%s: %+v %v", who, v, err)
		}
	}
	*e.now = t0.Add(time.Hour)
	if v, _ := e.svc.GetStage(ctx, Viewer{"owner"}, "b1", domain.KindDelivery); v.QR != domain.QRExpired || v.QRExpiresAt == nil {
		t.Errorf("QR vencido: %+v", v)
	}
	if _, err := e.svc.GetStage(ctx, Viewer{"otro"}, "b1", domain.KindDelivery); err != bookings.ErrNotFound {
		t.Errorf("CA-09, ajeno: %v", err)
	}
	if _, err := e.svc.GetStage(ctx, Viewer{"owner"}, "b1", domain.Kind("x")); err != bookings.ErrNotFound {
		t.Errorf("etapa inválida: %v", err)
	}
	if _, err := e.svc.GetStage(ctx, Viewer{"owner"}, "zzz", domain.KindDelivery); err != bookings.ErrNotFound {
		t.Errorf("reserva inexistente: %v", err)
	}
}
