package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
)

// inProgress es una reserva en curso del objeto i1 que termina el 22 de
// noviembre; lateAt = 08:00 UTC del 23.
func inProgress(id string) bookings.Booking {
	b := booking(id, bookings.StatusInProgress)
	la := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	b.LateAt = &la
	return b
}

func accepted(id, start, end string) bookings.Booking {
	b := booking(id, bookings.StatusAccepted)
	b.OwnerID, b.RequesterID = "owner", "next-"+id
	b.Start, b.End = bookings.MustDate(start), bookings.MustDate(end)
	return b
}

func TestDetectLateMarksAndNotifies(t *testing.T) {
	e := newEnv(t, inProgress("late"))
	*e.now = time.Date(2026, 11, 23, 7, 59, 0, 0, time.UTC) // aún dentro de la tolerancia
	if n, err := e.svc.DetectLate(ctx); err != nil || n != 0 || e.repo.bs["late"].IsLate {
		t.Fatalf("antes de lateAt: n=%d err=%v isLate=%v", n, err, e.repo.bs["late"].IsLate)
	}
	*e.now = time.Date(2026, 11, 23, 8, 1, 0, 0, time.UTC)
	n, err := e.svc.DetectLate(ctx)
	if err != nil || n != 1 || !e.repo.bs["late"].IsLate {
		t.Fatalf("n=%d err=%v isLate=%v", n, err, e.repo.bs["late"].IsLate)
	}
	if e.not.count("booking_late:owner") != 1 || e.not.count("booking_late:req") != 1 {
		t.Errorf("RF-09: ambas partes deben recibir aviso: %v", e.not.sent)
	}
	// Idempotente: otra pasada no vuelve a marcar ni a avisar.
	if n, _ := e.svc.DetectLate(ctx); n != 0 || len(e.not.sent) != 2 {
		t.Errorf("segunda pasada: n=%d avisos=%v", n, e.not.sent)
	}
}

func TestDetectLateIgnoresReturnedAndOnTime(t *testing.T) {
	done := inProgress("done")
	done.Status = bookings.StatusCompleted
	onTime := inProgress("ontime")
	later := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	onTime.LateAt = &later
	e := newEnv(t, done, onTime)
	*e.now = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	if n, err := e.svc.DetectLate(ctx); err != nil || n != 0 || len(e.not.sent) != 0 {
		t.Errorf("n=%d err=%v avisos=%v", n, err, e.not.sent)
	}
}

func TestDetectLateMarksAffectedBooking(t *testing.T) { // CA-08
	e := newEnv(t,
		inProgress("late"),
		accepted("next", "2026-11-23", "2026-11-24"),   // empieza hoy: afectada
		accepted("future", "2026-11-28", "2026-11-29"), // aún no le toca
		accepted("marked", "2026-11-23", "2026-11-23"), // ya afectada por otra
	)
	other := "otra-tardia"
	e.repo.bs["marked"].AffectedBy = &other
	e.repo.bs["next"].ItemID, e.repo.bs["future"].ItemID, e.repo.bs["marked"].ItemID = "i1", "i1", "i1"
	*e.now = time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC) // 09:00 en México del 23

	if n, err := e.svc.DetectLate(ctx); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if got := e.repo.bs["next"].AffectedBy; got == nil || *got != "late" {
		t.Errorf("next.AffectedBy = %v", got)
	}
	if e.repo.bs["future"].AffectedBy != nil {
		t.Error("una reserva que aún no empieza no se marca")
	}
	if *e.repo.bs["marked"].AffectedBy != "otra-tardia" {
		t.Error("una ya marcada no se pisa")
	}
	if e.not.count("booking_affected:next-next") != 1 || e.not.count("booking_affected") != 1 {
		t.Errorf("solo el solicitante de la afectada recibe aviso: %v", e.not.sent)
	}
	// Otra pasada no repite el aviso.
	_, _ = e.svc.DetectLate(ctx)
	if e.not.count("booking_affected") != 1 {
		t.Errorf("aviso repetido: %v", e.not.sent)
	}
}

func TestDetectLateAnotherReplicaWins(t *testing.T) {
	e := newEnv(t, inProgress("late"))
	*e.now = time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	// Otra réplica la marca entre DueLate y MarkLate.
	racy := &lateRacy{memRepo: e.repo}
	svc, _ := NewService(Deps{Repo: racy, Bookings: racy, Notifier: e.not, Log: e.svc.d.Log, Now: func() time.Time { return *e.now }})
	if n, err := svc.DetectLate(ctx); err != nil || n != 0 || len(e.not.sent) != 0 {
		t.Errorf("n=%d err=%v avisos=%v", n, err, e.not.sent)
	}
}

type lateRacy struct{ *memRepo }

func (r *lateRacy) MarkLate(c context.Context, id string, at time.Time) error {
	r.mu.Lock()
	r.bs[id].IsLate = true // la otra réplica llegó primero
	r.mu.Unlock()
	return r.memRepo.MarkLate(c, id, at)
}

func TestDetectLateErrors(t *testing.T) {
	boom := errors.New("mongo caído")
	e := newEnv(t, inProgress("late"))
	*e.now = time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	for name, repo := range map[string]Repository{
		"DueLate":            &failRepo{memRepo: e.repo, failDue: boom},
		"MarkLate":           &failRepo{memRepo: e.repo, failMark: boom},
		"AffectedCandidates": &failRepo{memRepo: e.repo, failCands: boom},
	} {
		svc, _ := NewService(Deps{Repo: repo, Bookings: e.repo, Notifier: e.not, Log: e.svc.d.Log, Now: func() time.Time { return *e.now }})
		if _, err := svc.DetectLate(ctx); !errors.Is(err, boom) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	// Un fallo al marcar la afectada también se propaga.
	e2 := newEnv(t, inProgress("late"), accepted("next", "2026-11-23", "2026-11-23"))
	svc, _ := NewService(Deps{Repo: &failRepo{memRepo: e2.repo, failAffected: boom}, Bookings: e2.repo, Notifier: e2.not, Log: e.svc.d.Log, Now: func() time.Time { return *e.now }})
	if _, err := svc.DetectLate(ctx); !errors.Is(err, boom) {
		t.Errorf("MarkAffected: err = %v", err)
	}
}

type failRepo struct {
	*memRepo
	failDue, failMark, failCands, failAffected error
}

func (r *failRepo) DueLate(c context.Context, now time.Time, limit int) ([]LateBooking, error) {
	if r.failDue != nil {
		return nil, r.failDue
	}
	return r.memRepo.DueLate(c, now, limit)
}

func (r *failRepo) MarkLate(c context.Context, id string, at time.Time) error {
	if r.failMark != nil {
		return r.failMark
	}
	return r.memRepo.MarkLate(c, id, at)
}

func (r *failRepo) AffectedCandidates(c context.Context, l LateBooking, today bookings.Date) ([]LateBooking, error) {
	if r.failCands != nil {
		return nil, r.failCands
	}
	return r.memRepo.AffectedCandidates(c, l, today)
}

func (r *failRepo) MarkAffected(c context.Context, id, lateID string, at time.Time) error {
	if r.failAffected != nil {
		return r.failAffected
	}
	return r.memRepo.MarkAffected(c, id, lateID, at)
}
