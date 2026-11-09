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
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

const (
	bid         = "6731a0000000000000000001"
	ownerID     = "6730f1c2a9b3e4d5f6a7b8c1"
	requesterID = "6730f1c2a9b3e4d5f6a7b8c2"
	strangerID  = "6730f1c2a9b3e4d5f6a7b8c3"
	adminID     = "6730f1c2a9b3e4d5f6a7b8c4"
)

var (
	loc = func() *time.Location {
		l, err := bookings.Location()
		if err != nil {
			panic(err)
		}
		return l
	}()
	// Dentro de la reserva: ya empezó (inicio 18 nov) y aún no termina.
	midBooking  = time.Date(2026, 11, 19, 12, 0, 0, 0, loc)
	completedAt = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)
)

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

// fakeRepo imita la semántica del repositorio de MongoDB, incluidas las
// operaciones atómicas (AddNote y Close devuelven ErrNoMatch si no aplican).
type fakeRepo struct {
	rows       []domain.Incident
	next       int
	createErr  error
	countErr   error
	getErr     error
	noteErr    error
	closeErr   error
	hideOnNext bool // simula que otra operación cerró la incidencia entre lectura y escritura

	lastStatus domain.Status
	lastAfter  *domain.Cursor
	lastLimit  int
	listed     string
}

func (f *fakeRepo) NewID() string { f.next++; return fmt.Sprintf("%024x", f.next) }

func (f *fakeRepo) Create(_ context.Context, i domain.Incident) (domain.Incident, error) {
	if f.createErr != nil {
		return domain.Incident{}, f.createErr
	}
	f.rows = append(f.rows, i)
	return i, nil
}

func (f *fakeRepo) idx(id string) int {
	for n, r := range f.rows {
		if r.ID == id {
			return n
		}
	}
	return -1
}

func (f *fakeRepo) ByID(_ context.Context, id string) (domain.Incident, error) {
	if f.getErr != nil {
		return domain.Incident{}, f.getErr
	}
	if n := f.idx(id); n >= 0 {
		return f.rows[n], nil
	}
	return domain.Incident{}, domain.ErrNotFound
}

func (f *fakeRepo) Exists(_ context.Context, id string) (bool, error) { return f.idx(id) >= 0, nil }

