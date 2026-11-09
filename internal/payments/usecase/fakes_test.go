package usecase

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/payments/domain"
)

var t0 = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

// memRepo es un Repository en memoria con los mismos filtros que el real,
// bajo un candado (equivale a la atomicidad de un documento).
type memRepo struct {
	mu sync.Mutex
	bs map[string]*bookings.Booking
}

func newMemRepo(bs ...bookings.Booking) *memRepo {
	m := &memRepo{bs: map[string]*bookings.Booking{}}
	for _, b := range bs {
		b := b
		m.bs[b.ID] = &b
	}
	return m
}

func (m *memRepo) ByID(_ context.Context, id string) (bookings.Booking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bs[id]
	if !ok {
		return bookings.Booking{}, bookings.ErrNotFound
	}
	cp := *b
	cp.Payments = append([]bookings.BookingPayment(nil), b.Payments...)
	return cp, nil
}

func (m *memRepo) ChangePayment(_ context.Context, id string, c domain.Change) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bs[id]
	if !ok || (b.Status != bookings.StatusAccepted && b.Status != bookings.StatusInProgress && b.Status != bookings.StatusCompleted) {
		return ErrNoMatch
	}
	for i := range b.Payments {
		p := &b.Payments[i]
		if p.Stage == c.Stage && p.Status == c.From {
			p.Status = c.To
			at := c.At
			if c.To == bookings.PaymentReported {
				p.ReportedAt, p.ReportedNote = &at, c.Note
			} else {
				p.ConfirmedAt = &at
			}
			b.Version++
			return nil
		}
	}
	return ErrNoMatch
}

func lateOf(b *bookings.Booking) LateBooking {
	return LateBooking{ID: b.ID, ItemID: b.ItemID, OwnerID: b.OwnerID, RequesterID: b.RequesterID, End: b.End}
}

func (m *memRepo) DueLate(_ context.Context, now time.Time, limit int) ([]LateBooking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []LateBooking
	for _, b := range m.bs {
		if b.Status == bookings.StatusInProgress && !b.IsLate && b.LateAt != nil && b.LateAt.Before(now) && len(out) < limit {
			out = append(out, lateOf(b))
		}
	}
	return out, nil
}

func (m *memRepo) MarkLate(_ context.Context, id string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bs[id]
	if !ok || b.Status != bookings.StatusInProgress || b.IsLate {
		return ErrNoMatch
	}
	b.IsLate = true
	b.Version++
	return nil
}

func (m *memRepo) AffectedCandidates(_ context.Context, late LateBooking, today bookings.Date) ([]LateBooking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []LateBooking
	for _, b := range m.bs {
		if b.ItemID == late.ItemID && b.Status == bookings.StatusAccepted && b.AffectedBy == nil &&
			b.Start.After(late.End) && !b.Start.After(today) {
			out = append(out, lateOf(b))
		}
	}
	return out, nil
}

func (m *memRepo) MarkAffected(_ context.Context, id, lateID string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bs[id]
	if !ok || b.Status != bookings.StatusAccepted || b.AffectedBy != nil {
		return ErrNoMatch
	}
	b.AffectedBy = &lateID
	b.Version++
	return nil
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []string // "tipo:destinatario"
	err  error
}

func (n *fakeNotifier) Notify(_ context.Context, to string, e notify.Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, e.Type+":"+to)
	return n.err
}

func (n *fakeNotifier) count(prefix string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := 0
	for _, s := range n.sent {
		if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
			c++
		}
	}
	return c
}

type env struct {
	svc  *Service
	repo *memRepo
	not  *fakeNotifier
	now  *time.Time
}

func newEnv(t *testing.T, bs ...bookings.Booking) *env {
	t.Helper()
	now := t0
	e := &env{repo: newMemRepo(bs...), not: &fakeNotifier{}, now: &now}
	svc, err := NewService(Deps{
		Repo: e.repo, Bookings: e.repo, Notifier: e.not,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return *e.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

func booking(id string, status bookings.Status, pays ...bookings.BookingPayment) bookings.Booking {
	return bookings.Booking{
		ID: id, ItemID: "i1", OwnerID: "owner", RequesterID: "req", Status: status,
		Start: bookings.MustDate("2026-11-20"), End: bookings.MustDate("2026-11-22"),
		Price:    bookings.PriceSnapshot{Modality: bookings.ModalityRent, PricePerDayCents: 3000},
		Payments: pays,
	}
}
