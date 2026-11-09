package usecase

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/handover/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

var (
	t0  = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC) // 12:00 en México, día de inicio
	loc = func() *time.Location { l, _ := bookings.Location(); return l }()
)

// memRepo guarda una reserva en memoria y aplica los mismos filtros que el
// repositorio real, bajo un candado (equivale a la atomicidad de un documento).
type memRepo struct {
	mu sync.Mutex
	b  bookings.Booking
}

func (m *memRepo) ByID(_ context.Context, id string) (bookings.Booking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id != m.b.ID {
		return bookings.Booking{}, bookings.ErrNotFound
	}
	return m.b, nil
}

func (m *memRepo) stage(k domain.Kind) **bookings.Handover {
	if k == domain.KindReturn {
		return &m.b.Return
	}
	return &m.b.Delivery
}

func (m *memRepo) SaveEvidence(_ context.Context, id string, k domain.Kind, h bookings.Handover) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := *m.stage(k)
	if id != m.b.ID || m.b.Status != k.RequiredStatus() || (cur != nil && cur.ConfirmedAt != nil) {
		return ErrNoMatch
	}
	*m.stage(k) = &h
	m.b.Version++
	return nil
}

func (m *memRepo) SaveQR(_ context.Context, id string, k domain.Kind, qr bookings.QR) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := *m.stage(k)
	if id != m.b.ID || cur == nil || cur.ConfirmedAt != nil || cur.EvidenceVersion != qr.EvidenceVersion || m.b.Status != k.RequiredStatus() {
		return ErrNoMatch
	}
	c := *cur
	c.QR = &qr
	*m.stage(k) = &c
	m.b.Version++
	return nil
}

func (m *memRepo) Confirm(_ context.Context, id string, c domain.Confirmation, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := *m.stage(c.Kind)
	if id != m.b.ID || m.b.Status != c.Kind.RequiredStatus() || cur == nil || cur.ConfirmedAt != nil ||
		cur.QR == nil || cur.QR.TokenHash != tokenHash || cur.QR.UsedAt != nil ||
		!c.At.Before(cur.QR.ExpiresAt) || cur.QR.EvidenceVersion != cur.EvidenceVersion {
		return ErrNoMatch
	}
	n := *cur
	qr := *cur.QR
	used, at, by := c.At, c.At, c.ConfirmedBy
	qr.UsedAt = &used
	n.QR, n.ConfirmedAt, n.ConfirmedByID = &qr, &at, &by
	*m.stage(c.Kind) = &n
	m.b.Status = c.NewStatus
	m.b.History = append(m.b.History, c.History)
	if c.LateAt != nil {
		m.b.LateAt = c.LateAt
	}
	if c.Adjustment != nil {
		m.b.Adjustments = append(m.b.Adjustments, *c.Adjustment)
	}
	m.b.Version++
	return nil
}

type fakeImages struct {
	resolveErr error
	gotUser    string
	gotPurpose images.Purpose
}

func (f *fakeImages) Signature(context.Context, string, images.Purpose) (images.UploadSignature, error) {
	return images.UploadSignature{}, nil
}

func (f *fakeImages) Resolve(_ context.Context, userID string, p images.Purpose, _ string, refs []images.Ref) ([]images.Image, error) {
	f.gotUser, f.gotPurpose = userID, p
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	out := make([]images.Image, 0, len(refs))
	for _, r := range refs {
		out = append(out, images.Image{PublicID: r.PublicID, Delivery: images.DeliveryAuthenticated})
	}
	return out, nil
}

func (f *fakeImages) URLs(img images.Image) (images.URLs, error) {
	return images.URLs{URL: "signed://" + img.PublicID, ThumbURL: "signed://thumb/" + img.PublicID}, nil
}

type fakeNotifier struct {
	mu     sync.Mutex
	events map[string][]notify.Event
	err    error
}

func (n *fakeNotifier) Notify(_ context.Context, userID string, e notify.Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.events == nil {
		n.events = map[string][]notify.Event{}
	}
	n.events[userID] = append(n.events[userID], e)
	return n.err
}

type fakeLateness struct{ adj *bookings.Adjustment }

func (fakeLateness) LateAt(b bookings.Booking) time.Time {
	return b.End.AddDays(1).StartOf(loc).Add(2 * time.Hour)
}
func (f fakeLateness) Adjustment(bookings.Booking, time.Time) *bookings.Adjustment { return f.adj }

type env struct {
	svc   *Service
	repo  *memRepo
	imgs  *fakeImages
	notif *fakeNotifier
	now   *time.Time
}

func newEnv(t *testing.T, status bookings.Status) *env {
	t.Helper()
	now := t0
	e := &env{
		repo: &memRepo{b: bookings.Booking{
			ID: "b1", ItemID: "i1", OwnerID: "owner", RequesterID: "req", Status: status,
			Start: bookings.MustDate("2026-11-20"), End: bookings.MustDate("2026-11-22"), Version: 1,
		}},
		imgs: &fakeImages{}, notif: &fakeNotifier{}, now: &now,
	}
	svc, err := NewService(Deps{
		Repo: e.repo, Bookings: e.repo, Images: e.imgs, Notifier: e.notif,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Lateness: fakeLateness{},
		Now: func() time.Time { return *e.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

func refs(ids ...string) []images.Ref {
	out := make([]images.Ref, 0, len(ids))
	for _, id := range ids {
		out = append(out, images.Ref{PublicID: id})
	}
	return out
}
