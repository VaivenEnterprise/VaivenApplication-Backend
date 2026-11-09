package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

type memRepo struct {
	items    map[string]domain.ItemForBooking
	bookings map[string]domain.Booking
	seq      int
	fail     map[string]error // por método
}

func newMemRepo() *memRepo {
	return &memRepo{items: map[string]domain.ItemForBooking{}, bookings: map[string]domain.Booking{}, fail: map[string]error{}}
}

func (r *memRepo) NewID() string { r.seq++; return fmt.Sprintf("%024x", r.seq) }

func (r *memRepo) Create(_ context.Context, b domain.Booking) error {
	if err := r.fail["Create"]; err != nil {
		return err
	}
	r.bookings[b.ID] = b
	return nil
}

func (r *memRepo) ByID(_ context.Context, id string) (domain.Booking, error) {
	b, ok := r.bookings[id]
	if !ok {
		return domain.Booking{}, domain.ErrNotFound
	}
	return b, nil
}

func (r *memRepo) Update(_ context.Context, b domain.Booking) (domain.Booking, error) {
	if err := r.fail["Update"]; err != nil {
		return domain.Booking{}, err
	}
	if cur, ok := r.bookings[b.ID]; !ok || cur.Version != b.Version {
		return domain.Booking{}, domain.ErrVersionConflict
	}
	b.Version++
	r.bookings[b.ID] = b
	return b, nil
}

func (r *memRepo) Accept(ctx context.Context, b domain.Booking, now time.Time) (domain.Booking, []domain.Booking, error) {
	if err := r.fail["Accept"]; err != nil {
		return domain.Booking{}, nil, err
	}
	if taken, _ := r.HasBlockingOverlap(ctx, b.ItemID, b.Start, b.End); taken {
		return domain.Booking{}, nil, domain.ErrPeriodTaken
	}
	acc, err := r.Update(ctx, b)
	if err != nil {
		return domain.Booking{}, nil, err
	}
	var rejected []domain.Booking
	reason := domain.RejectReasonUnavailable
	for _, o := range r.bookings {
		if o.ID != acc.ID && o.ItemID == acc.ItemID && o.Status == domain.StatusPending && o.Overlaps(acc.Start, acc.End) {
			_ = o.Transition(domain.StatusRejected, domain.RoleOwner, now)
			o.RejectReason = &reason
			saved, _ := r.Update(ctx, o)
			rejected = append(rejected, saved)
		}
	}
	return acc, rejected, nil
}

func (r *memRepo) HasBlockingOverlap(_ context.Context, itemID string, start, end domain.Date) (bool, error) {
	if err := r.fail["HasBlockingOverlap"]; err != nil {
		return false, err
	}
	for _, b := range r.bookings {
		if b.ItemID == itemID && b.Status.Blocks() && b.Overlaps(start, end) {
			return true, nil
		}
	}
	return false, nil
}

func (r *memRepo) HasPendingOverlap(_ context.Context, itemID, requester string, start, end domain.Date, now time.Time) (bool, error) {
	if err := r.fail["HasPendingOverlap"]; err != nil {
		return false, err
	}
	for _, b := range r.bookings {
		if b.ItemID == itemID && b.RequesterID == requester && b.EffectiveStatus(now) == domain.StatusPending && b.Overlaps(start, end) {
			return true, nil
		}
	}
	return false, nil
}

