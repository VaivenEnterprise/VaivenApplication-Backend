//go:build integration

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
)

// startup hace lo mismo que el arranque de cmd/api con los validadores.
func startupValidators(t *testing.T, db *drv.Database) {
	t.Helper()
	ctx := context.Background()
	if _, err := authmongo.EnsureSchemas(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := itemsmongo.EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := adaptermongo.EnsureOptionalFieldValidators(ctx, db, adaptermongo.OptionalFieldsAction); err != nil {
		t.Fatal(err)
	}
}

func states(t *testing.T, db *drv.Database) map[adaptermongo.Strictness]int {
	t.Helper()
	out := map[adaptermongo.Strictness]int{}
	for _, sp := range allValidatorSpecs() {
		st, err := adaptermongo.ValidatorState(context.Background(), db, sp)
		if err != nil {
			t.Fatal(err)
		}
		out[st]++
	}
	return out
}

func isValidationFailure(err error) bool {
	var we drv.WriteException
	return errors.As(err, &we) && we.HasErrorCode(121)
}

// Spec 021, T030 (CA-14, despliegue seguro): el arranque de la API deja todo
// tolerante y NO endurece; `enforce-validators` es el único camino a estricto y
// se niega con null; ya estricto, un reinicio no lo afloja; una escritura con
// null recibe el código 121 en users, items y el resto; y se puede volver atrás.
func TestIntegrationEnforceValidatorsLifecycle(t *testing.T) {
	ctx := context.Background()
	client, _ := testMongo(t)
	db := client.Database(fmt.Sprintf("enforce_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	total := len(allValidatorSpecs())

	// 1. Arranque (despliegue A): todo tolerante; un null entra.
	startupValidators(t, db)
	if got := states(t, db); got[adaptermongo.StrictnessTolerant] != total {
		t.Fatalf("tras el arranque = %v; todo debe quedar tolerante", got)
	}
	if _, err := db.Collection("ratings").InsertOne(ctx, bson.M{"score": 4, "comment": nil}); err != nil {
		t.Fatalf("tolerante: un null debe entrar: %v", err)
	}
	startupValidators(t, db) // reiniciar no endurece
	if got := states(t, db); got[adaptermongo.StrictnessTolerant] != total {
		t.Fatalf("un reinicio no debe endurecer: %v", got)
	}

	tool := dbValidatorTool{dbNullSource: dbNullSource{db: db}, db: db}
	run := func(args ...string) (int, string) {
		var out, errOut bytes.Buffer
		code := runEnforceValidators(args, &out, &errOut,
			func() (config.Config, error) { return config.Config{AppEnv: config.EnvStaging}, nil },
			func(context.Context, config.Config) (validatorTool, func(), error) { return tool, func() {}, nil })
		return code, out.String() + errOut.String()
	}

	// 2. Con un null sin migrar se niega.
	if code, out := run("--apply"); code != enforceRefused {
		t.Fatalf("con null debe negarse: %d\n%s", code, out)
	}
	if got := states(t, db); got[adaptermongo.StrictnessTolerant] != total {
		t.Fatalf("negarse no debe cambiar nada: %v", got)
	}

	// 3. Migrado (se quita el null), dry-run no cambia y --apply endurece.
	if _, err := db.Collection("ratings").DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatal(err)
	}
	if code, out := run(); code != enforceOK || states(t, db)[adaptermongo.StrictnessStrict] != 0 {
		t.Fatalf("dry-run: %d\n%s", code, out)
	}
	if code, out := run("--apply"); code != enforceOK {
		t.Fatalf("--apply: %d\n%s", code, out)
	}
	if got := states(t, db); got[adaptermongo.StrictnessStrict] != total {
		t.Fatalf("tras --apply = %v", got)
	}

	// 4. Estricto: el null recibe 121 en users, items y el resto; lo válido entra.
	now := time.Now().UTC()
	userBase := func() bson.M {
		return bson.M{"email": "a@example.com", "displayName": "A", "status": "active", "passwordHash": "h", "role": "user",
			"ratingSum": int64(0), "ratingCount": int64(0), "acceptedTermsAt": now, "createdAt": now, "updatedAt": now, "version": int64(1)}
	}
	if _, err := db.Collection("users").InsertOne(ctx, userBase()); err != nil {
		t.Fatalf("un usuario válido debe entrar: %v", err)
	}
	withNull := userBase()
	withNull["email"] = "b@example.com"
	withNull["phone"] = nil
	if _, err := db.Collection("users").InsertOne(ctx, withNull); !isValidationFailure(err) {
		t.Errorf("users con phone null: %v, want 121", err)
	}
	if _, err := db.Collection("refresh_tokens").InsertOne(ctx, bson.M{"userId": "u", "familyId": "f", "tokenHash": "h", "expiresAt": now, "createdAt": now, "usedAt": nil}); !isValidationFailure(err) {
		t.Errorf("refresh_tokens con usedAt null: %v, want 121", err)
	}
	item := bson.M{"ownerId": "o", "title": "t", "description": "d", "category": "c", "condition": "good", "modality": "loan",
		"declaredValueCents": int64(5000), "pricePerDayCents": int64(0), "priceSource": "manual", "zone": bson.M{"municipality": "m", "neighborhood": nil},
		"photos": bson.A{bson.M{"publicId": "p"}}, "status": "active", "createdAt": now, "updatedAt": now, "version": int64(1)}
	if _, err := db.Collection("items").InsertOne(ctx, item); !isValidationFailure(err) {
		t.Errorf("items con zone.neighborhood null: %v, want 121", err)
	}
	for _, c := range []struct {
		col string
		doc bson.M
	}{
		{"ratings", bson.M{"score": 3, "comment": nil}},
		{"bookings", bson.M{"cancelReason": nil}},
		{"reports", bson.M{"details": nil}},
		{"incidents", bson.M{"closedAt": nil}},
		{"sanctions", bson.M{"endsAt": nil}},
		{"notifications", bson.M{"readAt": nil}},
		{"reco_states", bson.M{"builtAt": nil}},
	} {
		if _, err := db.Collection(c.col).InsertOne(ctx, c.doc); !isValidationFailure(err) {
			t.Errorf("%s con null: %v, want 121", c.col, err)
		}
		valid := bson.M{}
		for k, v := range c.doc {
			if v != nil {
				valid[k] = v
			}
		}
		if _, err := db.Collection(c.col).InsertOne(ctx, valid); err != nil {
			t.Errorf("%s sin el campo debe entrar: %v", c.col, err)
		}
	}

	// 5. Un reinicio de la API no afloja el modo estricto.
	startupValidators(t, db)
	if got := states(t, db); got[adaptermongo.StrictnessStrict] != total {
		t.Fatalf("un reinicio no debe aflojar el estricto: %v", got)
	}

	// 6. Camino de vuelta explícito.
	if code, out := run("--apply", "--tolerant"); code != enforceOK {
		t.Fatalf("--tolerant: %d\n%s", code, out)
	}
	if got := states(t, db); got[adaptermongo.StrictnessTolerant] != total {
		t.Fatalf("tras volver = %v", got)
	}
}
