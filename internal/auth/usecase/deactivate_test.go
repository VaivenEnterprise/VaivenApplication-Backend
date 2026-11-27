package usecase

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// Spec 021, T011: baja atómica con transacción, erasers, bandeja de salida y
// reanudación.

// fakeTx simula la transacción: si fn falla, restaura el estado de las cuentas y
// las sesiones; con retries repite fn (TransientTransactionError).
type fakeTx struct {
	users   *fakeUsers
	refresh *fakeRefresh
	retries int
	calls   int
	inside  bool
}

func (f *fakeTx) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	f.users.mu.Lock()
	snapU := map[string]domain.User{}
	for k, v := range f.users.byID {
		snapU[k] = v
	}
	snapP := map[string]bool{}
	for k, v := range f.users.pending {
		snapP[k] = v
	}
	f.users.mu.Unlock()
	f.refresh.mu.Lock()
	snapR := map[string]domain.RefreshToken{}
	for k, v := range f.refresh.byHash {
		snapR[k] = v
	}
	f.refresh.mu.Unlock()

	rollback := func() {
		f.users.mu.Lock()
		f.users.byID, f.users.pending = maps.Clone(snapU), maps.Clone(snapP)
		f.users.mu.Unlock()
		f.refresh.mu.Lock()
		f.refresh.byHash = maps.Clone(snapR)
		f.refresh.mu.Unlock()
	}
	for i := 0; ; i++ {
		f.calls++
		f.inside = true
		err := fn(ctx)
		f.inside = false
		if err != nil {
			rollback()
			return err
		}
		if i < f.retries {
			rollback() // el driver repite la función tras un error transitorio
			continue
		}
		return nil
	}
}

// fakeEraser anota las llamadas y puede fallar o dejar resultados.
type fakeEraser struct {
	name  string
	res   EraseResult
	err   error
	calls *[]string
	tx    *fakeTx
}

func (e *fakeEraser) Name() string { return e.name }
func (e *fakeEraser) Erase(_ context.Context, userID string, _ time.Time) (EraseResult, error) {
	if e.tx != nil && !e.tx.inside {
		return EraseResult{}, errors.New("Erase llamado fuera de la transacción")
	}
	*e.calls = append(*e.calls, e.name+":"+userID)
	return e.res, e.err
}

type fakeJobs struct {
	enqueued []Job
	batches  int
	open     int64
	failEnq  error
	failOpen error
}

func (f *fakeJobs) Enqueue(_ context.Context, jobs []Job, _ time.Time) error {
	if f.failEnq != nil {
		return f.failEnq
	}
	f.batches++
	f.enqueued = append(f.enqueued, jobs...)
	return nil
}
func (f *fakeJobs) Claim(context.Context, time.Time, int) ([]Job, error) { return nil, nil }
func (f *fakeJobs) Done(context.Context, string) error                   { return nil }
func (f *fakeJobs) Fail(context.Context, string, JobFailure) error       { return nil }
func (f *fakeJobs) Open(context.Context, string) (int64, error)          { return f.open, f.failOpen }

// dynBookings devuelve false las primeras `after` consultas y true después:
// simula una reserva que se acepta entre la primera comprobación y la
// transacción.
type dynBookings struct {
	calls int
	after int
}

func (b *dynBookings) HasActiveBookings(context.Context, string) (bool, error) {
	b.calls++
	return b.calls > b.after, nil
}

type deleteEnv struct {
	*env
	tx     *fakeTx
	jobs   *fakeJobs
	rev    *fakeRevocations
	events *recordedEvents
	calls  *[]string
}

func newDeleteEnv(t *testing.T, mutate ...func(*Deps)) *deleteEnv {
	t.Helper()
	de := &deleteEnv{jobs: &fakeJobs{}, rev: &fakeRevocations{}, events: &recordedEvents{}, calls: &[]string{}}
	de.env = newEnv(t, append([]func(*Deps){func(d *Deps) {
		d.Revocations, d.Events = de.rev, de.events
		d.Jobs = de.jobs
		d.Pending = d.Users.(*fakeUsers)
	}}, mutate...)...)
	de.tx = &fakeTx{users: de.users, refresh: de.refresh}
	de.svc.d.Tx = de.tx
	return de
}

func (de *deleteEnv) eraser(name string, res EraseResult, err error) *fakeEraser {
	e := &fakeEraser{name: name, res: res, err: err, calls: de.calls, tx: de.tx}
	de.svc.d.Erasers = append(de.svc.d.Erasers, e)
	return e
}

