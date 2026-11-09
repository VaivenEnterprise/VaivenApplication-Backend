package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

func ptr(s string) *string { return &s }

func TestAccept(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a := create(t, e, req, "2026-11-10", "2026-11-12")
	b := create(t, e, Viewer{ID: "r2"}, "2026-11-12", "2026-11-13")
	e.not.sent = nil

	if code(t, mustErr(e.svc.Accept(ctx, req, a.Booking.ID, nil))) != shared.CodeForbidden {
		t.Error("el solicitante no acepta")
	}
	if !errors.Is(mustErr(e.svc.Accept(ctx, other, a.Booking.ID, nil)), domain.ErrNotFound) {
		t.Error("un tercero recibe 404")
	}
	if code(t, mustErr(e.svc.Accept(ctx, owner, a.Booking.ID, ptr("ab")))) != shared.CodeValidation {
		t.Error("punto de encuentro corto")
	}
	v, err := e.svc.Accept(ctx, owner, a.Booking.ID, ptr("  Metro Coyoacán  "))
	if err != nil || v.Booking.Status != domain.StatusAccepted || *v.Booking.MeetingPoint != "Metro Coyoacán" || v.ViewerRole != domain.RoleOwner {
		t.Fatalf("Accept = %+v %v", v.Booking, err)
	}
	if got := e.repo.bookings[b.Booking.ID]; got.Status != domain.StatusRejected || *got.RejectReason != "no_disponible" {
		t.Errorf("cruzada = %+v", got)
	}
	want := []sent{{"req", "booking.accepted"}, {"r2", "booking.rejected"}}
	if len(e.not.sent) != 2 || e.not.sent[0] != want[0] || e.not.sent[1] != want[1] {
		t.Errorf("avisos = %+v", e.not.sent)
	}
	if !errors.Is(mustErr(e.svc.Accept(ctx, owner, a.Booking.ID, nil)), domain.ErrInvalidTransition) {
		t.Error("aceptar dos veces")
	}
	e.repo.fail["Accept"] = domain.ErrPeriodTaken
	c := create(t, e, Viewer{ID: "r3"}, "2026-11-20", "2026-11-20")
	if !errors.Is(mustErr(e.svc.Accept(ctx, owner, c.Booking.ID, nil)), domain.ErrPeriodTaken) {
		t.Error("CA-05 desde el repositorio")
	}
	if !errors.Is(mustErr(e.svc.Accept(ctx, owner, "nada", nil)), domain.ErrNotFound) {
		t.Error("inexistente")
	}
}

