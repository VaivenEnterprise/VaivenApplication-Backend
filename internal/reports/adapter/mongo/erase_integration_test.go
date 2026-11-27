//go:build integration

package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
)

func rawReport(t *testing.T, r *Repo, id string) bson.Raw {
	t.Helper()
	oid, _ := bson.ObjectIDFromHex(id)
	var raw bson.Raw
	if err := r.col.FindOne(context.Background(), bson.D{{Key: "_id", Value: oid}}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func purgeOf(raw bson.Raw) (time.Time, bool) {
	v, err := raw.LookupErr(PurgeAtField)
	if err != nil {
		return time.Time{}, false
	}
	return v.Time().UTC(), true
}

// Spec 021, T026 (D-4, N-3, CA-11): purgeAt = baja + 12 meses en los reportes
// que la persona hizo o que recibió como persona reportada; los demás (incluido
// un reporte sobre un objeto de la persona) no cambian; el texto tampoco.
func TestIntegrationEraserSchedulesPurge(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	er := NewEraser(db)
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	purge := time.Date(2027, 11, 26, 12, 0, 0, 0, time.UTC)
	const ana, beto = "6730f1c2a9b3e4d5f6a7b8a1", "6730f1c2a9b3e4d5f6a7b8b2"

	made, _, _ := repo.Create(ctx, newReport(repo, ana, domain.TargetItem, itemID))         // hecho por Ana
	received, _, _ := repo.Create(ctx, newReport(repo, beto, domain.TargetUser, ana))       // recibido por Ana
	onItem, _, _ := repo.Create(ctx, newReport(repo, beto, domain.TargetItem, ana))         // targetId = Ana, pero es un objeto
	unrelated, _, _ := repo.Create(ctx, newReport(repo, beto, domain.TargetUser, reporter)) // de otras personas
	onItemBefore, unrelatedBefore := rawReport(t, repo, onItem.ID), rawReport(t, repo, unrelated.ID)
	madeBefore := rawReport(t, repo, made.ID)

	res, err := er.Erase(ctx, ana, at)
	if err != nil {
		t.Fatalf("Erase: %v", err)
	}
	for _, id := range []string{made.ID, received.ID} {
		if p, ok := purgeOf(rawReport(t, repo, id)); !ok || !p.Equal(purge) {
			t.Errorf("purgeAt = %v (%v); quiero %v", p, ok, purge)
		}
	}
	if string(rawReport(t, repo, onItem.ID)) != string(onItemBefore) || string(rawReport(t, repo, unrelated.ID)) != string(unrelatedBefore) {
		t.Error("un reporte ajeno cambió o tiene purgeAt")
	}
	if res.Counts["reports.purgeScheduled"] != 2 || len(res.Images) != 0 {
		t.Errorf("resultado = %+v", res)
	}
	// Texto sin cambios: el documento solo gana purgeAt.
	read, err := repo.ByID(ctx, made.ID)
	if err != nil || read.Details == nil || *read.Details != "Detalle" || len(rawReport(t, repo, made.ID)) <= len(madeBefore) {
		t.Errorf("reporte tras la baja = %+v, %v", read, err)
	}
	// CA-11 con reloj falso.
	if p, _ := purgeOf(rawReport(t, repo, made.ID)); !at.AddDate(0, 11, 0).Before(p) || p.After(at.AddDate(0, 12, 1)) {
		t.Errorf("a los 11 meses debe existir y a los 12 vencer: purgeAt = %v", p)
	}

	// Idempotente; y la otra parte, dada de baja después, no cambia la fecha más temprana.
	if res2, err := er.Erase(ctx, ana, at); err != nil || res2.Counts["reports.purgeScheduled"] != 0 {
		t.Errorf("segunda pasada = %+v, %v", res2, err)
	}
	if _, err := er.Erase(ctx, beto, at.AddDate(0, 3, 0)); err != nil {
		t.Fatal(err)
	}
	if p, _ := purgeOf(rawReport(t, repo, received.ID)); !p.Equal(purge) {
		t.Errorf("la fecha más temprana debe prevalecer: %v", p)
	}
	if p, ok := purgeOf(rawReport(t, repo, onItem.ID)); !ok || !p.Equal(at.AddDate(0, 15, 0)) {
		t.Errorf("el reporte de Beto sobre un objeto = %v (%v)", p, ok)
	}
}

// Un error posterior en la transacción deshace purgeAt (CA-05).
func TestIntegrationEraserRollsBackWithTransaction(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	rep, _, _ := repo.Create(ctx, newReport(repo, reporter, domain.TargetItem, itemID))
	boom := errors.New("falla a media transacción")
	err := adaptermongo.NewTxRunner(db).Do(ctx, func(tx context.Context) error {
		if _, err := NewEraser(db).Erase(tx, reporter, time.Now()); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := purgeOf(rawReport(t, repo, rep.ID)); ok {
		t.Error("purgeAt debía deshacerse con la transacción")
	}
}
