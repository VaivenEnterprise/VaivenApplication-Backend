//go:build integration

package mongo

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Spec 021, T020 (CA-01): se borran los eventos y la marca de reinicio de la
// persona, sin tocar a nadie más ni el candado del modelo.
func TestIntegrationEraserDeletesRecoData(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	inter := NewInteractionRepo(db)
	meta := NewMetaRepo(db)
	er := NewEraser(db)
	if err := inter.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	for _, e := range []struct{ user, item string }{{"ana", "i1"}, {"ana", "i2"}, {"ana", "i3"}, {"beto", "i1"}} {
		if ok, err := inter.Record(ctx, view(e.user, e.item, t0)); err != nil || !ok {
			t.Fatalf("Record = %v, %v", ok, err)
		}
	}
	if err := meta.SetReset(ctx, "ana", t0); err != nil {
		t.Fatal(err)
	}
	if err := meta.SetReset(ctx, "beto", t0); err != nil {
		t.Fatal(err)
	}
	// El documento del modelo (_id = "model") no es de nadie y no se toca.
	if _, err := db.Collection(MetaCollection).InsertOne(ctx, bson.M{"_id": modelMetaID, "buildId": "b1"}); err != nil {
		t.Fatal(err)
	}

	res, err := er.Erase(ctx, "ana", t0)
	if err != nil || res.Counts["reco_interactions.deleted"] != 3 || res.Counts["reco_states.deleted"] != 1 {
		t.Fatalf("Erase = %+v, %v", res, err)
	}
	if n, _ := db.Collection(InteractionsCollection).CountDocuments(ctx, bson.D{{Key: "userId", Value: "ana"}}); n != 0 {
		t.Errorf("quedan %d interacciones de Ana", n)
	}
	if n, _ := db.Collection(InteractionsCollection).CountDocuments(ctx, bson.D{{Key: "userId", Value: "beto"}}); n != 1 {
		t.Errorf("las de Beto = %d, want 1", n)
	}
	if _, found, err := meta.ResetAt(ctx, "ana"); err != nil || found {
		t.Errorf("la marca de reinicio de Ana debe borrarse: %v, %v", found, err)
	}
	if _, found, _ := meta.ResetAt(ctx, "beto"); !found {
		t.Error("la marca de Beto no se toca")
	}
	if n, _ := db.Collection(MetaCollection).CountDocuments(ctx, bson.D{{Key: "_id", Value: modelMetaID}}); n != 1 {
		t.Error("el documento del modelo no se toca")
	}
	resets, err := meta.Resets(ctx)
	if err != nil || len(resets) != 1 {
		t.Errorf("Resets = %v, %v", resets, err)
	}

	// Idempotente, y la purga posterior del recálculo no falla sin datos.
	if again, err := er.Erase(ctx, "ana", t0); err != nil || again.Counts["reco_interactions.deleted"] != 0 || again.Counts["reco_states.deleted"] != 0 {
		t.Errorf("segunda ejecución = %+v, %v", again, err)
	}
	if err := inter.DeleteUsers(ctx, []string{"ana"}); err != nil {
		t.Errorf("DeleteUsers sin datos = %v", err)
	}
	if err := inter.DeleteUser(ctx, "ana"); err != nil {
		t.Errorf("DeleteUser sin datos = %v", err)
	}
	if er.Name() != "reco" {
		t.Errorf("Name = %q", er.Name())
	}
}
