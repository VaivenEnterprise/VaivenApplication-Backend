package usecase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/domain"
)

var ctx = context.Background()

func pending(stage bookings.Stage) bookings.BookingPayment {
	return bookings.BookingPayment{Stage: stage, AmountCents: 4500, Status: bookings.PaymentPending}
}

func ptr(s string) *string { return &s }

func TestNewServiceRequiresDeps(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	e := newEnv(t)
	if _, err := NewService(Deps{Repo: e.repo, Bookings: e.repo, Notifier: e.not, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Errorf("con lo mínimo debe crear: %v", err)
	}
}

func TestReportThenConfirm(t *testing.T) { // CA-06
	e := newEnv(t, booking("b1", bookings.StatusAccepted, pending(bookings.StageDelivery), pending(bookings.StageReturn)))
	if err := e.svc.Confirm(ctx, Viewer{"owner"}, "b1", bookings.StageDelivery); err != domain.ErrNotReported {
		t.Fatalf("sin reporte previo: %v", err)
	}
	if err := e.svc.Report(ctx, Viewer{"req"}, "b1", bookings.StageDelivery, ptr("SPEI")); err != nil {
		t.Fatal(err)
	}
	p, _ := e.repo.bs["b1"].PaymentFor(bookings.StageDelivery)
	if p.Status != bookings.PaymentReported || p.ReportedAt == nil || p.ReportedNote == nil || *p.ReportedNote != "SPEI" {
		t.Fatalf("pago = %+v", p)
	}
	if e.not.count("payment_reported:owner") != 1 {
		t.Errorf("avisos = %v", e.not.sent)
	}
	if err := e.svc.Report(ctx, Viewer{"req"}, "b1", bookings.StageDelivery, nil); err != domain.ErrNotPending {
		t.Errorf("reportar dos veces: %v", err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"owner"}, "b1", bookings.StageDelivery); err != nil {
		t.Fatal(err)
	}
	p, _ = e.repo.bs["b1"].PaymentFor(bookings.StageDelivery)
	if p.Status != bookings.PaymentConfirmed || p.ConfirmedAt == nil || e.not.count("payment_confirmed:req") != 1 {
		t.Errorf("pago = %+v avisos = %v", p, e.not.sent)
	}
	if err := e.svc.Confirm(ctx, Viewer{"owner"}, "b1", bookings.StageDelivery); err != domain.ErrNotReported {
		t.Errorf("confirmar dos veces: %v", err)
	}
	// La otra etapa no se tocó: el pago de la devolución sigue pendiente.
	if q, _ := e.repo.bs["b1"].PaymentFor(bookings.StageReturn); q.Status != bookings.PaymentPending {
		t.Errorf("devolución = %+v", q)
	}
}

func TestPaymentErrors(t *testing.T) {
	e := newEnv(t,
		booking("b1", bookings.StatusAccepted, pending(bookings.StageDelivery)),
		booking("loan", bookings.StatusAccepted),
		booking("pend", bookings.StatusPending, pending(bookings.StageDelivery)),
	)
	cases := []struct {
		name string
		do   func() error
		want error
	}{
		{"tercero reporta → 404", func() error { return e.svc.Report(ctx, Viewer{"otro"}, "b1", bookings.StageDelivery, nil) }, bookings.ErrNotFound},
		{"tercero confirma → 404", func() error { return e.svc.Confirm(ctx, Viewer{"otro"}, "b1", bookings.StageDelivery) }, bookings.ErrNotFound},
		{"propietario no reporta", func() error { return e.svc.Report(ctx, Viewer{"owner"}, "b1", bookings.StageDelivery, nil) }, domain.ErrForbidden},
		{"solicitante no confirma", func() error { return e.svc.Confirm(ctx, Viewer{"req"}, "b1", bookings.StageDelivery) }, domain.ErrForbidden},
		{"préstamo sin pagos (RF-08)", func() error { return e.svc.Report(ctx, Viewer{"req"}, "loan", bookings.StageDelivery, nil) }, domain.ErrNoPayment},
		{"reserva pendiente", func() error { return e.svc.Report(ctx, Viewer{"req"}, "pend", bookings.StageDelivery, nil) }, domain.ErrWrongBookingStatus},
		{"etapa inválida", func() error { return e.svc.Report(ctx, Viewer{"req"}, "b1", "otra", nil) }, bookings.ErrNotFound},
		{"etapa inválida al confirmar", func() error { return e.svc.Confirm(ctx, Viewer{"owner"}, "b1", "otra") }, bookings.ErrNotFound},
		{"reserva inexistente", func() error { return e.svc.Report(ctx, Viewer{"req"}, "zzz", bookings.StageDelivery, nil) }, bookings.ErrNotFound},
	}
	for _, c := range cases {
		if err := c.do(); err != c.want {
			t.Errorf("%s: err = %v, quiero %v", c.name, err, c.want)
		}
	}
	if err := e.svc.Report(ctx, Viewer{"req"}, "b1", bookings.StageDelivery, ptr(strings.Repeat("x", 201))); err == nil {
		t.Error("nota demasiado larga debe fallar")
	}
	if len(e.not.sent) != 0 {
		t.Errorf("ninguna operación fallida debe avisar: %v", e.not.sent)
	}
}

func TestReportSurvivesNotifierFailure(t *testing.T) {
	e := newEnv(t, booking("b1", bookings.StatusAccepted, pending(bookings.StageDelivery)))
	e.not.err = errors.New("push caído")
	if err := e.svc.Report(ctx, Viewer{"req"}, "b1", bookings.StageDelivery, nil); err != nil {
		t.Errorf("un aviso fallido no debe fallar el reporte: %v", err)
	}
}

// El QR y el pago son independientes (RF-07): reportar y confirmar el pago no
// toca el estado de la reserva.
func TestPaymentDoesNotChangeBookingStatus(t *testing.T) {
	e := newEnv(t, booking("b1", bookings.StatusAccepted, pending(bookings.StageDelivery)))
	_ = e.svc.Report(ctx, Viewer{"req"}, "b1", bookings.StageDelivery, nil)
	_ = e.svc.Confirm(ctx, Viewer{"owner"}, "b1", bookings.StageDelivery)
	if e.repo.bs["b1"].Status != bookings.StatusAccepted {
		t.Errorf("status = %s", e.repo.bs["b1"].Status)
	}
}

// racyRepo cambia el pago justo antes de guardar, como lo haría otra petición.
type racyRepo struct {
	*memRepo
	before func()
}

func (r *racyRepo) ChangePayment(c context.Context, id string, ch domain.Change) error {
	r.before()
	return r.memRepo.ChangePayment(c, id, ch)
}

func TestReportLosesRace(t *testing.T) {
	e := newEnv(t, booking("b1", bookings.StatusAccepted, pending(bookings.StageDelivery)))
	racy := &racyRepo{memRepo: e.repo, before: func() {
		e.repo.mu.Lock()
		e.repo.bs["b1"].Payments[0].Status = bookings.PaymentReported
		e.repo.mu.Unlock()
	}}
	svc, _ := NewService(Deps{Repo: racy, Bookings: racy, Notifier: e.not, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return t0 }})
	if err := svc.Report(ctx, Viewer{"req"}, "b1", bookings.StageDelivery, nil); err != domain.ErrNotPending {
		t.Errorf("err = %v, quiero ErrNotPending", err)
	}
	if len(e.not.sent) != 0 {
		t.Error("el que pierde la carrera no avisa")
	}
	// Si la regla sigue pasando tras releer, se informa un conflicto genérico.
	racy.before = func() {
		e.repo.mu.Lock()
		e.repo.bs["b1"].Payments[0].Status = bookings.PaymentPending
		e.repo.bs["b1"].Status = bookings.StatusAccepted
		e.repo.mu.Unlock()
	}
	racy2 := &alwaysNoMatch{memRepo: e.repo}
	svc2, _ := NewService(Deps{Repo: racy2, Bookings: racy2, Notifier: e.not, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return t0 }})
	e.repo.bs["b1"].Payments[0].Status = bookings.PaymentPending
	if err := svc2.Report(ctx, Viewer{"req"}, "b1", bookings.StageDelivery, nil); err != domain.ErrWrongBookingStatus {
		t.Errorf("err = %v, quiero ErrWrongBookingStatus", err)
	}
}

type alwaysNoMatch struct{ *memRepo }

func (alwaysNoMatch) ChangePayment(context.Context, string, domain.Change) error { return ErrNoMatch }

func TestConcurrentReportOnlyOneWins(t *testing.T) {
	e := newEnv(t, booking("b1", bookings.StatusAccepted, pending(bookings.StageDelivery)))
	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = e.svc.Report(ctx, Viewer{"req"}, "b1", bookings.StageDelivery, nil)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 || e.not.count("payment_reported") != 1 {
		t.Errorf("exitosas = %d, avisos = %d; quiero 1 y 1 (%v)", ok, e.not.count("payment_reported"), errs)
	}
}