func TestDeleteAccountHappyPath(t *testing.T) {
	ctx := context.Background()
	de := newDeleteEnv(t)
	reg := de.mustRegister(t)
	id := reg.User.ID
	later := de.now.Add(24 * time.Hour)
	de.eraser("bookings", EraseResult{Notices: []Notice{{UserID: "otra", Type: "booking.cancelled", Title: "t", Body: "b"}}}, nil)
	de.eraser("items", EraseResult{Images: []ImageRef{{PublicID: "p1", Delivery: "upload"}, {PublicID: "p2", NotBefore: later}}}, nil)
	de.eraser("chat", EraseResult{}, nil)

	if err := de.svc.DeleteAccount(ctx, id, "una-contrasena-larga"); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	want := []string{"bookings:" + id, "items:" + id, "chat:" + id}
	if !reflect.DeepEqual(*de.calls, want) {
		t.Errorf("erasers = %v, want %v", *de.calls, want)
	}
	u := de.users.byID[id]
	if u.IsActive() || u.Email != domain.AnonymizedEmail(id) || u.DisplayName != domain.DeletedDisplayName || u.PasswordHash != "" {
		t.Errorf("cuenta = %+v", u)
	}
	if len(de.refresh.byHash) != 0 {
		t.Error("las sesiones deben borrarse")
	}
	if len(de.jobs.enqueued) != 3 || de.jobs.batches != 1 {
		t.Fatalf("jobs = %+v en %d lotes", de.jobs.enqueued, de.jobs.batches)
	}
	if j := de.jobs.enqueued[0]; j.Kind != domain.JobNotice || j.Notice == nil || j.Notice.UserID != "otra" || !j.NextAttemptAt.Equal(de.svc.now()) {
		t.Errorf("job aviso = %+v", j)
	}
	if j := de.jobs.enqueued[2]; j.Kind != domain.JobImage || !j.NextAttemptAt.Equal(later) || !j.Retention {
		t.Errorf("job imagen con fecha (retención) = %+v", j)
	}
	if j := de.jobs.enqueued[1]; j.Retention || j.Kind != domain.JobImage {
		t.Errorf("la foto inmediata no es de retención: %+v", j)
	}
	if len(de.rev.users) != 1 || de.rev.users[0] != id {
		t.Errorf("revocación = %v", de.rev.users)
	}
	if de.users.pending[id] {
		t.Error("sin pasos pendientes la marca deletionPending debe quitarse")
	}
	if !strings.Contains(de.events.names(), "account.deleted") {
		t.Errorf("eventos = %s", de.events.names())
	}
}

