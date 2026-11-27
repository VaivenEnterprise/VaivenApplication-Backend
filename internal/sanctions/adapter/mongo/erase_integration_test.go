//go:build integration

package mongo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/domain"
)

func rawSanction(t *testing.T, r *Repo, id string) bson.Raw {
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

// Spec 021, T026 (D-4, N-3, CA-11): purgeAt = baja + 12 meses solo en las
// sanciones de la persona; el texto no cambia y el resto no se toca.
func TestIntegrationEraserSchedulesPurge(t *testing.T) {
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("sanctions_erase_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := NewRepo(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	er := NewEraser(db)
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	purge := time.Date(2027, 11, 26, 12, 0, 0, 0, time.UTC)
	now := at.Add(-48 * time.Hour)

	mine := func(user string, n int) domain.Sanction {
		s, err := repo.Create(ctx, domain.Sanction{ID: repo.NewID(), UserID: user, Type: domain.TypeWarning, Reason: "Llegó tarde.", CreatedByAdminID: "adm", CreatedAt: now.Add(time.Duration(n) * time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	a1, a2, b1 := mine("ana", 0), mine("ana", 1), mine("beto", 2)
	b1Before := rawSanction(t, repo, b1.ID)

	res, err := er.Erase(ctx, "ana", at)
	if err != nil {
		t.Fatalf("Erase: %v", err)
	}
	for _, id := range []string{a1.ID, a2.ID} {
		raw := rawSanction(t, repo, id)
		if p, ok := purgeOf(raw); !ok || !p.Equal(purge) {
			t.Errorf("purgeAt = %v (%v); quiero %v", p, ok, purge)
		}
		if raw.Lookup("reason").StringValue() != "Llegó tarde." { // N-3: el texto no cambia
			t.Error("el texto cambió antes de purgar")
		}
	}
	if string(rawSanction(t, repo, b1.ID)) != string(b1Before) {
		t.Error("la sanción de otra persona cambió o tiene purgeAt")
	}
	if res.Counts["sanctions.purgeScheduled"] != 2 {
		t.Errorf("counts = %v", res.Counts)
	}
	// CA-11 con reloj falso.
	if p, _ := purgeOf(rawSanction(t, repo, a1.ID)); !at.AddDate(0, 11, 0).Before(p) || p.After(at.AddDate(0, 12, 1)) {
		t.Errorf("a los 11 meses debe existir y a los 12 vencer: purgeAt = %v", p)
	}
	// Idempotente, y una baja posterior no cambia la fecha más temprana.
	if res2, err := er.Erase(ctx, "ana", at.AddDate(0, 1, 0)); err != nil || res2.Counts["sanctions.purgeScheduled"] != 0 {
		t.Errorf("segunda pasada = %+v, %v", res2, err)
	}
	if p, _ := purgeOf(rawSanction(t, repo, a1.ID)); !p.Equal(purge) {
		t.Errorf("la fecha más temprana debe prevalecer: %v", p)
	}
}

// Un error posterior en la transacción deshace purgeAt (CA-05).
func TestIntegrationEraserRollsBackWithTransaction(t *testing.T) {
	ctx := context.Background()
	db := testClient.Database(fmt.Sprintf("sanctions_erase_tx_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := NewRepo(db)
	s, err := repo.Create(ctx, domain.Sanction{ID: repo.NewID(), UserID: "ana", Type: domain.TypeWarning, Reason: "x", CreatedByAdminID: "adm", CreatedAt: time.Now().UTC().Truncate(time.Millisecond)})
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("falla a media transacción")
	err = adaptermongo.NewTxRunner(db).Do(ctx, func(tx context.Context) error {
		if _, err := NewEraser(db).Erase(tx, "ana", time.Now()); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := purgeOf(rawSanction(t, repo, s.ID)); ok {
		t.Error("purgeAt debía deshacerse con la transacción")
	}
}
