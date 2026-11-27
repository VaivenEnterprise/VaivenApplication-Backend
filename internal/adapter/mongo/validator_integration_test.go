//go:build integration

package mongo

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

const codeDocumentValidationFailure = 121

// Spec 021, T008: EnsureValidator es idempotente; con warn un documento con
// null entra (y el servidor deja el aviso en su registro) y uno válido también;
// con error el mismo null recibe el código 121.
func TestIntegrationEnsureOptionalFieldValidators(t *testing.T) {
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatalf("no se pudo iniciar MongoDB con Docker: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	uri, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	client, err := mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("vaiven_validators")

	names := make([]string, 0, len(OptionalFieldSchemas()))
	for n := range OptionalFieldSchemas() {
		names = append(names, n)
	}
	slices.Sort(names)

	// Primera vez: crea todas (warn); segunda: sin cambios.
	first, err := EnsureOptionalFieldValidators(ctx, db, ValidationActionWarn)
	if err != nil || !slices.Equal(first.Created, names) || len(first.Updated)+len(first.Skipped)+len(first.Unchanged) != 0 {
		t.Fatalf("primera vez = %+v, %v", first, err)
	}
	second, err := EnsureOptionalFieldValidators(ctx, db, ValidationActionWarn)
	if err != nil || !slices.Equal(second.Unchanged, names) || len(second.Created)+len(second.Updated)+len(second.Skipped) != 0 {
		t.Fatalf("segunda vez = %+v, %v", second, err)
	}

	ratings := db.Collection("ratings")
	valid := bson.M{"_id": bson.NewObjectID(), "score": 5, "comment": "Excelente"}
	withNull := bson.M{"_id": bson.NewObjectID(), "score": 4, "comment": nil}
	for name, doc := range map[string]bson.M{"válido": valid, "con null (warn)": withNull} {
		if _, err := ratings.InsertOne(ctx, doc); err != nil {
			t.Fatalf("warn: insertar %s debe entrar: %v", name, err)
		}
	}
	// Un documento con tipo equivocado en un opcional anidado también solo avisa.
	if _, err := db.Collection("bookings").InsertOne(ctx, bson.M{"_id": bson.NewObjectID(), "delivery": bson.M{"qr": bson.M{"usedAt": nil}}}); err != nil {
		t.Fatalf("warn: insertar reserva con null anidado debe entrar: %v", err)
	}

	// Con error (T030): el null se rechaza con el código 121 y lo válido entra;
	// el documento viejo con null (nivel moderate) sigue pudiéndose actualizar.
	upd, err := EnsureOptionalFieldValidators(ctx, db, ValidationActionError)
	if err != nil || !slices.Equal(upd.Updated, names) {
		t.Fatalf("pasar a error = %+v, %v", upd, err)
	}
	_, err = ratings.InsertOne(ctx, bson.M{"_id": bson.NewObjectID(), "score": 3, "comment": nil})
	var we drv.WriteException
	if err == nil || !errors.As(err, &we) || !we.HasErrorCode(codeDocumentValidationFailure) {
		t.Errorf("error: el null debe rechazarse con código %d, got %v", codeDocumentValidationFailure, err)
	}
	if _, err := ratings.InsertOne(ctx, bson.M{"_id": bson.NewObjectID(), "score": 3}); err != nil {
		t.Errorf("error: el documento sin el campo debe entrar: %v", err)
	}
	if _, err := ratings.UpdateOne(ctx, bson.M{"_id": withNull["_id"]}, bson.M{"$set": bson.M{"score": 2}}); err != nil {
		t.Errorf("moderate: el documento viejo con null debe poder actualizarse: %v", err)
	}
}