func TestDeleteAccountKeepsMarkWhileJobsOpenOrRedisFails(t *testing.T) {
	ctx := context.Background()
	de := newDeleteEnv(t)
	reg := de.mustRegister(t)
	de.jobs.open = 2
	if err := de.svc.DeleteAccount(ctx, reg.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatal(err)
	}
	if !de.users.pending[reg.User.ID] {
		t.Error("con jobs abiertos la marca se conserva")
	}

	de2 := newDeleteEnv(t)
	reg2 := de2.mustRegister(t)
	de2.rev.fail = errBoom
	if err := de2.svc.DeleteAccount(ctx, reg2.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatalf("Redis caído no revierte la baja: %v", err)
	}
	if de2.users.byID[reg2.User.ID].IsActive() || !de2.users.pending[reg2.User.ID] {
		t.Error("cuenta dada de baja y marcada")
	}
	if !strings.Contains(de2.events.names(), "account.deletion_revoke") {
		t.Errorf("eventos = %s", de2.events.names())
	}

	// Fallos al consultar jobs o limpiar la marca: se registran, no fallan.
	de3 := newDeleteEnv(t)
	reg3 := de3.mustRegister(t)
	de3.jobs.failOpen = errBoom
	if err := de3.svc.DeleteAccount(ctx, reg3.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatal(err)
	}
	if !de3.users.pending[reg3.User.ID] {
		t.Error("si no se pueden contar los jobs la marca se conserva")
	}
	de4 := newDeleteEnv(t)
	reg4 := de4.mustRegister(t)
	de4.users.failPending = errBoom
	if err := de4.svc.DeleteAccount(ctx, reg4.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(de4.events.names(), "account.deletion_resume") {
		t.Errorf("eventos = %s", de4.events.names())
	}
}

func TestDeleteAccountWrongPasswordChangesNothing(t *testing.T) { // CA-03
	de := newDeleteEnv(t)
	reg := de.mustRegister(t)
	de.eraser("items", EraseResult{}, nil)
	err := de.svc.DeleteAccount(context.Background(), reg.User.ID, "otra-contrasena")
	if !domain.Is(err, domain.ErrWrongPassword) {
		t.Fatalf("err = %v", err)
	}
	if de.tx.calls != 0 || len(*de.calls) != 0 || !de.users.byID[reg.User.ID].IsActive() {
		t.Error("403: no se abre la transacción ni cambia nada")
	}
}

func TestDeleteAccountActiveBookingsFirstCheck(t *testing.T) { // CA-03
	de := newDeleteEnv(t, func(d *Deps) { d.Bookings = fakeBookings{active: true} })
	reg := de.mustRegister(t)
	de.eraser("items", EraseResult{}, nil)
	if err := de.svc.DeleteAccount(context.Background(), reg.User.ID, "una-contrasena-larga"); !domain.Is(err, domain.ErrActiveBookings) {
		t.Fatalf("err = %v", err)
	}
	if de.tx.calls != 0 || len(*de.calls) != 0 || len(de.rev.users) != 0 {
		t.Error("409 sin abrir transacción ni tocar Redis")
	}
}

func TestDeleteAccountActiveBookingsRecheckedInsideTx(t *testing.T) { // CA-04
	bk := &dynBookings{after: 1} // la 1.ª consulta (fuera) no ve nada; la 2.ª (dentro) sí
	de := newDeleteEnv(t, func(d *Deps) { d.Bookings = bk })
	reg := de.mustRegister(t)
	e := de.eraser("items", EraseResult{}, nil)
	err := de.svc.DeleteAccount(context.Background(), reg.User.ID, "una-contrasena-larga")
	if !domain.Is(err, domain.ErrActiveBookings) {
		t.Fatalf("err = %v, want ErrActiveBookings", err)
	}
	if len(*e.calls) != 0 {
		t.Error("ningún eraser corre si la comprobación dentro de la transacción rechaza")
	}
	u := de.users.byID[reg.User.ID]
	if !u.IsActive() || u.PasswordHash == "" || len(de.refresh.byHash) == 0 || len(de.rev.users) != 0 || de.users.pending[reg.User.ID] {
		t.Errorf("409 dentro de la transacción no cambia nada: %+v", u)
	}
	if u.Version != reg.User.Version {
		t.Errorf("el cerrojo se revierte con la transacción: version %d, want %d", u.Version, reg.User.Version)
	}
}

func TestDeleteAccountEraserErrorAbortsEverything(t *testing.T) { // CA-05
	de := newDeleteEnv(t)
	reg := de.mustRegister(t)
	de.eraser("bookings", EraseResult{}, nil)
	de.eraser("chat", EraseResult{}, errBoom)
	de.eraser("items", EraseResult{}, nil)
	err := de.svc.DeleteAccount(context.Background(), reg.User.ID, "una-contrasena-larga")
	if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), "chat") {
		t.Fatalf("err = %v", err)
	}
	if len(*de.calls) != 2 {
		t.Errorf("tras el fallo no corren los siguientes erasers: %v", *de.calls)
	}
	u := de.users.byID[reg.User.ID]
	if !u.IsActive() || u.PasswordHash == "" || len(de.refresh.byHash) != 1 || len(de.jobs.enqueued) != 0 || len(de.rev.users) != 0 {
		t.Errorf("la cuenta debe quedar completa: %+v", u)
	}
}

func TestDeleteAccountStepFailuresAbort(t *testing.T) {
	cases := map[string]func(de *deleteEnv){
		"cerrojo":         func(de *deleteEnv) { de.users.failLock = errBoom },
		"sesiones":        func(de *deleteEnv) { de.refresh.failRevoke = errBoom },
		"bandeja":         func(de *deleteEnv) { de.jobs.failEnq = errBoom },
		"cuenta":          func(de *deleteEnv) { de.svc.d.Users = failingDeactivate{de.users} },
		"reservas":        func(de *deleteEnv) { de.svc.d.Bookings = fakeBookings{err: errBoom} },
		"lectura inicial": func(de *deleteEnv) { de.users.failAll = errBoom },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			de := newDeleteEnv(t)
			reg := de.mustRegister(t)
			de.eraser("items", EraseResult{Images: []ImageRef{{PublicID: "p"}}}, nil)
			setup(de)
			if err := de.svc.DeleteAccount(context.Background(), reg.User.ID, "una-contrasena-larga"); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
			if de.users.failAll == nil && !de.users.byID[reg.User.ID].IsActive() {
				t.Error("la cuenta debe seguir activa")
			}
			if len(de.rev.users) != 0 {
				t.Error("no se toca Redis si la transacción falla")
			}
		})
	}
}

