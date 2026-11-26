//go:build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	recomongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/adapter/mongo"
)

// Recorrido real de los tres subcomandos (spec 015, T008) contra MongoDB:
// sembrar interacciones de demostración, recalcular, evaluar con --source db
// y purgar.
func TestIntegrationRecoCommands(t *testing.T) {
	client, uri := testMongo(t)
	const dbName = "vaiven_reco_cmd"
	db := client.Database(dbName)
	ctx := context.Background()
	if err := recomongo.NewInteractionRepo(db).EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	owner := bson.NewObjectID()
	if _, err := db.Collection("users").InsertOne(ctx, bson.D{{Key: "_id", Value: owner}, {Key: "status", Value: "active"}}); err != nil {
		t.Fatal(err)
	}
	var items []any
	for i := range 40 {
		items = append(items, bson.D{
			{Key: "ownerId", Value: owner.Hex()}, {Key: "title", Value: fmt.Sprintf("Objeto %d", i)},
			{Key: "category", Value: []string{"hogar", "herramientas_electricas", "deportes", "otros"}[i%4]},
			{Key: "modality", Value: "rent"}, {Key: "pricePerDayCents", Value: int64(1000 * (i + 1))},
			{Key: "zone", Value: bson.D{{Key: "municipality", Value: []string{"Tula", "Pachuca"}[i%2]}}},
			{Key: "status", Value: "active"}, {Key: "createdAt", Value: time.Now().UTC()},
		})
	}
	if _, err := db.Collection("items").InsertMany(ctx, items); err != nil {
		t.Fatal(err)
	}
	load := func() (config.Config, error) {
		return config.Config{AppEnv: config.EnvTest, MongoURI: uri, MongoDB: dbName, MongoConnectTimeout: 30 * time.Second}, nil
	}

	var out, errb bytes.Buffer
	if code := runRecoSeed([]string{"--users", "50"}, &out, &errb, load, mongoRecoDeps); code != 0 {
		t.Fatalf("seed-interactions: %d %s", code, errb.String())
	}
	// 50 personas × 12 = 600 eventos; los que repiten persona y objeto en el
	// mismo bloque de 10 min los descarta el índice único (CA-09).
	if n, _ := db.Collection(recomongo.InteractionsCollection).CountDocuments(ctx, bson.D{{Key: "demo", Value: true}}); n < 400 || n > 600 {
		t.Fatalf("interacciones de demostración = %d", n)
	}
	out.Reset()
	if code := runRecoRecompute(&out, &errb, load, mongoRecoDeps); code != 0 || !strings.Contains(out.String(), "40 objetos activos") {
		t.Fatalf("recompute: %d %s %s", code, out.String(), errb.String())
	}
	if _, _, ok, err := recomongo.NewMetaRepo(db).Current(ctx); err != nil || !ok {
		t.Fatalf("no hay modelo vigente: %v", err)
	}
	if n, _ := db.Collection(recomongo.ModelCollection).CountDocuments(ctx, bson.D{}); n == 0 {
		t.Error("el modelo no tiene vecinos")
	}
	out.Reset()
	if code := runRecoEval([]string{"--source", "db"}, &out, &errb, load, mongoRecoDeps); code != 0 || !strings.Contains(out.String(), "Personas evaluadas: 50") {
		t.Errorf("eval --source db: %d %s", code, out.String())
	}
	out.Reset()
	if code := runRecoSeed([]string{"--purge"}, &out, &errb, load, mongoRecoDeps); code != 0 {
		t.Fatalf("purge: %d %s", code, errb.String())
	}
	if n, _ := db.Collection(recomongo.InteractionsCollection).CountDocuments(ctx, bson.D{}); n != 0 {
		t.Errorf("quedan %d interacciones", n)
	}
}
