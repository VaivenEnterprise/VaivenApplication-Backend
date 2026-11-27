//go:build integration

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
)

// verifyOn ejecuta verify-no-nulls contra db y devuelve el código y la salida.
func verifyOn(t *testing.T, db *drv.Database) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := verifyNulls(context.Background(), &out, &errb, "", dbNullSource{db: db})
	if errb.Len() > 0 {
		t.Logf("stderr: %s", errb.String())
	}
	return code, out.String()
}

func migrateOn(t *testing.T, db *drv.Database, apply bool) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := migrateNulls(context.Background(), &out, &errb, "", dbNullMigrator{dbNullSource{db: db}}, apply, "respaldo-prueba")
	if errb.Len() > 0 {
		t.Logf("stderr: %s", errb.String())
	}
	return code, out.String()
}

// CA-15 sobre MongoDB real: una copia con null en la raíz, anidados y dentro de
// arreglos queda con cero null; el dry-run no escribe; repetir cambia 0; no se
// borra ningún documento y los valores reales se conservan.
func TestIntegrationMigrateRemoveNulls(t *testing.T) {
	client, _ := testMongo(t)
	db := client.Database("vaiven_migrate")
	ctx := context.Background()

	users := db.Collection("users")
	bookings := db.Collection("bookings")
	clean := db.Collection("items")
	if _, err := users.InsertMany(ctx, []any{
		bson.D{{Key: "_id", Value: 1}, {Key: "email", Value: "a@example.com"}, {Key: "phone", Value: nil}, {Key: "suspendedUntil", Value: nil}},
		bson.D{{Key: "_id", Value: 2}, {Key: "email", Value: "b@example.com"}, {Key: "phone", Value: "+5255"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := bookings.InsertMany(ctx, []any{
		bson.D{{Key: "_id", Value: 1}, {Key: "meetingPoint", Value: nil}, {Key: "itemSnapshot", Value: bson.D{{Key: "title", Value: "T"}, {Key: "firstPhoto", Value: nil}}},
			{Key: "payments", Value: bson.A{
				bson.D{{Key: "stage", Value: "deposit"}, {Key: "reportedAt", Value: nil}, {Key: "amountCents", Value: 500}},
				bson.D{{Key: "stage", Value: "rent"}, {Key: "reportedAt", Value: "2026-11-26"}},
			}},
			{Key: "tags", Value: bson.A{"a", nil, "b", nil}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := clean.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "title", Value: "Taladro"}}); err != nil {
		t.Fatal(err)
	}
	if code, out := verifyOn(t, db); code != verifyNullsFound {
		t.Fatalf("antes: verify = %d, want 1\n%s", code, out)
	}

	// Dry-run: informa y no escribe.
	if code, out := migrateOn(t, db, false); code != migrateOK || !strings.Contains(out, "cambiaría=1") {
		t.Fatalf("dry-run: código %d\n%s", code, out)
	}
	if code, _ := verifyOn(t, db); code != verifyNullsFound {
		t.Fatal("el dry-run no debe quitar nulls")
	}

	// Apply.
	code, out := migrateOn(t, db, true)
	if code != migrateOK || !strings.Contains(out, `backup-ref="respaldo-prueba"`) {
		t.Fatalf("apply: código %d\n%s", code, out)
	}
	if code, out := verifyOn(t, db); code != verifyNullsClean {
		t.Fatalf("tras apply: verify = %d, want 0\n%s", code, out)
	}

	// Idempotente: la segunda pasada no cambia nada.
	code, out = migrateOn(t, db, true)
	if code != migrateOK || !strings.Contains(out, "0 documentos modificados") {
		t.Fatalf("segunda pasada: código %d\n%s", code, out)
	}

	// No se borró ningún documento y los valores reales siguen ahí.
	for col, want := range map[string]int64{"users": 2, "bookings": 1, "items": 1} {
		if n, err := db.Collection(col).CountDocuments(ctx, bson.D{}); err != nil || n != want {
			t.Errorf("%s: %d documentos (%v), want %d", col, n, err, want)
		}
	}
	var u struct {
		Email string  `bson:"email"`
		Phone *string `bson:"phone"`
	}
	if err := users.FindOne(ctx, bson.D{{Key: "_id", Value: 2}}).Decode(&u); err != nil || u.Phone == nil || *u.Phone != "+5255" {
		t.Errorf("el teléfono real se perdió: %+v %v", u, err)
	}
	var b bson.Raw
	if err := bookings.FindOne(ctx, bson.D{{Key: "_id", Value: 1}}).Decode(&b); err != nil {
		t.Fatal(err)
	}
	pays, _ := b.Lookup("payments").Array().Values()
	if len(pays) != 2 || pays[1].Document().Lookup("reportedAt").StringValue() != "2026-11-26" {
		t.Errorf("payments alterado: %v", b.Lookup("payments"))
	}
	if _, err := pays[0].Document().LookupErr("reportedAt"); err == nil {
		t.Error("payments[0].reportedAt debía quitarse")
	}
	tags, _ := b.Lookup("tags").Array().Values()
	if len(tags) != 2 || tags[0].StringValue() != "a" || tags[1].StringValue() != "b" {
		t.Errorf("tags = %v, want [a b]", b.Lookup("tags"))
	}
}

// Una ruta con más de un nivel de arreglo se reporta y no se toca.
func TestIntegrationMigrateUnsupportedPathUntouched(t *testing.T) {
	client, _ := testMongo(t)
	db := client.Database("vaiven_migrate_unsup")
	ctx := context.Background()
	if _, err := db.Collection("x").InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "m", Value: bson.A{bson.A{nil, 1}}}, {Key: "a", Value: nil}}); err != nil {
		t.Fatal(err)
	}
	code, out := migrateOn(t, db, true)
	if code != migrateUnsupported || !strings.Contains(out, "NO SOPORTADA (no se toca): x.m[][]") {
		t.Fatalf("código %d\n%s", code, out)
	}
	var raw bson.Raw
	if err := db.Collection("x").FindOne(ctx, bson.D{}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.LookupErr("a"); err == nil {
		t.Error("el null soportado (a) debía quitarse")
	}
	inner, _ := raw.Lookup("m").Array().Values()
	if len(inner) != 1 {
		t.Fatalf("m = %v", raw.Lookup("m"))
	}
	if vs, _ := inner[0].Array().Values(); len(vs) != 2 || vs[0].Type != bson.TypeNull {
		t.Errorf("el arreglo anidado no debía tocarse: %v", raw.Lookup("m"))
	}
}
