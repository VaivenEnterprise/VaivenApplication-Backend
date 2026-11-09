package usecase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

var ctx = context.Background()

func TestNewServiceRequiresDeps(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	e := newEnv(t, bookings.StatusAccepted)
	if _, err := NewService(Deps{Repo: e.repo, Bookings: e.repo, Images: e.imgs, Notifier: e.notif, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}); err != nil {
		t.Errorf("con lo mínimo debe crear (hora y zona por defecto): %v", err)
	}
}

func TestRecordEvidenceDelivery(t *testing.T) {
	e := newEnv(t, bookings.StatusAccepted)
	v, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "  Rayón leve en la tapa ", refs("p1", "p2"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Stage.EvidenceVersion != 1 || v.Stage.ConditionNotes != "Rayón leve en la tapa" || len(v.Stage.Photos) != 2 {
		t.Errorf("stage = %+v", v.Stage)
	}
	if len(v.URLs) != 2 || v.URLs[0].URL != "signed://p1" {
		t.Errorf("URLs = %+v", v.URLs)
	}
	if v.QR != domain.QRNone || v.QRExpiresAt != nil {
		t.Errorf("sin QR aún: %v %v", v.QR, v.QRExpiresAt)
	}
	if e.imgs.gotUser != "owner" || e.imgs.gotPurpose != images.PurposeEvidence {
		t.Errorf("Resolve(%q, %q): debe usar el propósito evidence y a quien sube", e.imgs.gotUser, e.imgs.gotPurpose)
	}
	if e.repo.b.Delivery == nil || e.repo.b.Status != bookings.StatusAccepted {
		t.Error("la evidencia no cambia el estado de la reserva")
	}
}

func TestRecordEvidenceReplacementInvalidatesQR(t *testing.T) {
	e := newEnv(t, bookings.StatusAccepted)
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "Primera", refs("p1")); err != nil {
		t.Fatal(err)
	}
	q, err := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "Segunda", refs("p2"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Stage.EvidenceVersion != 2 || v.QR != domain.QRNone {
		t.Errorf("v2 sin QR esperado, got version=%d qr=%s", v.Stage.EvidenceVersion, v.QR)
	}
	// CA-04: el QR anterior ya no sirve.
	err = e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, q.Token)
	if !errors.Is(err, domain.ErrQRInvalid) {
		t.Errorf("CA-04: err = %v, quiero ErrQRInvalid", err)
	}
}

func TestRecordEvidenceErrors(t *testing.T) {
	cases := []struct {
		name   string
		status bookings.Status
		user   string
		kind   domain.Kind
		notes  string
		refs   []images.Ref
		want   error
	}{
		{"tercero → 404", bookings.StatusAccepted, "otro", domain.KindDelivery, "ok ok", refs("p"), bookings.ErrNotFound},
		{"solicitante no registra la entrega", bookings.StatusAccepted, "req", domain.KindDelivery, "ok ok", refs("p"), domain.ErrForbidden},
		{"reserva pendiente", bookings.StatusPending, "owner", domain.KindDelivery, "ok ok", refs("p"), domain.ErrWrongStatus},
		{"devolución antes de la entrega", bookings.StatusAccepted, "req", domain.KindReturn, "ok ok", refs("p"), domain.ErrWrongStatus},
		{"notas cortas", bookings.StatusAccepted, "owner", domain.KindDelivery, "ab", refs("p"), domain.ErrConditionNotes},
		{"sin fotos", bookings.StatusAccepted, "owner", domain.KindDelivery, "ok ok", nil, domain.ErrPhotoCount},
		{"seis fotos", bookings.StatusAccepted, "owner", domain.KindDelivery, "ok ok", refs("1", "2", "3", "4", "5", "6"), domain.ErrPhotoCount},
		{"etapa inválida", bookings.StatusAccepted, "owner", domain.Kind("x"), "ok ok", refs("p"), bookings.ErrNotFound},
	}
	for _, c := range cases {
		e := newEnv(t, c.status)
		if _, err := e.svc.RecordEvidence(ctx, Viewer{c.user}, "b1", c.kind, c.notes, c.refs); err != c.want {
			t.Errorf("%s: err = %v, quiero %v", c.name, err, c.want)
		}
	}
	e := newEnv(t, bookings.StatusAccepted)
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "otra", domain.KindDelivery, "ok ok", refs("p")); err != bookings.ErrNotFound {
		t.Errorf("reserva inexistente: %v", err)
	}
	e.imgs.resolveErr = errors.New("foto ajena")
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "ok ok", refs("p")); err == nil || err.Error() != "foto ajena" {
		t.Errorf("el error de images.Resolve se propaga: %v", err)
	}
}