func (r *memRepo) ListByParty(_ context.Context, userID string, role domain.Role, status *domain.Status, after *domain.Cursor, limit int) ([]domain.Booking, error) {
	if err := r.fail["ListByParty"]; err != nil {
		return nil, err
	}
	var out []domain.Booking
	for _, b := range r.bookings {
		party := b.RequesterID
		if role == domain.RoleOwner {
			party = b.OwnerID
		}
		if party != userID || (status != nil && b.Status != *status) {
			continue
		}
		if after != nil && !(b.CreatedAt.Before(after.CreatedAt) || (b.CreatedAt.Equal(after.CreatedAt) && b.ID < after.ID)) {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *memRepo) DuePending(_ context.Context, now time.Time, limit int) ([]domain.Booking, error) {
	if err := r.fail["DuePending"]; err != nil {
		return nil, err
	}
	var out []domain.Booking
	for _, b := range r.bookings {
		if b.Status == domain.StatusPending && b.ExpiresAt != nil && !now.Before(*b.ExpiresAt) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (r *memRepo) ItemForBooking(_ context.Context, id string) (domain.ItemForBooking, error) {
	it, ok := r.items[id]
	if !ok {
		return domain.ItemForBooking{}, domain.ErrItemNotFound
	}
	return it, nil
}

type fakeImages struct{ err error }

func (fakeImages) Signature(context.Context, string, images.Purpose) (images.UploadSignature, error) {
	return images.UploadSignature{}, nil
}
func (fakeImages) Resolve(context.Context, string, images.Purpose, string, []images.Ref) ([]images.Image, error) {
	return nil, nil
}
func (f fakeImages) URLs(i images.Image) (images.URLs, error) {
	return images.URLs{URL: "u/" + i.PublicID, ThumbURL: "t/" + i.PublicID}, f.err
}

type fakeUsers struct{ err error }

func (f fakeUsers) PublicUsers(_ context.Context, ids []string) (map[string]users.PublicUser, error) {
	out := map[string]users.PublicUser{}
	for _, id := range ids {
		if id != "ghost" {
			out[id] = users.PublicUser{ID: id, DisplayName: "Persona " + id}
		}
	}
	return out, f.err
}

type fakeChat struct {
	id  string
	err error
}

func (f fakeChat) Open(context.Context, string, string, string) (string, error) { return f.id, f.err }

type sent struct{ to, typ string }

type fakeNotifier struct {
	sent []sent
	err  error
}

func (f *fakeNotifier) Notify(_ context.Context, userID string, e notify.Event) error {
	f.sent = append(f.sent, sent{userID, e.Type})
	return f.err
}

type env struct {
	svc  *Service
	repo *memRepo
	not  *fakeNotifier
	logs *bytes.Buffer
	now  *time.Time
}

// 2026-11-08 12:00 en Ciudad de México.
var t0 = time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC)

func newEnv(t *testing.T, mutate ...func(*Deps)) env {
	t.Helper()
	now := t0
	e := env{repo: newMemRepo(), not: &fakeNotifier{}, logs: &bytes.Buffer{}, now: &now}
	e.repo.items["item1"] = domain.ItemForBooking{ID: "item1", OwnerID: "own", Status: "active", Title: "Taladro",
		Modality: domain.ModalityRent, PricePerDayCents: 3000, FirstPhoto: &images.Image{PublicID: "p1"}}
	e.repo.items["loan1"] = domain.ItemForBooking{ID: "loan1", OwnerID: "own", Status: "active", Title: "Escalera", Modality: domain.ModalityLoan}
	d := Deps{Repo: e.repo, Images: fakeImages{}, Users: fakeUsers{}, Chat: fakeChat{id: "conv1"}, Notifier: e.not,
		Log: slog.New(slog.NewTextHandler(e.logs, nil)), Now: func() time.Time { return *e.now }}
	for _, m := range mutate {
		m(&d)
	}
	svc, err := NewService(d)
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

var (
	owner = Viewer{ID: "own"}
	req   = Viewer{ID: "req"}
	other = Viewer{ID: "otro"}
)

func create(t *testing.T, e env, v Viewer, start, end string) BookingView {
	t.Helper()
	got, err := e.svc.Create(context.Background(), v, CreateInput{
		QuoteInput:         QuoteInput{ItemID: "item1", Start: start, End: end, PaymentMode: domain.PaymentSplit5050},
		AcceptedTotalCents: int64(domain.Days(domain.MustDate(start), domain.MustDate(end))) * 3000,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return got
}

func code(t *testing.T, err error) shared.Code {
	t.Helper()
	var de *shared.Error
	if !errors.As(err, &de) {
		t.Fatalf("err = %v, no es de dominio", err)
	}
	return de.Code
}

func mustErr[T any](_ T, err error) error { return err }
