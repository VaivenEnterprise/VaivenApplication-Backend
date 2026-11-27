package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// Spec 021, T021: reanudación de bajas (jobs de la bandeja y marca
// deletionPending).

// queueJobs es una bandeja en memoria con la semántica del adaptador: Claim
// devuelve los vencidos sin arrendamiento y suma un intento.
type queueJobs struct {
	jobs     map[string]*Job
	leased   map[string]bool
	failed   map[string]bool
	failures []JobFailure
	next     int
	failDone error
	failClm  error
	failOpen error
}

func newQueue() *queueJobs {
	return &queueJobs{jobs: map[string]*Job{}, leased: map[string]bool{}, failed: map[string]bool{}}
}

func (q *queueJobs) add(user string, kind domain.JobKind, at time.Time) string {
	q.next++
	id := fmt.Sprintf("j%04d", q.next)
	j := &Job{ID: id, UserID: user, Kind: kind, NextAttemptAt: at}
	switch kind {
	case domain.JobImage:
		j.Image = &ImageRef{PublicID: "p-" + id, Delivery: "upload"}
	case domain.JobNotice:
		j.Notice = &Notice{UserID: "otra", Type: "booking.cancelled", Title: "t", Body: "b"}
	}
	q.jobs[id] = j
	return id
}

func (q *queueJobs) Enqueue(_ context.Context, jobs []Job, _ time.Time) error {
	for _, j := range jobs {
		j := j
		q.next++
		j.ID = fmt.Sprintf("j%04d", q.next)
		q.jobs[j.ID] = &j
	}
	return nil
}

func (q *queueJobs) Claim(_ context.Context, now time.Time, limit int) ([]Job, error) {
	if q.failClm != nil {
		return nil, q.failClm
	}
	var ids []string
	for id, j := range q.jobs {
		if !q.leased[id] && !q.failed[id] && !j.NextAttemptAt.After(now) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var out []Job
	for _, id := range ids {
		if len(out) == limit {
			break
		}
		q.leased[id] = true
		q.jobs[id].Attempts++
		out = append(out, *q.jobs[id])
	}
	return out, nil
}

func (q *queueJobs) Done(_ context.Context, id string) error {
	if q.failDone != nil {
		return q.failDone
	}
	delete(q.jobs, id)
	delete(q.leased, id)
	return nil
}

func (q *queueJobs) Fail(_ context.Context, id string, f JobFailure) error {
	q.failures = append(q.failures, f)
	delete(q.leased, id)
	if f.Exhausted {
		q.failed[id] = true
		return nil
	}
	q.jobs[id].NextAttemptAt = f.Next
	return nil
}

func (q *queueJobs) Open(_ context.Context, userID string) (int64, error) {
	if q.failOpen != nil {
		return 0, q.failOpen
	}
	var n int64
	for id, j := range q.jobs {
		if j.UserID == userID && !q.failed[id] {
			n++
		}
	}
	return n, nil
}

type fakeImages struct {
	destroyed []string
	err       error
}

func (f *fakeImages) Destroy(_ context.Context, r ImageRef) error {
	if f.err != nil {
		return f.err
	}
	f.destroyed = append(f.destroyed, r.PublicID)
	return nil
}

type fakeNotices struct {
	sent []Notice
	err  error
}

func (f *fakeNotices) Send(_ context.Context, n Notice) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, n)
	return nil
}

type resumeEnv struct {
	r       *Resumer
	users   *fakeUsers
	q       *queueJobs
	imgs    *fakeImages
	notices *fakeNotices
	rev     *fakeRevocations
	events  *recordedEvents
	now     time.Time
	uid     string
}

