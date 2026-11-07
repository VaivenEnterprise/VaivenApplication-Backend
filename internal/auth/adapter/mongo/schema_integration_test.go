//go:build integration

package mongo

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// codeDocumentValidationFailure es el error del servidor al rechazar una
// escritura por el validador.
const codeDocumentValidationFailure = 121

// newSchemaDB devuelve una base exclusiva de la prueba con los validadores
// aplicados, para que los repositorios se prueben bajo validación.
func newSchemaDB(t *testing.T) *drv.Database {
	t.Helper()
	db := newDB(t)
	if _, err := EnsureSchemas(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchemas: %v", err)
	}
	return db
}

// collOptions devuelve las opciones de la colección (validator, nivel, acción).
func collOptions(t *testing.T, db *drv.Database, name string) bson.Raw {
	t.Helper()
	specs, err := db.ListCollectionSpecifications(context.Background(), bson.D{{Key: "name", Value: name}})
	if err != nil || len(specs) != 1 {
		t.Fatalf("ListCollectionSpecifications(%s) = %v, %v", name, specs, err)
	}
	return specs[0].Options
}

func assertValidated(t *testing.T, db *drv.Database, name, wantTitle string) {
	t.Helper()
	opts := collOptions(t, db, name)
	title, err := opts.LookupErr("validator", "$jsonSchema", "title")
	if err != nil || title.StringValue() != wantTitle {
		t.Errorf("%s: validator $jsonSchema.title = %v (%v), want %q", name, title, err, wantTitle)
	}
	if lvl := opts.Lookup("validationLevel").StringValue(); lvl != "moderate" {
		t.Errorf("%s: validationLevel = %q, want moderate", name, lvl)
	}
	if act := opts.Lookup("validationAction").StringValue(); act != "error" {
		t.Errorf("%s: validationAction = %q, want error", name, act)
	}
}

func assertRejected(t *testing.T, err error, what string) {
	t.Helper()
	var we drv.WriteException
	if err == nil {
		t.Errorf("%s: la inserción debía rechazarse", what)
		return
	}
	if !errors.As(err, &we) || !we.HasErrorCode(codeDocumentValidationFailure) {
		t.Errorf("%s: err = %v, want código %d", what, err, codeDocumentValidationFailure)
	}
}

func validUserDoc(email string) bson.M {
	now := time.Now().UTC()
	return bson.M{
		"_id": bson.NewObjectID(), "email": email, "passwordHash": "h", "displayName": "Ana",
		"phone": nil, "role": "user", "status": "active", "acceptedTermsAt": now,
		"ratingSum": int64(0), "ratingCount": int64(0), "suspendedUntil": nil,
		"createdAt": now, "updatedAt": now, "version": int64(1), "deactivatedAt": nil,
	}
}

func TestIntegrationEnsureSchemasFreshDB(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)

	first, err := EnsureSchemas(ctx, db)
	if err != nil {
		t.Fatalf("EnsureSchemas (1): %v", err)
	}
	if !slices.Equal(first.Created, []string{UsersCollection, RefreshTokensCollection}) || len(first.Updated) != 0 || len(first.Skipped) != 0 {
		t.Errorf("primera vez = %+v", first)
	}
	// Segunda vez: idempotente, reemplaza el validador con collMod.
	second, err := EnsureSchemas(ctx, db)
	if err != nil {
		t.Fatalf("EnsureSchemas (2): %v", err)
	}
	if len(second.Created) != 0 || !slices.Equal(second.Updated, []string{UsersCollection, RefreshTokensCollection}) || len(second.Skipped) != 0 {
		t.Errorf("segunda vez = %+v", second)
	}
	assertValidated(t, db, UsersCollection, "users")
	assertValidated(t, db, RefreshTokensCollection, "refresh_tokens")

	// Casos inválidos rechazados con el código 121.
	users := db.Collection(UsersCollection)
	noEmail := validUserDoc("x@example.com")
	delete(noEmail, "email")
	_, err = users.InsertOne(ctx, noEmail)
	assertRejected(t, err, "users sin email")

	badRole := validUserDoc("r@example.com")
	badRole["role"] = "superadmin"
	_, err = users.InsertOne(ctx, badRole)
	assertRejected(t, err, "users con role superadmin")

	badCount := validUserDoc("c@example.com")
	badCount["ratingCount"] = "2"
	_, err = users.InsertOne(ctx, badCount)
	assertRejected(t, err, "users con ratingCount string")

	negCount := validUserDoc("n@example.com")
	negCount["ratingCount"] = int64(-1)
	_, err = users.InsertOne(ctx, negCount)
	assertRejected(t, err, "users con ratingCount negativo")

	now := time.Now().UTC()
	_, err = db.Collection(RefreshTokensCollection).InsertOne(ctx, bson.M{
		"_id": bson.NewObjectID(), "userId": "u1", "familyId": "f1", "expiresAt": now, "createdAt": now,
	})
	assertRejected(t, err, "refresh_tokens sin tokenHash")

	// Un documento válido escrito a mano y por los repositorios pasa.
	if _, err := users.InsertOne(ctx, validUserDoc("ok@example.com")); err != nil {
		t.Errorf("insertar usuario válido: %v", err)
	}
	if _, err := NewUserRepo(db).Create(ctx, newUser("repo@example.com")); err != nil {
		t.Errorf("UserRepo.Create bajo validación: %v", err)
	}
	if err := NewRefreshRepo(db).Create(ctx, domain.RefreshToken{
		UserID: "u1", FamilyID: "f1", TokenHash: "h1", ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}); err != nil {
		t.Errorf("RefreshRepo.Create bajo validación: %v", err)
	}
}

