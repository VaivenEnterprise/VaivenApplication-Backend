package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

func create(t *testing.T, e env, v Viewer) string {
	t.Helper()
	got, err := e.svc.Create(context.Background(), v, rentInput())
	if err != nil {
		t.Fatal(err)
	}
	return got.Item.ID
}

func TestPauseResume(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	id := create(t, e, owner)

	v, err := e.svc.Pause(ctx, owner, id)
	if err != nil || v.Item.Status != domain.StatusPaused || v.Item.Version != 2 {
		t.Fatalf("Pause = %+v %v", v.Item, err)
	}
	// Pausar otra vez no cambia la versión.
	if v, err = e.svc.Pause(ctx, owner, id); err != nil || v.Item.Version != 2 {
		t.Errorf("Pause idempotente = %d %v", v.Item.Version, err)
	}
	if v, err = e.svc.Resume(ctx, owner, id); err != nil || v.Item.Status != domain.StatusActive || v.Item.Version != 3 {
		t.Errorf("Resume = %+v %v", v.Item, err)
	}
	assertCode(t, mustErr(e.svc.Pause(ctx, Viewer{ID: "otro"}, id)), shared.CodeForbidden)
	if err := mustErr(e.svc.Pause(ctx, owner, "nope")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	e.repo.conflicts = 1
	if err := mustErr(e.svc.Pause(ctx, owner, id)); !errors.Is(err, domain.ErrVersionConflict) {
		t.Errorf("conflicto: %v", err)
	}
	_ = e.svc.Block(ctx, id)
	if err := mustErr(e.svc.Resume(ctx, owner, id)); !errors.Is(err, domain.ErrBlocked) {
		t.Errorf("reanudar bloqueado: %v", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	id := create(t, e, owner)

	assertCode(t, e.svc.Delete(ctx, Viewer{ID: "otro"}, id), shared.CodeForbidden)
	if err := e.svc.Delete(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	if e.repo.items[id].Status != domain.StatusRemoved {
		t.Error("no quedó removed")
	}
	// Eliminado: nadie lo ve y no se puede volver a eliminar.
	if _, err := e.svc.Get(ctx, owner, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get eliminado: %v", err)
	}
	if err := e.svc.Delete(ctx, owner, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Delete eliminado: %v", err)
	}
	if err := e.svc.Delete(ctx, owner, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}

	// Un bloqueado se puede eliminar.
	id2 := create(t, e, owner)
	_ = e.svc.Block(ctx, id2)
	if err := e.svc.Delete(ctx, owner, id2); err != nil {
		t.Errorf("eliminar bloqueado: %v", err)
	}
}

func TestDeleteWithBookingsCA07(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, func(d *Deps) { d.Bookings = fakeBookings{busy: true} })
	id := create(t, e, owner)
	if err := e.svc.Delete(ctx, owner, id); !errors.Is(err, domain.ErrHasActiveBookings) {
		t.Errorf("con reservas: %v", err)
	}
	e2 := newEnv(t, func(d *Deps) { d.Bookings = fakeBookings{err: errors.New("caído")} })
	id = create(t, e2, owner)
	if err := e2.svc.Delete(ctx, owner, id); err == nil || errors.Is(err, domain.ErrHasActiveBookings) {
		t.Errorf("error del puerto: %v", err)
	}
}

func TestListMine(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	var ids []string
	for i := range 5 {
		e.now = time.Date(2026, 11, 8, 10, i, 0, 0, time.UTC)
		ids = append(ids, create(t, e, owner))
	}
	create(t, e, Viewer{ID: "otro"})
	_, _ = e.svc.Pause(ctx, owner, ids[1])
	_ = e.svc.Delete(ctx, owner, ids[2])

	// Páginas de 2: más recientes primero, sin removed ni ajenos.
	var seen []string
	q := ListQuery{Limit: 2}
	for range 4 {
		p, err := e.svc.ListMine(ctx, owner, q)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range p.Items {
			seen = append(seen, v.Item.ID)
			if !v.ShowDeclaredValue || v.Owner.ID != "owner1" {
				t.Errorf("vista = %+v", v)
			}
		}
		if p.Next == nil {
			break
		}
		q.After = p.Next
	}
	want := []string{ids[4], ids[3], ids[1], ids[0]}
	if len(seen) != len(want) {
		t.Fatalf("vistos = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("vistos = %v, want %v", seen, want)
		}
	}

	paused := domain.StatusPaused
	p, err := e.svc.ListMine(ctx, owner, ListQuery{Status: &paused})
	if err != nil || len(p.Items) != 1 || p.Items[0].Item.ID != ids[1] || p.Next != nil {
		t.Errorf("pausados = %+v %v", p, err)
	}
	removed := domain.StatusRemoved
	assertCode(t, func() error { _, err := e.svc.ListMine(ctx, owner, ListQuery{Status: &removed}); return err }(), shared.CodeValidation)

	if p, err := e.svc.ListMine(ctx, Viewer{ID: "nadie"}, ListQuery{Limit: 500}); err != nil || len(p.Items) != 0 || p.Items == nil {
		t.Errorf("vacío = %+v %v", p, err)
	}
	e.repo.failNext = errors.New("mongo")
	if _, err := e.svc.ListMine(ctx, owner, ListQuery{}); err == nil {
		t.Error("error del repositorio")
	}
	e2 := newEnv(t, func(d *Deps) { d.Users = fakeUsers{err: errors.New("u")} })
	create(t, newEnv(t), owner)
	e2.repo.items = e.repo.items
	if _, err := e2.svc.ListMine(ctx, owner, ListQuery{}); err == nil {
		t.Error("error de usuarios")
	}
	e.images.urlErr = errors.New("url")
	if _, err := e.svc.ListMine(ctx, owner, ListQuery{}); err == nil {
		t.Error("error de URL")
	}
}

func TestPageSizeBounds(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	for i := range MaxPageSize + 1 {
		e.now = time.Date(2026, 11, 8, 0, 0, i, 0, time.UTC)
		create(t, e, owner)
	}
	p, _ := e.svc.ListMine(ctx, owner, ListQuery{})
	if len(p.Items) != DefaultPageSize || p.Next == nil {
		t.Errorf("por defecto = %d", len(p.Items))
	}
	p, _ = e.svc.ListMine(ctx, owner, ListQuery{Limit: 1000})
	if len(p.Items) != MaxPageSize || p.Next == nil {
		t.Errorf("máximo = %d", len(p.Items))
	}
}

func TestModerationPort(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	id := create(t, e, owner)

	snap, err := e.svc.Snapshot(ctx, id)
	if err != nil || snap.Title != "Taladro percutor" || snap.UsageRules != "Devolver limpio" || snap.Status != "active" || snap.OwnerID != "owner1" {
		t.Errorf("Snapshot = %+v %v", snap, err)
	}
	// Reintenta si el dueño edita a la vez.
	e.repo.conflicts = 2
	if err := e.svc.Block(ctx, id); err != nil {
		t.Fatalf("Block con reintentos: %v", err)
	}
	if e.repo.items[id].Status != domain.StatusBlocked {
		t.Error("no quedó blocked")
	}
	if err := e.svc.Block(ctx, id); err != nil {
		t.Errorf("Block idempotente: %v", err)
	}
	// Bloqueado: otros no lo ven.
	if _, err := e.svc.Get(ctx, Viewer{ID: "otro"}, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("otro ve bloqueado: %v", err)
	}

	id2 := create(t, e, owner)
	e.repo.conflicts = blockAttempts
	if err := e.svc.Block(ctx, id2); !errors.Is(err, domain.ErrVersionConflict) {
		t.Errorf("sin más reintentos: %v", err)
	}
	_ = e.svc.Delete(ctx, owner, id2)
	if err := e.svc.Block(ctx, id2); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Block eliminado: %v", err)
	}
	if snap, err := e.svc.Snapshot(ctx, id2); err != nil || snap.Status != "removed" || snap.UsageRules != "Devolver limpio" {
		t.Errorf("Snapshot eliminado = %+v %v", snap, err)
	}
	if err := e.svc.Block(ctx, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Block inexistente: %v", err)
	}
	if _, err := e.svc.Snapshot(ctx, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Snapshot inexistente: %v", err)
	}
	if busy, err := (NoActiveBookings{}).HasActiveBookings(ctx, id); busy || err != nil {
		t.Error("NoActiveBookings debe responder que no hay reservas")
	}
}
