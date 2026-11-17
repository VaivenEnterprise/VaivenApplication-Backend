package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

const (
	bid         = "6731a0000000000000000001"
	ownerID     = "6730f1c2a9b3e4d5f6a7b8c1"
	requesterID = "6730f1c2a9b3e4d5f6a7b8c2"
	strangerID  = "6730f1c2a9b3e4d5f6a7b8c3"
)

var completedAt = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

// --- dobles ---

type fakeBookings struct {
	b   bookings.Booking
	err error
}

func (f fakeBookings) ByID(_ context.Context, id string) (bookings.Booking, error) {
	if f.err != nil {
		return bookings.Booking{}, f.err
	}
	if id != f.b.ID {
		return bookings.Booking{}, bookings.ErrNotFound
	}
	return f.b, nil
}

type fakeRepo struct {
	rows      []domain.Rating
	next      int
	createErr error
	listErr   error
	findErr   error
	after     string
	limit     int
}

func (f *fakeRepo) NewID() string { f.next++; return fmt.Sprintf("%024x", f.next) }
func (f *fakeRepo) Create(_ context.Context, r domain.Rating) (domain.Rating, error) {
	if f.createErr != nil {
		return domain.Rating{}, f.createErr
	}
	for _, x := range f.rows {
		if x.BookingID == r.BookingID && x.RaterID == r.RaterID {
			return domain.Rating{}, domain.ErrAlreadyRated
		}
	}
	f.rows = append(f.rows, r)
	return r, nil
}
func (f *fakeRepo) ByBookingAndRater(_ context.Context, bookingID, raterID string) (domain.Rating, bool, error) {
	if f.findErr != nil {
		return domain.Rating{}, false, f.findErr
	}
	for _, x := range f.rows {
		if x.BookingID == bookingID && x.RaterID == raterID {
			return x, true, nil
		}
	}
	return domain.Rating{}, false, nil
}
func (f *fakeRepo) ListByRatee(_ context.Context, rateeID, after string, limit int) ([]domain.Rating, error) {
	f.after, f.limit = after, limit
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []domain.Rating
	for i := len(f.rows) - 1; i >= 0; i-- { // más recientes primero
		r := f.rows[i]
		if r.RateeID == rateeID && (after == "" || r.ID < after) {
			out = append(out, r)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type fakeUsers struct {
	profiles map[string]users.PublicUser
	err      error
	calls    int
}

func (f *fakeUsers) PublicUsers(_ context.Context, ids []string) (map[string]users.PublicUser, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]users.PublicUser{}
	for _, id := range ids {
		if p, ok := f.profiles[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

type fakeNotifier struct {
	sent []sentEvent
	err  error
}

type sentEvent struct {
	to string
	e  notify.Event
}

func (f *fakeNotifier) Notify(_ context.Context, to string, e notify.Event) error {
	f.sent = append(f.sent, sentEvent{to, e})
	return f.err
}

type env struct {
	svc   *Service
	repo  *fakeRepo
	users *fakeUsers
	not   *fakeNotifier
	logs  *bytes.Buffer
	clock *time.Time
}

func completed() bookings.Booking {
	return bookings.Booking{
		ID: bid, OwnerID: ownerID, RequesterID: requesterID, Status: bookings.StatusCompleted,
		History: []bookings.HistoryEntry{{Status: bookings.StatusCompleted, At: completedAt, By: bookings.RoleSystem}},
	}
}

func newEnv(t *testing.T, b bookings.Booking) *env {
	t.Helper()
	now := completedAt.Add(time.Hour)
	e := &env{
		repo: &fakeRepo{}, not: &fakeNotifier{}, logs: &bytes.Buffer{}, clock: &now,
		users: &fakeUsers{profiles: map[string]users.PublicUser{
			ownerID: {ID: ownerID, DisplayName: "Beto"}, requesterID: {ID: requesterID, DisplayName: "Ana"},
		}},
	}
	svc, err := NewService(Deps{
		Repo: e.repo, Bookings: fakeBookings{b: b}, Users: e.users, Notifier: e.not,
		Log: slog.New(slog.NewTextHandler(e.logs, nil)), Now: func() time.Time { return *e.clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

func str(s string) *string { return &s }

func TestNewServiceRequiresDeps(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	e := newEnv(t, completed())
	d := Deps{Repo: e.repo, Bookings: fakeBookings{}, Users: e.users, Notifier: e.not, Log: slog.Default()}
	svc, err := NewService(d)
	if err != nil || svc.d.Now == nil {
		t.Errorf("NewService = %v, %v", svc, err)
	}
}

// --- Rate ---

func TestRateSavesAndNotifiesTheOtherParty(t *testing.T) { // CA-01
	e := newEnv(t, completed())
	v, err := e.svc.Rate(context.Background(), requesterID, bid, 5, str("  Excelente  "))
	if err != nil {
		t.Fatal(err)
	}
	r := v.Rating
	if r.Score != 5 || r.RateeID != ownerID || r.RaterID != requesterID || r.RatedRole != bookings.RoleOwner ||
		r.Comment == nil || *r.Comment != "Excelente" || r.BookingID != bid || !r.CreatedAt.Equal(*e.clock) {
		t.Errorf("rating = %+v", r)
	}
	if v.RaterName != "Ana" {
		t.Errorf("RaterName = %q", v.RaterName)
	}
	if len(e.repo.rows) != 1 {
		t.Fatalf("guardadas = %d", len(e.repo.rows))
	}
	if len(e.not.sent) != 1 || e.not.sent[0].to != ownerID || e.not.sent[0].e.Type != EventRatingReceived ||
		e.not.sent[0].e.Data["bookingId"] != bid {
		t.Errorf("avisos = %+v", e.not.sent)
	}
	for _, s := range []string{"Excelente", "comment"} {
		if strings.Contains(fmt.Sprint(e.not.sent[0].e), s) {
			t.Errorf("el aviso no debe llevar el comentario (%q)", s)
		}
	}
}

func TestRateRejections(t *testing.T) {
	inProgress := completed()
	inProgress.Status = bookings.StatusInProgress

	tests := []struct {
		name  string
		b     bookings.Booking
		user  string
		score int
		now   time.Time
		want  error
	}{
		{"no completada (CA-02)", inProgress, ownerID, 5, completedAt, domain.ErrNotCompleted},
		{"ajeno", completed(), strangerID, 5, completedAt, bookings.ErrNotFound},
		{"plazo vencido", completed(), ownerID, 5, completedAt.AddDate(0, 0, 14), domain.ErrWindowClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, tt.b)
			*e.clock = tt.now
			if _, err := e.svc.Rate(context.Background(), tt.user, bid, tt.score, nil); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if len(e.repo.rows) != 0 || len(e.not.sent) != 0 {
				t.Error("un rechazo no guarda ni avisa")
			}
		})
	}

	e := newEnv(t, completed())
	for _, score := range []int{0, 6} {
		if _, err := e.svc.Rate(context.Background(), ownerID, bid, score, nil); err == nil {
			t.Errorf("score %d debe ser inválido", score)
		}
	}
	if _, err := e.svc.Rate(context.Background(), ownerID, bid, 4, str(strings.Repeat("x", 301))); err == nil {
		t.Error("comentario largo debe ser inválido")
	}
	if _, err := e.svc.Rate(context.Background(), ownerID, "6731a0000000000000000099", 4, nil); !errors.Is(err, bookings.ErrNotFound) {
		t.Errorf("reserva inexistente: %v", err)
	}
	boom := errors.New("mongo caído")
	e.svc.d.Bookings = fakeBookings{err: boom}
	if _, err := e.svc.Rate(context.Background(), ownerID, bid, 4, nil); !errors.Is(err, boom) {
		t.Errorf("error de lectura: %v", err)
	}
}

func TestRateTwiceIsConflict(t *testing.T) { // CA-03
	e := newEnv(t, completed())
	if _, err := e.svc.Rate(context.Background(), ownerID, bid, 4, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Rate(context.Background(), ownerID, bid, 1, nil); !errors.Is(err, domain.ErrAlreadyRated) {
		t.Errorf("segunda calificación: %v", err)
	}
	if len(e.repo.rows) != 1 || len(e.not.sent) != 1 {
		t.Errorf("guardadas=%d avisos=%d, quiero 1 y 1", len(e.repo.rows), len(e.not.sent))
	}
	// La contraparte sí puede calificar.
	if _, err := e.svc.Rate(context.Background(), requesterID, bid, 5, nil); err != nil {
		t.Errorf("la contraparte debe poder calificar: %v", err)
	}
}

func TestRateRepoFailureAndNotifierFailure(t *testing.T) {
	e := newEnv(t, completed())
	boom := errors.New("transacción fallida")
	e.repo.createErr = boom
	if _, err := e.svc.Rate(context.Background(), ownerID, bid, 4, nil); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
	if len(e.not.sent) != 0 {
		t.Error("si no se guardó no se avisa")
	}

	e = newEnv(t, completed())
	e.not.err = errors.New("push caído")
	if _, err := e.svc.Rate(context.Background(), ownerID, bid, 4, nil); err != nil {
		t.Fatalf("un fallo al avisar no debe deshacer la calificación: %v", err)
	}
	if !strings.Contains(e.logs.String(), "no se pudo notificar") || len(e.repo.rows) != 1 {
		t.Errorf("logs=%q guardadas=%d", e.logs.String(), len(e.repo.rows))
	}
}

func TestRateNameFallsBackWhenProfileUnavailable(t *testing.T) {
	e := newEnv(t, completed())
	e.users.err = errors.New("users caído")
	v, err := e.svc.Rate(context.Background(), ownerID, bid, 4, nil)
	if err != nil || v.RaterName != DeletedName {
		t.Errorf("v=%+v err=%v", v, err)
	}
	e = newEnv(t, completed())
	delete(e.users.profiles, ownerID)
	if v, _ := e.svc.Rate(context.Background(), ownerID, bid, 4, nil); v.RaterName != DeletedName {
		t.Errorf("sin perfil debe decir %q, dice %q", DeletedName, v.RaterName)
	}
}

// --- Status ---

func TestStatus(t *testing.T) {
	ctx := context.Background()
	t.Run("puede calificar", func(t *testing.T) {
		e := newEnv(t, completed())
		v, err := e.svc.Status(ctx, ownerID, bid)
		if err != nil || !v.Status.CanRate || v.Status.MyRating != nil || v.Status.ExpiresAt == nil ||
			!v.Status.ExpiresAt.Equal(completedAt.AddDate(0, 0, 14)) {
			t.Errorf("v = %+v, err = %v", v, err)
		}
	})
	t.Run("ya calificó", func(t *testing.T) {
		e := newEnv(t, completed())
		if _, err := e.svc.Rate(ctx, ownerID, bid, 4, str("bien")); err != nil {
			t.Fatal(err)
		}
		v, err := e.svc.Status(ctx, ownerID, bid)
		if err != nil || v.Status.CanRate || v.Status.MyRating == nil || v.Status.MyRating.Score != 4 || v.MyRaterName != "Beto" {
			t.Errorf("v = %+v, err = %v", v, err)
		}
		other, _ := e.svc.Status(ctx, requesterID, bid)
		if !other.Status.CanRate || other.Status.MyRating != nil {
			t.Errorf("la otra parte aún puede calificar: %+v", other)
		}
	})
	t.Run("aún no completada", func(t *testing.T) {
		b := completed()
		b.Status, b.History = bookings.StatusInProgress, nil
		v, err := newEnv(t, b).svc.Status(ctx, ownerID, bid)
		if err != nil || v.Status.CanRate || v.Status.ExpiresAt != nil || v.Status.MyRating != nil {
			t.Errorf("v = %+v, err = %v", v, err)
		}
	})
	t.Run("plazo vencido", func(t *testing.T) {
		e := newEnv(t, completed())
		*e.clock = completedAt.AddDate(0, 0, 15)
		v, err := e.svc.Status(ctx, ownerID, bid)
		if err != nil || v.Status.CanRate || v.Status.ExpiresAt == nil {
			t.Errorf("v = %+v, err = %v", v, err)
		}
	})
	t.Run("ajeno", func(t *testing.T) {
		if _, err := newEnv(t, completed()).svc.Status(ctx, strangerID, bid); !errors.Is(err, bookings.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("reserva inexistente", func(t *testing.T) {
		if _, err := newEnv(t, completed()).svc.Status(ctx, ownerID, "6731a0000000000000000099"); !errors.Is(err, bookings.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("error del repositorio", func(t *testing.T) {
		e := newEnv(t, completed())
		boom := errors.New("mongo caído")
		e.repo.findErr = boom
		if _, err := e.svc.Status(ctx, ownerID, bid); !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
	})
}

// --- List ---

func seedRatings(e *env, n int) {
	for i := 0; i < n; i++ {
		raterID := requesterID
		if i%2 == 1 {
			raterID = strangerID // sin perfil: «Cuenta eliminada»
		}
		e.repo.rows = append(e.repo.rows, domain.Rating{
			ID: e.repo.NewID(), BookingID: fmt.Sprintf("b%d", i), RaterID: raterID, RateeID: ownerID,
			RatedRole: bookings.RoleOwner, Score: i%5 + 1, CreatedAt: completedAt.Add(time.Duration(i) * time.Minute),
		})
	}
}

func TestListPagesNewestFirstWithNames(t *testing.T) { // RF-04
	e := newEnv(t, completed())
	seedRatings(e, 5)
	ctx := context.Background()

	p1, err := e.svc.List(ctx, ownerID, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.Items) != 2 || p1.Next == nil || *p1.Next != p1.Items[1].Rating.ID {
		t.Fatalf("p1 = %+v", p1)
	}
	if p1.Items[0].Rating.BookingID != "b4" || p1.Items[1].Rating.BookingID != "b3" {
		t.Errorf("orden = %s, %s; quiero b4, b3", p1.Items[0].Rating.BookingID, p1.Items[1].Rating.BookingID)
	}
	if p1.Items[0].RaterName != "Ana" || p1.Items[1].RaterName != DeletedName {
		t.Errorf("nombres = %q, %q", p1.Items[0].RaterName, p1.Items[1].RaterName)
	}
	if e.repo.limit != 3 {
		t.Errorf("se debe pedir uno más para saber si hay otra página: limit=%d", e.repo.limit)
	}

	p2, err := e.svc.List(ctx, ownerID, *p1.Next, 2)
	if err != nil || len(p2.Items) != 2 || p2.Next == nil || e.repo.after != *p1.Next {
		t.Fatalf("p2 = %+v, err = %v", p2, err)
	}
	p3, err := e.svc.List(ctx, ownerID, *p2.Next, 2)
	if err != nil || len(p3.Items) != 1 || p3.Next != nil {
		t.Errorf("p3 = %+v, err = %v", p3, err)
	}
}

func TestListLimitsAndEmpty(t *testing.T) {
	e := newEnv(t, completed())
	ctx := context.Background()
	if _, err := e.svc.List(ctx, ownerID, "", 0); err != nil || e.repo.limit != DefaultPage+1 {
		t.Errorf("limit por omisión: %d (%v)", e.repo.limit, err)
	}
	if _, err := e.svc.List(ctx, ownerID, "", 500); err != nil || e.repo.limit != MaxPage+1 {
		t.Errorf("limit máximo: %d (%v)", e.repo.limit, err)
	}
	p, err := e.svc.List(ctx, ownerID, "", 10)
	if err != nil || p.Items == nil || len(p.Items) != 0 || p.Next != nil {
		t.Errorf("página vacía = %+v (%v)", p, err)
	}
	e.users.calls = 0
	_, _ = e.svc.List(ctx, ownerID, "", 10)
	if e.users.calls != 1 {
		t.Errorf("sin calificaciones no se piden nombres: %d llamadas a users", e.users.calls)
	}
}

func TestListErrors(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, completed())
	if _, err := e.svc.List(ctx, strangerID, "", 10); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("persona inexistente: %v", err)
	}
	boom := errors.New("caído")
	e.repo.listErr = boom
	if _, err := e.svc.List(ctx, ownerID, "", 10); !errors.Is(err, boom) {
		t.Errorf("repo: %v", err)
	}
	e = newEnv(t, completed())
	e.users.err = boom
	if _, err := e.svc.List(ctx, ownerID, "", 10); !errors.Is(err, boom) {
		t.Errorf("users: %v", err)
	}
}

func TestListNamesLookupFailure(t *testing.T) {
	e := newEnv(t, completed())
	seedRatings(e, 1)
	// La primera lectura (perfil de la persona) pasa; la segunda (nombres) falla.
	e.svc.d.Users = &countingUsers{inner: e.users, failOn: 2}
	if _, err := e.svc.List(context.Background(), ownerID, "", 10); err == nil {
		t.Error("si falla la lectura de nombres debe fallar la lista")
	}
}

type countingUsers struct {
	inner  *fakeUsers
	failOn int
	n      int
}

func (c *countingUsers) PublicUsers(ctx context.Context, ids []string) (map[string]users.PublicUser, error) {
	c.n++
	if c.n == c.failOn {
		return nil, errors.New("caído")
	}
	return c.inner.PublicUsers(ctx, ids)
}