func TestIntegrationEnsureSchemasLegacyCollections(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	users := db.Collection(UsersCollection)
	tokens := db.Collection(RefreshTokensCollection)

	// Colecciones existentes sin validador y con un documento heredado inválido.
	legacyUserID := bson.NewObjectID()
	if _, err := users.InsertOne(ctx, bson.M{"_id": legacyUserID, "email": "legacy@example.com", "role": "superadmin"}); err != nil {
		t.Fatal(err)
	}
	legacyTokID := bson.NewObjectID()
	if _, err := tokens.InsertOne(ctx, bson.M{"_id": legacyTokID, "userId": "u0"}); err != nil {
		t.Fatal(err)
	}

	res, err := EnsureSchemas(ctx, db)
	if err != nil {
		t.Fatalf("EnsureSchemas: %v", err)
	}
	if len(res.Created) != 0 || !slices.Equal(res.Updated, []string{UsersCollection, RefreshTokensCollection}) || len(res.Skipped) != 0 {
		t.Errorf("resultado = %+v", res)
	}
	assertValidated(t, db, UsersCollection, "users")
	assertValidated(t, db, RefreshTokensCollection, "refresh_tokens")

	// Los documentos heredados siguen intactos.
	for _, c := range []struct {
		col *drv.Collection
		id  bson.ObjectID
	}{{users, legacyUserID}, {tokens, legacyTokID}} {
		n, err := c.col.CountDocuments(ctx, bson.D{})
		if err != nil || n != 1 {
			t.Errorf("%s: count = %d, %v; want 1", c.col.Name(), n, err)
		}
		if err := c.col.FindOne(ctx, bson.D{{Key: "_id", Value: c.id}}).Err(); err != nil {
			t.Errorf("%s: el documento heredado desapareció: %v", c.col.Name(), err)
		}
	}
	// moderate: el documento heredado inválido aún se puede actualizar.
	if _, err := users.UpdateOne(ctx, bson.D{{Key: "_id", Value: legacyUserID}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "displayName", Value: "Heredado"}}}}); err != nil {
		t.Errorf("actualizar documento heredado con moderate: %v", err)
	}

	// Una inserción nueva inválida se rechaza; las válidas de los repos pasan.
	_, err = users.InsertOne(ctx, bson.M{"_id": bson.NewObjectID(), "email": "bad@example.com"})
	assertRejected(t, err, "users incompleto tras collMod")

	repo := NewUserRepo(db)
	u, err := repo.Create(ctx, newUser("ana@example.com"))
	if err != nil {
		t.Fatalf("UserRepo.Create: %v", err)
	}
	if err := repo.AddRating(ctx, u.ID, 5); err != nil {
		t.Errorf("AddRating: %v", err)
	}
	now := time.Now().UTC()
	if err := NewRefreshRepo(db).Create(ctx, domain.RefreshToken{
		UserID: u.ID, FamilyID: "f1", TokenHash: "h1", ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}); err != nil {
		t.Errorf("RefreshRepo.Create: %v", err)
	}
	if err := NewRefreshRepo(db).RevokeAllForUser(ctx, u.ID, now); err != nil {
		t.Errorf("RevokeAllForUser: %v", err)
	}
}