func TestRecordEvidenceAfterConfirmationIsRejected(t *testing.T) {
	e := newEnv(t, bookings.StatusAccepted)
	confirmDelivery(t, e)
	// Ya está in_progress: la evidencia de entrega confirmada no cambia (RF-13).
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "cambio", refs("p9")); err != domain.ErrAlreadyConfirmed {
		t.Errorf("err = %v, quiero ErrAlreadyConfirmed", err)
	}
}

func TestRecordEvidenceLosesRace(t *testing.T) {
	e := newEnv(t, bookings.StatusAccepted)
	// La reserva se cancela entre la lectura y el guardado: el repositorio no aplica.
	racy := &racyRepo{memRepo: e.repo, before: func() { e.repo.mu.Lock(); e.repo.b.Status = bookings.StatusCancelled; e.repo.mu.Unlock() }}
	svc, _ := NewService(Deps{Repo: racy, Bookings: racy, Images: e.imgs, Notifier: e.notif, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return t0 }})
	if _, err := svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "ok ok", refs("p")); err != domain.ErrWrongStatus {
		t.Errorf("err = %v, quiero ErrWrongStatus", err)
	}
}

// racyRepo ejecuta before justo antes de cada escritura.
type racyRepo struct {
	*memRepo
	before func()
}

func (r *racyRepo) SaveEvidence(c context.Context, id string, k domain.Kind, h bookings.Handover) error {
	r.before()
	return r.memRepo.SaveEvidence(c, id, k, h)
}
func (r *racyRepo) SaveQR(c context.Context, id string, k domain.Kind, qr bookings.QR) error {
	r.before()
	return r.memRepo.SaveQR(c, id, k, qr)
}
func (r *racyRepo) Confirm(c context.Context, id string, cf domain.Confirmation, h string) error {
	r.before()
	return r.memRepo.Confirm(c, id, cf, h)
}

func TestIssueQR(t *testing.T) {
	e := newEnv(t, bookings.StatusAccepted)
	if _, err := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery); err != domain.ErrNoEvidence {
		t.Errorf("sin evidencia: %v", err)
	}
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "Todo bien", refs("p1")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.IssueQR(ctx, Viewer{"req"}, "b1", domain.KindDelivery); err != domain.ErrForbidden {
		t.Errorf("el solicitante no genera el QR de entrega: %v", err)
	}
	if _, err := e.svc.IssueQR(ctx, Viewer{"otro"}, "b1", domain.KindDelivery); err != bookings.ErrNotFound {
		t.Errorf("tercero: %v", err)
	}
	q, err := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Token) != 43 || !q.ExpiresAt.Equal(t0.Add(15*time.Minute)) {
		t.Errorf("QR = %+v", q)
	}
	if q.Payload != "vaiven://qr?b=b1&s=delivery&t="+q.Token {
		t.Errorf("payload = %q", q.Payload)
	}
	// RNF-01: en la reserva solo queda el hash, nunca el token.
	st := e.repo.b.Delivery.QR
	if st == nil || st.TokenHash != domain.HashToken(q.Token) || strings.Contains(st.TokenHash, q.Token) {
		t.Errorf("QR guardado = %+v", st)
	}
	// Generar otro invalida el anterior.
	q2, err := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery)
	if err != nil || q2.Token == q.Token {
		t.Fatalf("segundo QR: %v", err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, q.Token); err != domain.ErrQRInvalid {
		t.Errorf("el QR anterior ya no vale: %v", err)
	}
	if err := e.svc.Confirm(ctx, Viewer{"req"}, "b1", domain.KindDelivery, q2.Token); err != nil {
		t.Errorf("el nuevo sí: %v", err)
	}
}

func TestIssueQRFailures(t *testing.T) {
	e := newEnv(t, bookings.StatusAccepted)
	if _, err := e.svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.Kind("x")); err != bookings.ErrNotFound {
		t.Errorf("etapa inválida: %v", err)
	}
	if _, err := e.svc.RecordEvidence(ctx, Viewer{"owner"}, "b1", domain.KindDelivery, "Todo bien", refs("p1")); err != nil {
		t.Fatal(err)
	}
	// La evidencia cambia entre la lectura y el guardado del QR: no aplica.
	racy := &racyRepo{memRepo: e.repo, before: func() {
		e.repo.mu.Lock()
		h := *e.repo.b.Delivery
		h.EvidenceVersion++
		e.repo.b.Delivery = &h
		e.repo.mu.Unlock()
	}}
	svc, _ := NewService(Deps{Repo: racy, Bookings: racy, Images: e.imgs, Notifier: e.notif, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return t0 }})
	if _, err := svc.IssueQR(ctx, Viewer{"owner"}, "b1", domain.KindDelivery); err != domain.ErrWrongStatus {
		t.Errorf("err = %v, quiero ErrWrongStatus", err)
	}
}
