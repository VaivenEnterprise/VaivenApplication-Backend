package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

func TestNewServiceDeps(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	e := newEnv(t, func(d *Deps) { d.Now, d.Location = nil, nil })
	if e.svc.d.Location.String() != domain.TimeZone || e.svc.now().IsZero() {
		t.Error("valores por defecto")
	}
}

func TestQuoteCA01(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	q, err := e.svc.Quote(ctx, req, QuoteInput{ItemID: "item1", Start: "2026-11-10", End: "2026-11-12", PaymentMode: domain.PaymentSplit5050})
	if err != nil || q.Days != 3 || q.TotalCents != 9000 || q.Payments[0].AmountCents != 4500 {
		t.Fatalf("Quote = %+v %v", q, err)
	}
	if mustErr(e.svc.Quote(ctx, owner, QuoteInput{ItemID: "item1"})) != domain.ErrOwnItem { // CA-04
		t.Error("propio")
	}
	if mustErr(e.svc.Quote(ctx, req, QuoteInput{ItemID: "nada"})) != domain.ErrItemNotFound {
		t.Error("inexistente")
	}
	e.repo.items["paused"] = domain.ItemForBooking{ID: "paused", OwnerID: "own", Status: "paused"}
	if mustErr(e.svc.Quote(ctx, req, QuoteInput{ItemID: "paused"})) != domain.ErrItemUnavailable {
		t.Error("pausado")
	}
	if code(t, mustErr(e.svc.Quote(ctx, req, QuoteInput{ItemID: "item1", Start: "2026-11-01", End: "2026-11-02", PaymentMode: domain.PaymentSplit5050}))) != shared.CodeValidation {
		t.Error("fecha pasada")
	}
}

func TestCreate(t *testing.T) {
	e := newEnv(t)
	v := create(t, e, req, "2026-11-10", "2026-11-12")
	b := v.Booking
	if b.Status != domain.StatusPending || *b.ConversationID != "conv1" || b.Price.TotalCents != 9000 ||
		v.ViewerRole != domain.RoleRequester || *v.ThumbURL != "t/p1" || v.Owner.DisplayName != "Persona own" || !v.ShowMeetingPoint {
		t.Fatalf("vista = %+v", v)
	}
	if len(e.not.sent) != 1 || e.not.sent[0] != (sent{"own", "booking.requested"}) {
		t.Errorf("notificaciones = %+v", e.not.sent)
	}
	// Préstamo: total 0 y sin pagos.
	loan, err := e.svc.Create(context.Background(), req, CreateInput{
		QuoteInput: QuoteInput{ItemID: "loan1", Start: "2026-11-10", End: "2026-11-10", PaymentMode: domain.PaymentNone},
	})
	if err != nil || loan.Booking.Price.TotalCents != 0 || len(loan.Booking.Payments) != 0 || loan.ThumbURL != nil {
		t.Errorf("préstamo = %+v %v", loan.Booking, err)
	}
}

func TestCreateCA07Snapshot(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	v := create(t, e, req, "2026-11-10", "2026-11-12")
	it := e.repo.items["item1"]
	it.PricePerDayCents, it.Title = 9900, "Otro título"
	e.repo.items["item1"] = it
	got, err := e.svc.Get(ctx, req, v.Booking.ID)
	if err != nil || got.Booking.Price.PricePerDayCents != 3000 || got.Booking.Item.Title != "Taladro" {
		t.Errorf("instantánea = %+v %v", got.Booking, err)
	}
	// CA-03: con el total anterior, quote_changed.
	err = mustErr(e.svc.Create(ctx, Viewer{ID: "r2"}, CreateInput{
		QuoteInput:         QuoteInput{ItemID: "item1", Start: "2026-11-20", End: "2026-11-20", PaymentMode: domain.PaymentFullOnDelivery},
		AcceptedTotalCents: 3000,
	}))
	if !errors.Is(err, domain.ErrQuoteChanged) {
		t.Errorf("CA-03 = %v", err)
	}
}

func TestCreateConflicts(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	first := create(t, e, req, "2026-11-10", "2026-11-12")
	in := CreateInput{QuoteInput: QuoteInput{ItemID: "item1", Start: "2026-11-12", End: "2026-11-13", PaymentMode: domain.PaymentSplit5050}, AcceptedTotalCents: 6000}
	if err := mustErr(e.svc.Create(ctx, req, in)); !errors.Is(err, domain.ErrDuplicatePending) {
		t.Errorf("pendiente repetida = %v", err)
	}
	// Otra persona sí puede pedir las mismas fechas (provisional de la spec §6).
	if _, err := e.svc.Create(ctx, Viewer{ID: "r2"}, in); err != nil {
		t.Errorf("otra persona = %v", err)
	}
	if _, err := e.svc.Accept(ctx, owner, first.Booking.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := mustErr(e.svc.Create(ctx, Viewer{ID: "r3"}, in)); !errors.Is(err, domain.ErrPeriodTaken) {
		t.Errorf("periodo tomado = %v", err)
	}
	if err := mustErr(e.svc.Create(ctx, owner, in)); !errors.Is(err, domain.ErrOwnItem) {
		t.Errorf("propio = %v", err)
	}
}

func TestCreateFailures(t *testing.T) {
	ctx := context.Background()
	in := CreateInput{QuoteInput: QuoteInput{ItemID: "item1", Start: "2026-11-20", End: "2026-11-20", PaymentMode: domain.PaymentFullOnDelivery}, AcceptedTotalCents: 3000}

	// P-06: si el chat falla, la solicitud no se crea.
	e := newEnv(t, func(d *Deps) { d.Chat = fakeChat{err: errors.New("caído")} })
	if code(t, mustErr(e.svc.Create(ctx, req, in))) != shared.CodeUnavailable || len(e.repo.bookings) != 0 {
		t.Error("chat caído")
	}
	// Sin chat todavía (008): conversationId null.
	e = newEnv(t, func(d *Deps) { d.Chat = fakeChat{} })
	if v, _ := e.svc.Create(ctx, req, in); v.Booking.ConversationID != nil {
		t.Error("sin conversación")
	}
	// P-07: si el aviso falla, la solicitud se crea igual y se registra.
	e = newEnv(t)
	e.not.err = errors.New("push caído")
	if _, err := e.svc.Create(ctx, req, in); err != nil || !strings.Contains(e.logs.String(), "no se pudo notificar") {
		t.Errorf("aviso caído = %v", err)
	}
	for _, m := range []string{"HasBlockingOverlap", "HasPendingOverlap", "Create"} {
		e = newEnv(t)
		e.repo.fail[m] = errors.New("mongo")
		if err := mustErr(e.svc.Create(ctx, req, in)); err == nil {
			t.Errorf("%s: error no propagado", m)
		}
	}
	e = newEnv(t)
	e.repo.items["item1"] = domain.ItemForBooking{ID: "item1", OwnerID: "own", Status: "active", Modality: domain.ModalityRent, PricePerDayCents: 3000}
	if v, err := e.svc.Create(ctx, req, in); err != nil || v.ThumbURL != nil {
		t.Errorf("sin foto = %v", err)
	}
	e = newEnv(t, func(d *Deps) { d.Images = fakeImages{err: errors.New("url")} })
	if err := mustErr(e.svc.Create(ctx, req, in)); err == nil {
		t.Error("error de miniatura")
	}
	e = newEnv(t, func(d *Deps) { d.Users = fakeUsers{err: errors.New("users")} })
	if err := mustErr(e.svc.Create(ctx, req, in)); err == nil {
		t.Error("error de perfiles")
	}
}
