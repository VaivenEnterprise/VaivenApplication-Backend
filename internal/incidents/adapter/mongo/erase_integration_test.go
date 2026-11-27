//go:build integration

package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

func rawIncident(t *testing.T, r *Repo, id string) bson.Raw {
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

// withoutPurge devuelve el documento sin purgeAt para comparar que el resto
// (texto incluido) no cambió (N-3).
func withoutPurge(t *testing.T, raw bson.Raw) string {
	t.Helper()
	var m bson.D
	if err := bson.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	out := bson.D{}
	for _, e := range m {
		if e.Key != PurgeAtField {
			out = append(out, e)
		}
	}
	b, err := bson.MarshalExtJSON(out, true, false)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Spec 021, T026 (D-4, N-3, CA-11): purgeAt = baja + 12 meses solo en las
// incidencias donde la persona reporta o es reportada; el texto no cambia; las
// evidencias salen con la fecha de purga; repetir no cambia nada.
func TestIntegrationEraserSchedulesPurge(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	purge := time.Date(2027, 11, 26, 12, 0, 0, 0, time.UTC)
	er := NewEraser(repo.col.Database())

	asReporter, _ := repo.Create(ctx, repo.sample("6731a0000000000000000001", "ana", "beto", 0))
	asReported, _ := repo.Create(ctx, repo.sample("6731a0000000000000000002", "carla", "ana", 1))
	other, _ := repo.Create(ctx, repo.sample("6731a0000000000000000003", "carla", "beto", 2))
	otherBefore := rawIncident(t, repo, other.ID)
	textBefore := withoutPurge(t, rawIncident(t, repo, asReporter.ID))

	res, err := er.Erase(ctx, "ana", at)
	if err != nil {
		t.Fatalf("Erase: %v", err)
	}
	for _, id := range []string{asReporter.ID, asReported.ID} {
		p, ok := purgeOf(rawIncident(t, repo, id))
		if !ok || !p.Equal(purge) {
			t.Errorf("purgeAt = %v (%v); quiero %v", p, ok, purge)
		}
	}
	if string(rawIncident(t, repo, other.ID)) != string(otherBefore) {
		t.Error("la incidencia ajena cambió o tiene purgeAt")
	}
	if got := withoutPurge(t, rawIncident(t, repo, asReporter.ID)); got != textBefore {
		t.Errorf("el texto cambió antes de purgar (N-3):\n%s\n%s", textBefore, got)
	}
	if res.Counts["incidents.purgeScheduled"] != 2 {
		t.Errorf("counts = %v", res.Counts)
	}
	// Una evidencia por incidencia, con la delivery de Cloudinary y la fecha de purga.
	if len(res.Images) != 2 {
		t.Fatalf("evidencias = %+v", res.Images)
	}
	for _, im := range res.Images {
		if im.PublicID != "vaiven/test/evidence/u/1" || im.Delivery != "authenticated" || !im.NotBefore.Equal(purge) {
			t.Errorf("evidencia = %+v", im)
		}
	}

	// CA-11 con reloj falso: a los 11 meses existe; pasados los 12 está vencida.
	p, _ := purgeOf(rawIncident(t, repo, asReporter.ID))
	if !at.AddDate(0, 11, 0).Before(p) || p.After(at.AddDate(0, 12, 1)) {
		t.Errorf("a los 11 meses debe existir y a los 12 vencer: purgeAt = %v", p)
	}

	// Idempotente: sin cambios y sin encolar evidencias otra vez.
	res2, err := er.Erase(ctx, "ana", at)
	if err != nil || len(res2.Images) != 0 || res2.Counts["incidents.purgeScheduled"] != 0 {
		t.Errorf("segunda pasada = %+v, %v", res2, err)
	}

	// La otra parte se da de baja DESPUÉS: gana la fecha más temprana (la de Ana).
	later, err := er.Erase(ctx, "beto", at.AddDate(0, 2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := purgeOf(rawIncident(t, repo, asReporter.ID)); !p.Equal(purge) {
		t.Errorf("la fecha más temprana debe prevalecer: %v", p)
	}
	if p, ok := purgeOf(rawIncident(t, repo, other.ID)); !ok || !p.Equal(at.AddDate(0, 14, 0)) {
		t.Errorf("la incidencia de Beto y Carla = %v (%v)", p, ok)
	}
	if len(later.Images) != 1 { // solo la de Beto con Carla: la de Ana ya tenía fecha igual o anterior
		t.Errorf("evidencias de la segunda baja = %d, quiero 1", len(later.Images))
	}
}

// Una incidencia sin evidencias tampoco falla y no devuelve fotos; sin
// incidencias no hay nada que hacer.
func TestIntegrationEraserWithoutEvidenceOrIncidents(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	er := NewEraser(repo.col.Database())
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

	if res, err := er.Erase(ctx, "nadie", at); err != nil || len(res.Images) != 0 {
		t.Fatalf("sin incidencias = %+v, %v", res, err)
	}
	in := repo.sample("6731a0000000000000000004", "ana", "beto", 0)
	in.Evidence = []images.Image{}
	if _, err := repo.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	res, err := er.Erase(ctx, "beto", at)
	if err != nil || len(res.Images) != 0 || res.Counts["incidents.purgeScheduled"] != 1 {
		t.Errorf("sin evidencias = %+v, %v", res, err)
	}
}

// Un error posterior en la transacción deshace purgeAt (CA-05).
func TestIntegrationEraserRollsBackWithTransaction(t *testing.T) {
	repo, db := newRepo(t)
	ctx := context.Background()
	in, _ := repo.Create(ctx, repo.sample("6731a0000000000000000005", "ana", "beto", 0))
	boom := errors.New("falla a media transacción")
	err := adaptermongo.NewTxRunner(db).Do(ctx, func(tx context.Context) error {
		if _, err := NewEraser(db).Erase(tx, "ana", time.Now()); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := purgeOf(rawIncident(t, repo, in.ID)); ok {
		t.Error("purgeAt debía deshacerse con la transacción")
	}
}