func TestRejectAndCancel(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a := create(t, e, req, "2026-11-10", "2026-11-12")
	if code(t, mustErr(e.svc.Reject(ctx, req, a.Booking.ID, nil))) != shared.CodeForbidden {
		t.Error("el solicitante no rechaza")
	}
	if code(t, mustErr(e.svc.Reject(ctx, owner, a.Booking.ID, ptr(strings.Repeat("a", 201))))) != shared.CodeValidation {
		t.Error("motivo largo")
	}
	v, err := e.svc.Reject(ctx, owner, a.Booking.ID, ptr("Ya está prestado"))
	if err != nil || v.Booking.Status != domain.StatusRejected || *v.Booking.RejectReason != "Ya está prestado" {
		t.Fatalf("Reject = %+v %v", v.Booking, err)
	}

	// Pending: solo el solicitante cancela, sin motivo obligatorio.
	b := create(t, e, req, "2026-11-20", "2026-11-21")
	if code(t, mustErr(e.svc.Cancel(ctx, owner, b.Booking.ID, ptr("No")))) != shared.CodeValidation {
		t.Error("motivo de 2 letras")
	}
	if code(t, mustErr(e.svc.Cancel(ctx, owner, b.Booking.ID, nil))) != shared.CodeForbidden {
		t.Error("el propietario no cancela una pendiente")
	}
	v, err = e.svc.Cancel(ctx, req, b.Booking.ID, nil)
	if err != nil || v.Booking.Status != domain.StatusCancelled || *v.Booking.CancelledBy != domain.RoleRequester || v.Booking.CancelReason != nil {
		t.Fatalf("cancelar pendiente = %+v %v", v.Booking, err)
	}

	// Accepted: cualquiera, con motivo obligatorio; libera el periodo (CA-08).
	c := create(t, e, req, "2026-11-25", "2026-11-26")
	if _, err := e.svc.Accept(ctx, owner, c.Booking.ID, nil); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(mustErr(e.svc.Cancel(ctx, owner, c.Booking.ID, nil)), domain.ErrReasonRequired) {
		t.Error("motivo obligatorio")
	}
	e.not.sent = nil
	v, err = e.svc.Cancel(ctx, owner, c.Booking.ID, ptr("Se descompuso"))
	if err != nil || *v.Booking.CancelledBy != domain.RoleOwner || *v.Booking.CancelReason != "Se descompuso" {
		t.Fatalf("cancelar aceptada = %+v %v", v.Booking, err)
	}
	if len(e.not.sent) != 1 || e.not.sent[0] != (sent{"req", "booking.cancelled"}) {
		t.Errorf("aviso = %+v", e.not.sent)
	}
	if taken, _ := e.repo.HasBlockingOverlap(ctx, "item1", domain.MustDate("2026-11-25"), domain.MustDate("2026-11-26")); taken {
		t.Error("CA-08: el periodo debe quedar libre")
	}
	for name, f := range map[string]func() error{
		"reject": func() error { return mustErr(e.svc.Reject(ctx, other, c.Booking.ID, nil)) },
		"cancel": func() error { return mustErr(e.svc.Cancel(ctx, other, c.Booking.ID, nil)) },
		"update1": func() error {
			e.repo.fail["Update"] = errors.New("x")
			return mustErr(e.svc.Reject(ctx, owner, create(t, e, req, "2026-11-28", "2026-11-28").Booking.ID, nil))
		},
	} {
		if f() == nil {
			t.Errorf("%s: debe fallar", name)
		}
	}
	delete(e.repo.fail, "Update")
	d := create(t, e, req, "2026-11-29", "2026-11-29")
	e.repo.fail["Update"] = errors.New("x")
	if mustErr(e.svc.Cancel(ctx, req, d.Booking.ID, nil)) == nil {
		t.Error("cancel con error al guardar")
	}
}

func TestMeetingPoint(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a := create(t, e, req, "2026-11-10", "2026-11-12")
	if !errors.Is(mustErr(e.svc.SetMeetingPoint(ctx, owner, a.Booking.ID, "Metro")), domain.ErrInvalidTransition) {
		t.Error("pendiente no admite punto de encuentro")
	}
	_, _ = e.svc.Accept(ctx, owner, a.Booking.ID, nil)
	if !errors.Is(mustErr(e.svc.SetMeetingPoint(ctx, req, a.Booking.ID, "Metro")), domain.ErrForbidden) {
		t.Error("solo el propietario")
	}
	if !errors.Is(mustErr(e.svc.SetMeetingPoint(ctx, owner, a.Booking.ID, "   ")), domain.ErrMeetingPointRequired) {
		t.Error("vacío")
	}
	if code(t, mustErr(e.svc.SetMeetingPoint(ctx, owner, a.Booking.ID, "ab"))) != shared.CodeValidation {
		t.Error("corto")
	}
	v, err := e.svc.SetMeetingPoint(ctx, owner, a.Booking.ID, "Parque Centenario")
	if err != nil || *v.Booking.MeetingPoint != "Parque Centenario" {
		t.Fatalf("SetMeetingPoint = %v", err)
	}
	if !errors.Is(mustErr(e.svc.SetMeetingPoint(ctx, other, a.Booking.ID, "Parque")), domain.ErrNotFound) {
		t.Error("tercero")
	}
	e.repo.fail["Update"] = errors.New("x")
	if mustErr(e.svc.SetMeetingPoint(ctx, owner, a.Booking.ID, "Otro lugar")) == nil {
		t.Error("error al guardar")
	}
}

func TestExpiredCannotBeAccepted(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a := create(t, e, req, "2026-11-20", "2026-11-21")
	*e.now = t0.Add(48 * time.Hour)
	if !errors.Is(mustErr(e.svc.Accept(ctx, owner, a.Booking.ID, nil)), domain.ErrInvalidTransition) {
		t.Error("aceptar vencida")
	}
	if !errors.Is(mustErr(e.svc.Cancel(ctx, req, a.Booking.ID, nil)), domain.ErrInvalidTransition) {
		t.Error("cancelar vencida")
	}
}