func TestDeleteAccountRetriedCallbackHasNoDuplicateEffects(t *testing.T) { // callback re-ejecutable
	de := newDeleteEnv(t)
	reg := de.mustRegister(t)
	de.tx.retries = 2
	de.eraser("items", EraseResult{Images: []ImageRef{{PublicID: "p"}}}, nil)
	if err := de.svc.DeleteAccount(context.Background(), reg.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatal(err)
	}
	if de.tx.calls != 3 {
		t.Errorf("llamadas = %d, want 3", de.tx.calls)
	}
	// Cada intento arma sus jobs de cero: ninguno arrastra los del anterior.
	for _, j := range de.jobs.enqueued {
		if j.Image == nil || j.Image.PublicID != "p" {
			t.Errorf("job inesperado %+v", j)
		}
	}
	if de.jobs.batches != 3 || len(de.jobs.enqueued) != 3 {
		t.Errorf("cada intento encola exactamente sus jobs: %d lotes, %d jobs", de.jobs.batches, len(de.jobs.enqueued))
	}
	if de.users.byID[reg.User.ID].IsActive() {
		t.Error("cuenta sin dar de baja")
	}
}

func TestDeleteAccountTwiceIsIdempotent(t *testing.T) { // RF-05, CA-12
	ctx := context.Background()
	de := newDeleteEnv(t)
	reg := de.mustRegister(t)
	de.eraser("items", EraseResult{Notices: []Notice{{UserID: "x"}}}, nil)
	if err := de.svc.DeleteAccount(ctx, reg.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatal(err)
	}
	snap := de.users.byID[reg.User.ID]
	jobs := len(de.jobs.enqueued)
	calls := len(*de.calls)
	if err := de.svc.DeleteAccount(ctx, reg.User.ID, "otra"); err != nil {
		t.Fatalf("segunda baja = %v", err)
	}
	if !reflect.DeepEqual(snap, de.users.byID[reg.User.ID]) || len(de.jobs.enqueued) != jobs || len(*de.calls) != calls {
		t.Error("la segunda baja no repite efectos")
	}
	if len(de.rev.users) < 2 {
		t.Error("la segunda baja reanuda la revocación")
	}
	if err := de.svc.DeleteAccount(ctx, "inexistente", "x"); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("cuenta inexistente = %v", err)
	}
}

func TestDeleteAccountLostRaceIsSuccess(t *testing.T) { // dos bajas simultáneas
	de := newDeleteEnv(t)
	reg := de.mustRegister(t)
	e := de.eraser("items", EraseResult{}, nil)
	de.svc.d.Users = lockLoses{de.users}
	if err := de.svc.DeleteAccount(context.Background(), reg.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(*e.calls) != 0 {
		t.Error("sin cerrojo no corre ningún eraser")
	}
}

// lockLoses simula que LockActive pierde la carrera: la cuenta se ve activa al
// leerla pero otra transacción ya la dio de baja.
type lockLoses struct{ *fakeUsers }

func (lockLoses) LockActive(context.Context, string) (bool, error) { return false, nil }

func TestDeleteAccountWithoutOptionalPorts(t *testing.T) {
	e := newEnv(t) // sin Tx, Erasers, Jobs ni Pending: la baja funciona igual
	reg := e.mustRegister(t)
	if err := e.svc.DeleteAccount(context.Background(), reg.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatal(err)
	}
	if e.users.byID[reg.User.ID].IsActive() {
		t.Error("cuenta sin dar de baja")
	}
}

func TestDeleteAccountEventsNeverLeakPersonalData(t *testing.T) { // RNF-03
	de := newDeleteEnv(t)
	reg := de.mustRegister(t)
	de.rev.fail = errBoom
	if err := de.svc.DeleteAccount(context.Background(), reg.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatal(err)
	}
	if len(*de.events) == 0 {
		t.Fatal("se esperaban eventos")
	}
	for _, ev := range *de.events {
		all := fmt.Sprintf("%+v", ev)
		if strings.Contains(all, "ana@example.com") || strings.Contains(all, "una-contrasena-larga") || strings.Contains(strings.ToLower(all), "boom") {
			t.Errorf("evento con datos sensibles: %s", all)
		}
	}
}

func TestResumeOnVanishedAccount(t *testing.T) {
	de := newDeleteEnv(t)
	de.svc.resumer.ResumeAccount(context.Background(), "no-existe")
	if !strings.Contains(de.events.names(), "account.deletion_resume") {
		t.Errorf("eventos = %s", de.events.names())
	}
}