func (f *fakeRepo) CountOpen(_ context.Context, bookingID string) (int, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	n := 0
	for _, r := range f.rows {
		if r.BookingID == bookingID && r.Status == domain.StatusOpen {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) ListByParty(_ context.Context, userID string, status domain.Status, after *domain.Cursor, limit int) ([]domain.Incident, error) {
	f.listed, f.lastStatus, f.lastAfter, f.lastLimit = "party:"+userID, status, after, limit
	var out []domain.Incident
	for i := len(f.rows) - 1; i >= 0; i-- { // más recientes primero
		r := f.rows[i]
		if (r.ReporterID == userID || r.ReportedID == userID) && (status == "" || r.Status == status) {
			out = append(out, r)
		}
	}
	out = skipThrough(out, after)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) ListAll(_ context.Context, status domain.Status, after *domain.Cursor, limit int) ([]domain.Incident, error) {
	f.listed, f.lastStatus, f.lastAfter, f.lastLimit = "all", status, after, limit
	var out []domain.Incident
	for _, r := range f.rows { // más antiguas primero
		if status == "" || r.Status == status {
			out = append(out, r)
		}
	}
	out = skipThrough(out, after)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// skipThrough descarta los elementos hasta el cursor (inclusive), como lo hace
// el filtro por (createdAt, id) del repositorio real.
func skipThrough(rows []domain.Incident, after *domain.Cursor) []domain.Incident {
	if after == nil {
		return rows
	}
	for n, r := range rows {
		if r.ID == after.ID {
			return rows[n+1:]
		}
	}
	return rows
}

func (f *fakeRepo) AddNote(_ context.Context, id string, n domain.Note) (domain.Incident, error) {
	if f.noteErr != nil {
		return domain.Incident{}, f.noteErr
	}
	i := f.idx(id)
	if i < 0 || f.hideOnNext || f.rows[i].Status != domain.StatusOpen || len(f.rows[i].Notes) >= domain.MaxNotes {
		return domain.Incident{}, ErrNoMatch
	}
	f.rows[i].Notes = append(f.rows[i].Notes, n)
	f.rows[i].Version++
	return f.rows[i], nil
}

func (f *fakeRepo) Close(_ context.Context, id string, fr domain.FinalResponse) (domain.Incident, error) {
	if f.closeErr != nil {
		return domain.Incident{}, f.closeErr
	}
	i := f.idx(id)
	if i < 0 || f.hideOnNext || f.rows[i].Status != domain.StatusOpen {
		return domain.Incident{}, ErrNoMatch
	}
	closed, _ := domain.Close(f.rows[i], fr)
	closed.Version++
	f.rows[i] = closed
	return closed, nil
}

type fakeImages struct {
	resolveErr error
	urlErr     error
	gotUser    string
	gotPurpose images.Purpose
	gotField   string
	resolved   int
}

func (f *fakeImages) Signature(context.Context, string, images.Purpose) (images.UploadSignature, error) {
	return images.UploadSignature{}, nil
}
func (f *fakeImages) Resolve(_ context.Context, userID string, p images.Purpose, field string, refs []images.Ref) ([]images.Image, error) {
	f.resolved++
	f.gotUser, f.gotPurpose, f.gotField = userID, p, field
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	out := make([]images.Image, 0, len(refs))
	for _, r := range refs {
		out = append(out, images.Image{PublicID: r.PublicID, Format: "jpg", Delivery: images.DeliveryAuthenticated})
	}
	return out, nil
}
func (f *fakeImages) URLs(i images.Image) (images.URLs, error) {
	if f.urlErr != nil {
		return images.URLs{}, f.urlErr
	}
	return images.URLs{URL: "https://img/" + i.PublicID + "?sig=10min", ThumbURL: "https://thumb/" + i.PublicID}, nil
}

type fakeUsers struct {
	profiles map[string]users.PublicUser
	err      error
}

func (f *fakeUsers) PublicUsers(_ context.Context, ids []string) (map[string]users.PublicUser, error) {
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

type sentEvent struct {
	to string
	e  notify.Event
}

type fakeNotifier struct {
	sent []sentEvent
	err  error
}

func (f *fakeNotifier) Notify(_ context.Context, to string, e notify.Event) error {
	f.sent = append(f.sent, sentEvent{to, e})
	return f.err
}

func (f *fakeNotifier) to() string {
	var out []string
	for _, s := range f.sent {
		out = append(out, s.e.Type+":"+s.to)
	}
	return strings.Join(out, ",")
}

type env struct {
	svc   *Service
	repo  *fakeRepo
	imgs  *fakeImages
	users *fakeUsers
	not   *fakeNotifier
	logs  *bytes.Buffer
	clock *time.Time
}

func booking(status bookings.Status) bookings.Booking {
	b := bookings.Booking{
		ID: bid, OwnerID: ownerID, RequesterID: requesterID,
		Start: bookings.MustDate("2026-11-18"), End: bookings.MustDate("2026-11-20"), Status: status,
	}
	if status == bookings.StatusCompleted {
		b.History = []bookings.HistoryEntry{{Status: bookings.StatusCompleted, At: completedAt, By: bookings.RoleSystem}}
	}
	return b
}

func newEnv(t *testing.T, b bookings.Booking) *env {
	t.Helper()
	now := midBooking
	e := &env{
		repo: &fakeRepo{}, imgs: &fakeImages{}, not: &fakeNotifier{}, logs: &bytes.Buffer{}, clock: &now,
		users: &fakeUsers{profiles: map[string]users.PublicUser{
			ownerID:     {ID: ownerID, DisplayName: "Beto"},
			requesterID: {ID: requesterID, DisplayName: "Ana"},
		}},
	}
	svc, err := NewService(Deps{
		Repo: e.repo, Bookings: fakeBookings{b: b}, Images: e.imgs, Users: e.users, Notifier: e.not,
		Log: slog.New(slog.NewTextHandler(e.logs, nil)), Now: func() time.Time { return *e.clock }, Location: loc,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

func okInput() OpenInput {
	return OpenInput{Reason: domain.ReasonDamaged, Description: "El objeto regresó con la tapa rota."}
}

func TestNewServiceRequiresDeps(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	e := newEnv(t, booking(bookings.StatusAccepted))
	svc, err := NewService(Deps{Repo: e.repo, Bookings: fakeBookings{}, Images: e.imgs, Users: e.users, Notifier: e.not, Log: slog.Default()})
	if err != nil || svc.d.Now == nil || svc.d.Location == nil {
		t.Errorf("NewService = %v, %v", svc, err)
	}
}

// --- Open ---

func TestOpenCreatesIncidentAndNotifiesReported(t *testing.T) { // CA-04
	b := booking(bookings.StatusInProgress)
	e := newEnv(t, b)
	in := okInput()
	in.Evidence = []images.Ref{{PublicID: "vaiven/test/evidence/u/1"}, {PublicID: "vaiven/test/evidence/u/2"}}
	v, err := e.svc.Open(context.Background(), requesterID, bid, in)
	if err != nil {
		t.Fatal(err)
	}
	i := v.Incident
	if i.Status != domain.StatusOpen || i.ReporterID != requesterID || i.ReportedID != ownerID || i.BookingID != bid ||
		i.Reason != domain.ReasonDamaged || i.Description != in.Description || len(i.Evidence) != 2 {
		t.Errorf("incidencia = %+v", i)
	}
	if v.Reporter.DisplayName != "Ana" || v.Reported.DisplayName != "Beto" || len(v.URLs) != 2 ||
		!strings.Contains(v.URLs[0].URL, "sig=10min") || len(v.Notes) != 0 || v.AdminView {
		t.Errorf("vista = %+v", v)
	}
	if e.imgs.gotUser != requesterID || e.imgs.gotPurpose != images.PurposeEvidence || e.imgs.gotField != "evidence" {
		t.Errorf("Resolve(user=%q, purpose=%q, field=%q)", e.imgs.gotUser, e.imgs.gotPurpose, e.imgs.gotField)
	}
	if got := e.not.to(); got != EventOpened+":"+ownerID {
		t.Errorf("avisos = %q, solo la parte reportada", got)
	}
	// La reserva no cambia (CA-04, RF-06): el caso de uso solo la lee.
	if b.Status != bookings.StatusInProgress {
		t.Error("abrir una incidencia no debe cambiar la reserva")
	}
}

func TestOpenWithoutEvidenceDoesNotCallImages(t *testing.T) {
	e := newEnv(t, booking(bookings.StatusAccepted))
	v, err := e.svc.Open(context.Background(), ownerID, bid, okInput())
	if err != nil || e.imgs.resolved != 0 || len(v.Incident.Evidence) != 0 || v.Incident.ReportedID != requesterID {
		t.Errorf("v=%+v err=%v resolved=%d", v, err, e.imgs.resolved)
	}
}

func TestOpenRejections(t *testing.T) {
	ctx := context.Background()
	t.Run("ajeno recibe 404 aunque los datos sean inválidos", func(t *testing.T) {
		e := newEnv(t, booking(bookings.StatusAccepted))
		if _, err := e.svc.Open(ctx, strangerID, bid, OpenInput{}); !errors.Is(err, bookings.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("reserva inexistente", func(t *testing.T) {
		e := newEnv(t, booking(bookings.StatusAccepted))
		if _, err := e.svc.Open(ctx, ownerID, "6731a0000000000000000099", okInput()); !errors.Is(err, bookings.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("datos inválidos", func(t *testing.T) {
		e := newEnv(t, booking(bookings.StatusAccepted))
		if _, err := e.svc.Open(ctx, ownerID, bid, OpenInput{Reason: "x", Description: "corta"}); err == nil {
			t.Error("quería validation_failed")
		}
		if len(e.repo.rows) != 0 || e.imgs.resolved != 0 {
			t.Error("no debe guardar ni consultar imágenes")
		}
	})
	t.Run("reserva en estado que no admite reportes", func(t *testing.T) {
		e := newEnv(t, booking(bookings.StatusCancelled))
		if _, err := e.svc.Open(ctx, ownerID, bid, okInput()); !errors.Is(err, domain.ErrBookingStatus) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("completada fuera de las 72 h", func(t *testing.T) {
		e := newEnv(t, booking(bookings.StatusCompleted))
		*e.clock = completedAt.Add(73 * time.Hour)
		if _, err := e.svc.Open(ctx, ownerID, bid, okInput()); !errors.Is(err, domain.ErrWindowClosed) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("completada dentro de las 72 h", func(t *testing.T) {
		e := newEnv(t, booking(bookings.StatusCompleted))
		*e.clock = completedAt.Add(70 * time.Hour)
		if _, err := e.svc.Open(ctx, ownerID, bid, okInput()); err != nil {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("no_show antes de la fecha de inicio", func(t *testing.T) {
		e := newEnv(t, booking(bookings.StatusAccepted))
		*e.clock = time.Date(2026, 11, 17, 12, 0, 0, 0, loc)
		in := okInput()
		in.Reason = domain.ReasonNoShow
		if _, err := e.svc.Open(ctx, ownerID, bid, in); !errors.Is(err, domain.ErrTooEarly) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("errores de la evidencia se propagan", func(t *testing.T) {
		e := newEnv(t, booking(bookings.StatusAccepted))
		e.imgs.resolveErr = images.ErrUnavailable
		in := okInput()
		in.Evidence = []images.Ref{{PublicID: "p"}}
		if _, err := e.svc.Open(ctx, ownerID, bid, in); !errors.Is(err, images.ErrUnavailable) {
			t.Errorf("err = %v", err)
		}
		if len(e.repo.rows) != 0 {
			t.Error("si falla la evidencia no se guarda nada")
		}
	})
	t.Run("errores de infraestructura se propagan", func(t *testing.T) {
		boom := errors.New("mongo caído")
		e := newEnv(t, booking(bookings.StatusAccepted))
		e.repo.countErr = boom
		if _, err := e.svc.Open(ctx, ownerID, bid, okInput()); !errors.Is(err, boom) {
			t.Errorf("contar: %v", err)
		}
		e = newEnv(t, booking(bookings.StatusAccepted))
		e.repo.createErr = boom
		if _, err := e.svc.Open(ctx, ownerID, bid, okInput()); !errors.Is(err, boom) || e.not.to() != "" {
			t.Errorf("guardar: %v (avisos %q)", err, e.not.to())
		}
		e = newEnv(t, booking(bookings.StatusAccepted))
		e.svc.d.Bookings = fakeBookings{err: boom}
		if _, err := e.svc.Open(ctx, ownerID, bid, okInput()); !errors.Is(err, boom) {
			t.Errorf("reserva: %v", err)
		}
	})
}

func TestOpenFourthIncidentFails(t *testing.T) { // CA-08
	e := newEnv(t, booking(bookings.StatusAccepted))
	for n := 1; n <= 3; n++ {
		if _, err := e.svc.Open(context.Background(), ownerID, bid, okInput()); err != nil {
			t.Fatalf("incidencia %d: %v", n, err)
		}
	}
	if _, err := e.svc.Open(context.Background(), ownerID, bid, okInput()); !errors.Is(err, domain.ErrTooManyOpen) {
		t.Errorf("la 4.ª debe fallar con ErrTooManyOpen: %v", err)
	}
	// Cerrar una libera un lugar.
	if _, err := e.svc.Close(context.Background(), adminID, e.repo.rows[0].ID, "Resuelto con ambas partes.", domain.OutcomeNone); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Open(context.Background(), ownerID, bid, okInput()); err != nil {
		t.Errorf("tras cerrar una cabe otra: %v", err)
	}
}

func TestOpenNotifierFailureDoesNotUndo(t *testing.T) {
	e := newEnv(t, booking(bookings.StatusAccepted))
	e.not.err = errors.New("push caído")
	if _, err := e.svc.Open(context.Background(), ownerID, bid, okInput()); err != nil {
		t.Fatalf("un fallo al avisar no debe deshacer la apertura: %v", err)
	}
	if !strings.Contains(e.logs.String(), "no se pudo notificar") || len(e.repo.rows) != 1 {
		t.Errorf("logs = %q, guardadas = %d", e.logs.String(), len(e.repo.rows))
	}
}

// --- Get ---

func seedIncident(t *testing.T, e *env) domain.Incident {
	t.Helper()
	v, err := e.svc.Open(context.Background(), requesterID, bid, OpenInput{
		Reason: domain.ReasonDamaged, Description: "El objeto regresó con la tapa rota.",
		Evidence: []images.Ref{{PublicID: "ev/1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return v.Incident
}

func TestGetVisibility(t *testing.T) { // RF-07, CA-07
	e := newEnv(t, booking(bookings.StatusAccepted))
	inc := seedIncident(t, e)
	ctx := context.Background()
	if _, err := e.svc.AddNote(ctx, adminID, inc.ID, "nota interna del equipo", false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddNote(ctx, adminID, inc.ID, "nota para las partes", true); err != nil {
		t.Fatal(err)
	}

	for _, party := range []string{requesterID, ownerID} {
		v, err := e.svc.Get(ctx, Viewer{ID: party}, inc.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Notes) != 1 || v.Notes[0].Text != "nota para las partes" || v.AdminView || len(v.URLs) != 1 {
			t.Errorf("%s ve %+v", party, v)
		}
	}
	v, err := e.svc.Get(ctx, Viewer{ID: adminID, Admin: true}, inc.ID)
	if err != nil || len(v.Notes) != 2 || !v.AdminView {
		t.Errorf("el admin ve todas las notas: %+v (%v)", v, err)
	}
	if _, err := e.svc.Get(ctx, Viewer{ID: strangerID}, inc.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ajeno: %v", err)
	}
	if _, err := e.svc.Get(ctx, Viewer{ID: strangerID}, "6731a0000000000000000099"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	// Un admin que además es parte la ve como cualquier parte (sin notas internas).
	e.repo.rows[0].ReportedID = adminID
	pv, err := e.svc.Get(ctx, Viewer{ID: adminID, Admin: true}, inc.ID)
	if err != nil || pv.AdminView || len(pv.Notes) != 1 {
		t.Errorf("admin que es parte: %+v (%v)", pv, err)
	}
}

func TestGetClosedShowsFinalResponseToBothParties(t *testing.T) { // CA-07
	e := newEnv(t, booking(bookings.StatusAccepted))
	inc := seedIncident(t, e)
	ctx := context.Background()
	if _, err := e.svc.Close(ctx, adminID, inc.ID, "Revisamos la evidencia y acordamos un ajuste.", domain.OutcomeMonetaryAdjustment); err != nil {
		t.Fatal(err)
	}
	for _, party := range []string{requesterID, ownerID} {
		v, err := e.svc.Get(ctx, Viewer{ID: party}, inc.ID)
		if err != nil {
			t.Fatal(err)
		}
		f := v.Incident.Final
		if v.Incident.Status != domain.StatusClosed || f == nil || f.Text == "" || f.Outcome != domain.OutcomeMonetaryAdjustment ||
			v.Incident.ClosedAt == nil || len(v.URLs) != 1 {
			t.Errorf("%s ve %+v", party, v.Incident)
		}
	}
}

func TestViewsErrors(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, booking(bookings.StatusAccepted))
	inc := seedIncident(t, e)
	e.imgs.urlErr = errors.New("firma fallida")
	if _, err := e.svc.Get(ctx, Viewer{ID: ownerID}, inc.ID); err == nil {
		t.Error("si falla la URL de la evidencia debe fallar")
	}
	e.imgs.urlErr = nil
	e.users.err = errors.New("users caído")
	if _, err := e.svc.Get(ctx, Viewer{ID: ownerID}, inc.ID); err == nil {
		t.Error("si fallan los perfiles debe fallar")
	}
	e.users.err = nil
	delete(e.users.profiles, requesterID)
	v, err := e.svc.Get(ctx, Viewer{ID: ownerID}, inc.ID)
	if err != nil || v.Reporter.DisplayName != DeletedName || v.Reporter.ID != requesterID {
		t.Errorf("sin perfil: %+v (%v)", v.Reporter, err)
	}
	e.repo.getErr = errors.New("mongo caído")
	if _, err := e.svc.Get(ctx, Viewer{ID: ownerID}, inc.ID); err == nil {
		t.Error("error del repositorio")
	}
}

// --- ListMine ---

func TestListMinePagesBothRoles(t *testing.T) { // RF-07
	e := newEnv(t, booking(bookings.StatusAccepted))
	ctx := context.Background()
	var ids []string
	for n := 0; n < 3; n++ {
		e.repo.rows = append(e.repo.rows, domain.Incident{
			ID: e.repo.NewID(), BookingID: fmt.Sprintf("b%d", n), ReporterID: requesterID, ReportedID: ownerID,
			Reason: domain.ReasonOther, Status: domain.StatusOpen, CreatedAt: completedAt.Add(time.Duration(n) * time.Minute),
		})
		ids = append(ids, e.repo.rows[n].ID)
	}
	e.repo.rows[0].Status = domain.StatusClosed

	p1, err := e.svc.ListMine(ctx, ownerID, "", nil, 2)
	if err != nil || len(p1.Items) != 2 || p1.Next == nil || p1.Items[0].Incident.ID != ids[2] {
		t.Fatalf("p1 = %+v (%v)", p1, err)
	}
	if p1.Next.ID != ids[1] || !p1.Next.CreatedAt.Equal(e.repo.rows[1].CreatedAt) {
		t.Errorf("cursor = %+v", p1.Next)
	}
	if e.repo.lastLimit != 3 {
		t.Errorf("se pide uno más: limit=%d", e.repo.lastLimit)
	}
	p2, err := e.svc.ListMine(ctx, ownerID, "", p1.Next, 2)
	if err != nil || len(p2.Items) != 1 || p2.Next != nil || e.repo.lastAfter == nil {
		t.Errorf("p2 = %+v (%v)", p2, err)
	}
	open, err := e.svc.ListMine(ctx, requesterID, domain.StatusOpen, nil, 10)
	if err != nil || len(open.Items) != 2 || e.repo.lastStatus != domain.StatusOpen {
		t.Errorf("abiertas = %+v (%v)", open, err)
	}
	for _, it := range open.Items {
		if it.AdminView {
			t.Error("/me/incidents siempre es la vista de las partes")
		}
	}
	none, err := e.svc.ListMine(ctx, strangerID, "", nil, 10)
	if err != nil || none.Items == nil || len(none.Items) != 0 || none.Next != nil {
		t.Errorf("sin incidencias = %+v (%v)", none, err)
	}
	if _, err := e.svc.ListMine(ctx, ownerID, "", nil, 0); err != nil || e.repo.lastLimit != DefaultPage+1 {
		t.Errorf("limit por omisión: %d", e.repo.lastLimit)
	}
	if _, err := e.svc.ListMine(ctx, ownerID, "", nil, 999); err != nil || e.repo.lastLimit != MaxPage+1 {
		t.Errorf("limit máximo: %d", e.repo.lastLimit)
	}
}
