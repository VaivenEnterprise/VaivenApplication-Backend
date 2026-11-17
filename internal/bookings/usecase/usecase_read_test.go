package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

func TestGet(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a := create(t, e, req, "2026-11-10", "2026-11-12")
	_, _ = e.svc.Accept(ctx, owner, a.Booking.ID, ptr("Metro"))

	for _, c := range []struct {
		v        Viewer
		role     domain.Role
		showMeet bool
	}{{owner, domain.RoleOwner, true}, {req, domain.RoleRequester, true}, {Viewer{ID: "adm", Admin: true}, domain.RoleAdmin, false}} {
		got, err := e.svc.Get(ctx, c.v, a.Booking.ID)
		if err != nil || got.ViewerRole != c.role || got.ShowMeetingPoint != c.showMeet {
			t.Errorf("%s = %+v %v", c.v.ID, got.ViewerRole, err)
		}
	}
	if !errors.Is(mustErr(e.svc.Get(ctx, other, a.Booking.ID)), domain.ErrNotFound) {
		t.Error("tercero")
	}
	// Una pendiente vencida se muestra expired (P-04).
	b := create(t, e, req, "2026-11-20", "2026-11-21")
	*e.now = t0.Add(49 * time.Hour)
	got, _ := e.svc.Get(ctx, req, b.Booking.ID)
	if got.Booking.Status != domain.StatusExpired || got.Booking.ExpiresAt != nil {
		t.Errorf("vencida = %s", got.Booking.Status)
	}
	// Dueño dado de baja: «Cuenta eliminada».
	e.repo.items["ghost"] = domain.ItemForBooking{ID: "ghost", OwnerID: "ghost", Status: "active", Modality: domain.ModalityLoan}
	*e.now = t0
	g, err := e.svc.Create(ctx, req, CreateInput{QuoteInput: QuoteInput{ItemID: "ghost", Start: "2026-11-10", End: "2026-11-10", PaymentMode: domain.PaymentNone}})
	if err != nil || g.Owner.DisplayName != DeletedName {
		t.Errorf("dueño eliminado = %+v %v", g.Owner, err)
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	for i := range 5 {
		*e.now = t0.Add(time.Duration(i) * time.Minute)
		v := create(t, e, req, "2026-11-20", "2026-11-20")
		e.repo.bookings[v.Booking.ID] = cancelled(e.repo.bookings[v.Booking.ID])
	}
	create(t, e, Viewer{ID: "r2"}, "2026-11-21", "2026-11-21")

	seen := 0
	q := ListQuery{Role: domain.RoleRequester, Limit: 2}
	for range 5 {
		p, err := e.svc.List(ctx, req, q)
		if err != nil {
			t.Fatal(err)
		}
		seen += len(p.Items)
		if p.Next == nil {
			break
		}
		q.After = p.Next
	}
	if seen != 5 {
		t.Errorf("solicitante vio %d", seen)
	}
	pend := domain.StatusPending
	if p, _ := e.svc.List(ctx, owner, ListQuery{Role: domain.RoleOwner, Status: &pend}); len(p.Items) != 1 || p.Items[0].ViewerRole != domain.RoleOwner {
		t.Errorf("pendientes del propietario = %+v", p.Items)
	}
	if p, _ := e.svc.List(ctx, other, ListQuery{Role: domain.RoleOwner, Limit: 500}); len(p.Items) != 0 || p.Items == nil {
		t.Error("vacío")
	}
	bad := domain.Status("x")
	for _, q := range []ListQuery{{Role: "x"}, {Role: domain.RoleOwner, Status: &bad}} {
		if code(t, mustErr(e.svc.List(ctx, owner, q))) != shared.CodeValidation {
			t.Errorf("%+v debe ser 400", q)
		}
	}
	e.repo.fail["ListByParty"] = errors.New("x")
	if mustErr(e.svc.List(ctx, req, ListQuery{Role: domain.RoleRequester})) == nil {
		t.Error("error del repositorio")
	}
	delete(e.repo.fail, "ListByParty")
	e2 := newEnv(t, func(d *Deps) { d.Users = fakeUsers{err: errors.New("x")} })
	e2.repo.bookings = e.repo.bookings
	if mustErr(e2.svc.List(ctx, req, ListQuery{Role: domain.RoleRequester})) == nil {
		t.Error("error de perfiles")
	}
	e3 := newEnv(t, func(d *Deps) { d.Images = fakeImages{err: errors.New("x")} })
	e3.repo.bookings = e.repo.bookings
	if mustErr(e3.svc.List(ctx, req, ListQuery{Role: domain.RoleRequester})) == nil {
		t.Error("error de miniatura")
	}
}

func cancelled(b domain.Booking) domain.Booking {
	b.Status = domain.StatusCancelled
	return b
}

func TestExpireDueCA06(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a := create(t, e, req, "2026-11-20", "2026-11-21")
	b := create(t, e, req, "2026-11-25", "2026-11-25")
	_, _ = e.svc.Cancel(ctx, req, b.Booking.ID, nil)
	e.not.sent = nil

	if n, err := e.svc.ExpireDue(ctx); err != nil || n != 0 {
		t.Errorf("antes de 48 h = %d %v", n, err)
	}
	*e.now = t0.Add(48 * time.Hour)
	n, err := e.svc.ExpireDue(ctx)
	if err != nil || n != 1 || e.repo.bookings[a.Booking.ID].Status != domain.StatusExpired {
		t.Fatalf("vencer = %d %v", n, err)
	}
	if len(e.not.sent) != 1 || e.not.sent[0] != (sent{"req", "booking.expired"}) {
		t.Errorf("aviso = %+v", e.not.sent)
	}
	if n, _ := e.svc.ExpireDue(ctx); n != 0 {
		t.Error("idempotente")
	}

	c := create(t, e, req, "2026-11-28", "2026-11-28")
	*e.now = t0.Add(200 * time.Hour)
	e.repo.fail["Update"] = domain.ErrVersionConflict // otra réplica ya la venció
	if n, err := e.svc.ExpireDue(ctx); err != nil || n != 0 {
		t.Errorf("conflicto = %d %v", n, err)
	}
	e.repo.fail["Update"] = errors.New("mongo")
	if _, err := e.svc.ExpireDue(ctx); err == nil {
		t.Error("error al guardar")
	}
	delete(e.repo.fail, "Update")
	e.repo.fail["DuePending"] = errors.New("mongo")
	if _, err := e.svc.ExpireDue(ctx); err == nil {
		t.Error("error al buscar")
	}
	_ = c
}

// Spec 009: las etapas de entrega y devolución se muestran con URL firmadas
// solo a las partes (RF-05); admin no las recibe.
func TestGetShowsHandoverStagesOnlyToParties(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a := create(t, e, req, "2026-11-10", "2026-11-12")
	b := e.repo.bookings[a.Booking.ID]
	exp := t0.Add(10 * time.Minute)
	b.Delivery = &domain.Handover{
		ConditionNotes: "Sin daños", EvidenceVersion: 1,
		Photos: []images.Image{{PublicID: "ev1", Delivery: images.DeliveryAuthenticated}},
		QR:     &domain.QR{TokenHash: "h", ExpiresAt: exp, EvidenceVersion: 1},
	}
	e.repo.bookings[b.ID] = b

	for _, v := range []Viewer{owner, req} {
		got, err := e.svc.Get(ctx, v, b.ID)
		if err != nil || got.Delivery == nil || len(got.Delivery.URLs) != 1 || got.Delivery.QR != domain.QRActive ||
			got.Delivery.QRExpiresAt == nil || !got.Delivery.QRExpiresAt.Equal(exp) {
			t.Errorf("%s: %+v %v", v.ID, got.Delivery, err)
		}
		if got.Return != nil {
			t.Errorf("%s: sin devolución registrada debe ser nil", v.ID)
		}
	}
	adm, err := e.svc.Get(ctx, Viewer{ID: "adm", Admin: true}, b.ID)
	if err != nil || adm.Delivery != nil || adm.Return != nil {
		t.Errorf("admin no recibe la evidencia: %+v %v", adm.Delivery, err)
	}
	*e.now = t0.Add(time.Hour)
	if got, _ := e.svc.Get(ctx, req, b.ID); got.Delivery.QR != domain.QRExpired {
		t.Errorf("QR vencido = %s", got.Delivery.QR)
	}
}