// newResumeEnv deja una cuenta ya dada de baja y marcada con deletionPending.
func newResumeEnv(t *testing.T) *resumeEnv {
	t.Helper()
	e := &resumeEnv{
		users: newFakeUsers(), q: newQueue(), imgs: &fakeImages{}, notices: &fakeNotices{},
		rev: &fakeRevocations{}, events: &recordedEvents{}, now: time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC),
	}
	u, err := e.users.Create(context.Background(), domain.User{Email: "ana@example.com", Status: domain.StatusActive, DisplayName: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.users.Deactivate(context.Background(), u.ID, domain.AnonymizedEmail(u.ID), e.now); err != nil {
		t.Fatal(err)
	}
	e.uid = u.ID
	r, err := NewResumer(ResumerDeps{
		Users: e.users, Pending: e.users, Jobs: e.q, Revocations: e.rev, AccessTTL: 15 * time.Minute,
		Images: e.imgs, Notices: e.notices, Events: e.events, Now: func() time.Time { return e.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	e.r = r
	return e
}

func TestNewResumerNeedsUsers(t *testing.T) {
	if _, err := NewResumer(ResumerDeps{}); err == nil {
		t.Error("sin Users debe fallar")
	}
	if r, err := NewResumer(ResumerDeps{Users: newFakeUsers()}); err != nil || r.d.Now == nil {
		t.Errorf("NewResumer mínimo = %v, %v", r, err)
	}
}

func TestResumeDeletionsPassRunsJobsRevokesAndClearsTheMark(t *testing.T) { // CA-06, CA-12
	ctx := context.Background()
	e := newResumeEnv(t)
	e.q.add(e.uid, domain.JobImage, e.now)
	e.q.add(e.uid, domain.JobNotice, e.now)
	e.q.add(e.uid, domain.JobImage, e.now.Add(time.Hour)) // aún no vence (evidencia con fecha)

	rep, err := e.r.ResumeDeletions(ctx)
	if err != nil || rep.JobsDone != 2 || rep.Accounts != 1 || rep.Cleared != 0 {
		t.Fatalf("pasada 1 = %+v, %v", rep, err)
	}
	if len(e.imgs.destroyed) != 1 || len(e.notices.sent) != 1 || len(e.rev.users) != 1 || e.rev.users[0] != e.uid {
		t.Errorf("destruidas=%v avisos=%d revocaciones=%v", e.imgs.destroyed, len(e.notices.sent), e.rev.users)
	}
	if !e.users.pending[e.uid] {
		t.Error("con un job futuro la marca se conserva")
	}

	e.now = e.now.Add(2 * time.Hour)
	rep, err = e.r.ResumeDeletions(ctx)
	if err != nil || rep.JobsDone != 1 || rep.Cleared != 1 {
		t.Fatalf("pasada 2 = %+v, %v", rep, err)
	}
	if e.users.pending[e.uid] || len(e.q.jobs) != 0 {
		t.Error("al terminar se quita la marca y la bandeja queda vacía")
	}

	// Repetir no cambia nada y no falla (idempotente).
	rep, err = e.r.ResumeDeletions(ctx)
	if err != nil || rep != (PassReport{}) {
		t.Errorf("pasada 3 = %+v, %v", rep, err)
	}
	if len(e.imgs.destroyed) != 2 {
		t.Errorf("no debe repetir borrados: %v", e.imgs.destroyed)
	}
}

func TestResumeAfterRedisOutageAndRepeat(t *testing.T) { // CA-06
	ctx := context.Background()
	e := newResumeEnv(t)
	e.rev.fail = errBoom
	rep, err := e.r.ResumeDeletions(ctx)
	if err != nil || rep.Accounts != 1 || rep.Cleared != 0 || !e.users.pending[e.uid] {
		t.Fatalf("con Redis caído = %+v, %v, marca=%v", rep, err, e.users.pending[e.uid])
	}
	if !strings.Contains(e.events.names(), "account.deletion_revoke") {
		t.Errorf("eventos = %s", e.events.names())
	}
	e.rev.fail = nil
	rep, err = e.r.ResumeDeletions(ctx)
	if err != nil || rep.Cleared != 1 || e.users.pending[e.uid] || len(e.rev.users) != 1 {
		t.Fatalf("con Redis de vuelta = %+v, %v", rep, err)
	}
}

func TestFailedJobsRetryWithBackoffThenGiveUp(t *testing.T) { // CA-06
	ctx := context.Background()
	e := newResumeEnv(t)
	e.imgs.err = errors.New("cloudinary 500 para ana@example.com")
	id := e.q.add(e.uid, domain.JobImage, e.now)

	for attempt := 1; attempt <= domain.MaxJobAttempts; attempt++ {
		rep, err := e.r.RunJobs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		last := attempt == domain.MaxJobAttempts
		if (rep.JobsRetried == 1) == last || (rep.JobsFailed == 1) != last {
			t.Fatalf("intento %d: %+v", attempt, rep)
		}
		f := e.q.failures[len(e.q.failures)-1]
		if f.Exhausted != last || f.Reason != "image: falló el paso" || !f.At.Equal(e.now) {
			t.Fatalf("intento %d: fallo = %+v", attempt, f)
		}
		if !last {
			if want := e.now.Add(domain.JobBackoff(attempt)); !f.Next.Equal(want) {
				t.Fatalf("intento %d: Next = %v, want %v", attempt, f.Next, want)
			}
			if again, _ := e.r.RunJobs(ctx); again != (PassReport{}) {
				t.Fatalf("antes de la espera no se reintenta: %+v", again)
			}
			e.now = f.Next // llega la hora del reintento
		}
	}
	if !e.q.failed[id] || e.q.jobs[id].Attempts != domain.MaxJobAttempts {
		t.Errorf("el job agotado queda fallido: %v / %d", e.q.failed[id], e.q.jobs[id].Attempts)
	}
	if !strings.Contains(e.events.names(), "deletion.job_failed") || !strings.Contains(e.events.names(), "deletion.job_retry") {
		t.Errorf("eventos = %s", e.events.names())
	}
	// RNF-03: ni el texto del error ni el correo salen en eventos ni en el motivo.
	for _, ev := range *e.events {
		if all := fmt.Sprintf("%+v", ev); strings.Contains(all, "@") || strings.Contains(all, "cloudinary") || !strings.HasPrefix(ev.Subject, "user:") {
			t.Errorf("evento con datos: %+v", ev)
		}
	}
	// Un fallido ya no cuenta como pendiente: la marca se puede quitar.
	if _, err := e.r.ResumeDeletions(ctx); err != nil || e.users.pending[e.uid] {
		t.Errorf("tras agotar, la cuenta queda sin marca: %v", err)
	}
}

func TestJobsWithoutPortsFailAndRetry(t *testing.T) {
	ctx := context.Background()
	e := newResumeEnv(t)
	e.r.d.Images, e.r.d.Notices = nil, nil
	e.q.add(e.uid, domain.JobImage, e.now)
	e.q.add(e.uid, domain.JobNotice, e.now)
	e.q.add(e.uid, "otro", e.now) // tipo desconocido
	e.q.jobs["j0001"].Image = nil // imagen sin referencia
	rep, err := e.r.RunJobs(ctx)
	if err != nil || rep.JobsRetried != 3 || rep.JobsDone != 0 {
		t.Fatalf("sin puertos = %+v, %v", rep, err)
	}
	e.r.d.Images, e.r.d.Notices = e.imgs, e.notices
	e.q.jobs["j0001"].Image = &ImageRef{PublicID: "p"}
	e.now = e.now.Add(time.Hour)
	rep, _ = e.r.RunJobs(ctx)
	if rep.JobsDone != 2 || rep.JobsRetried != 1 { // el de tipo desconocido nunca se puede ejecutar
		t.Errorf("con puertos = %+v", rep)
	}
}

func TestNoticeFailureIsRetried(t *testing.T) {
	e := newResumeEnv(t)
	e.notices.err = errBoom
	e.q.add(e.uid, domain.JobNotice, e.now)
	rep, err := e.r.RunJobs(context.Background())
	if err != nil || rep.JobsRetried != 1 || len(e.notices.sent) != 0 || e.q.failures[0].Reason != "notice: falló el paso" {
		t.Errorf("rep=%+v err=%v failures=%+v", rep, err, e.q.failures)
	}
}

func TestRunJobsStopsOnCancelWithoutCountingFailures(t *testing.T) {
	e := newResumeEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	e.q.add(e.uid, domain.JobImage, e.now)
	e.q.add(e.uid, domain.JobImage, e.now)
	e.r.d.Images = cancelingImages{cancel: cancel}
	rep, err := e.r.RunJobs(ctx)
	if err != nil || len(e.q.failures) != 0 || rep.JobsRetried != 0 || rep.JobsDone != 0 {
		t.Errorf("cancelado = %+v, %v, fallos=%v", rep, err, e.q.failures)
	}
	// Ya cancelado de entrada: no reclama nada.
	if rep, err := e.r.RunJobs(ctx); err != nil || rep != (PassReport{}) {
		t.Errorf("ctx cancelado = %+v, %v", rep, err)
	}
	if rep, err := e.r.ResumeDeletions(ctx); err != nil || rep != (PassReport{}) {
		t.Errorf("Pass con ctx cancelado = %+v, %v", rep, err)
	}
}

type cancelingImages struct{ cancel func() }

func (c cancelingImages) Destroy(ctx context.Context, _ ImageRef) error {
	c.cancel()
	return ctx.Err()
}

func TestRunJobsErrorsAreReportedWithoutData(t *testing.T) {
	ctx := context.Background()
	e := newResumeEnv(t)
	e.q.failClm = errBoom
	if _, err := e.r.RunJobs(ctx); !errors.Is(err, errBoom) {
		t.Errorf("Claim falla = %v", err)
	}
	if _, err := e.r.ResumeDeletions(ctx); !errors.Is(err, errBoom) {
		t.Errorf("Pass con Claim fallando = %v", err)
	}
	e.q.failClm = nil

	e.q.add(e.uid, domain.JobImage, e.now)
	e.q.failDone = errBoom
	rep, err := e.r.RunJobs(ctx)
	if err != nil || rep.JobsDone != 0 || !strings.Contains(e.events.names(), "deletion.job_done") {
		t.Errorf("Done falla = %+v, %v, %s", rep, err, e.events.names())
	}

	// Pending o Open fallando: se registra y la marca se conserva.
	e2 := newResumeEnv(t)
	e2.users.failPending = errBoom
	if _, err := e2.r.ResumeDeletions(ctx); !errors.Is(err, errBoom) {
		t.Errorf("ListPending falla = %v", err)
	}
	e3 := newResumeEnv(t)
	e3.q.failOpen = errBoom
	if rep, err := e3.r.ResumeDeletions(ctx); err != nil || rep.Cleared != 0 || !e3.users.pending[e3.uid] {
		t.Errorf("Open falla = %+v, %v", rep, err)
	}
}

func TestResumeDeletionsPassCapsJobsPerPass(t *testing.T) {
	e := newResumeEnv(t)
	for i := 0; i < maxJobsPerPass+60; i++ {
		e.q.add(e.uid, domain.JobImage, e.now)
	}
	rep, err := e.r.ResumeDeletions(context.Background())
	if err != nil || rep.JobsDone != maxJobsPerPass {
		t.Fatalf("pasada = %+v, %v, want %d", rep, err, maxJobsPerPass)
	}
	rep, _ = e.r.ResumeDeletions(context.Background())
	if rep.JobsDone != 60 || rep.Cleared != 1 {
		t.Errorf("resto = %+v", rep)
	}
}

func TestResumeDeletionsPassWithoutOptionalPorts(t *testing.T) {
	r, err := NewResumer(ResumerDeps{Users: newFakeUsers()})
	if err != nil {
		t.Fatal(err)
	}
	if rep, err := r.ResumeDeletions(context.Background()); err != nil || rep != (PassReport{}) {
		t.Errorf("sin puertos = %+v, %v", rep, err)
	}
	// Una cuenta activa o inexistente no se reanuda.
	if r.ResumeAccount(context.Background(), "no-existe") {
		t.Error("una cuenta inexistente no se reanuda")
	}
}

func TestServiceResumeDeletionsDelegates(t *testing.T) {
	de := newDeleteEnv(t)
	reg := de.mustRegister(t)
	de.jobs.open = 1 // la baja deja la marca porque hay un job por hacer
	if err := de.svc.DeleteAccount(context.Background(), reg.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatal(err)
	}
	if !de.users.pending[reg.User.ID] {
		t.Fatal("la cuenta debe quedar marcada")
	}
	de.jobs.open = 0
	rep, err := de.svc.ResumeDeletions(context.Background())
	if err != nil || rep.Accounts != 1 || rep.Cleared != 1 || de.users.pending[reg.User.ID] {
		t.Errorf("ResumeDeletions = %+v, %v", rep, err)
	}
}
