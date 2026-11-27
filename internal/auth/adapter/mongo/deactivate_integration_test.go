//go:build integration

package mongo

import (
	"context"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// fieldNames devuelve los nombres de los campos de raíz de un documento.
func fieldNames(t *testing.T, raw bson.Raw) []string {
	t.Helper()
	els, err := raw.Elements()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range els {
		out = append(out, e.Key())
	}
	slices.Sort(out)
	return out
}

// Spec 021, T013 (CA-01, CA-13, CA-16): tras la baja, users conserva solo los
// campos de §5.2, sin null, y la marca deletionPending existe solo mientras hay
// pasos pendientes.
func TestIntegrationDeactivateLeavesOnlyKeptFields(t *testing.T) {
	ctx := context.Background()
	db := newSchemaDB(t) // validadores en modo error
	repo := NewUserRepo(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)

	phone := "+525512345678"
	until := now.Add(time.Hour)
	u := newUser("ana@example.com")
	u.Phone, u.SuspendedUntil, u.RatingSum, u.RatingCount = &phone, &until, 9, 2
	u, err := repo.Create(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	other, err := repo.Create(ctx, newUser("otra@example.com"))
	if err != nil {
		t.Fatal(err)
	}

	locked, err := repo.LockActive(ctx, u.ID)
	if err != nil || !locked {
		t.Fatalf("LockActive = %v, %v", locked, err)
	}
	if ok, err := repo.LockActive(ctx, "no-es-id"); err != nil || ok {
		t.Errorf("LockActive id inválido = %v, %v", ok, err)
	}
	if ok, err := repo.LockActive(ctx, "0123456789abcdef01234567"); err != nil || ok {
		t.Errorf("LockActive inexistente = %v, %v", ok, err)
	}
	anon := domain.AnonymizedEmail(u.ID)
	if err := repo.Deactivate(ctx, u.ID, anon, now); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	raw := rawByID(t, repo.col, u.ID)
	want := []string{"_id", "acceptedTermsAt", "createdAt", "deactivatedAt", "deletionPending", "displayName", "email", "status", "updatedAt", "version"}
	if got := fieldNames(t, raw); !slices.Equal(got, want) {
		t.Errorf("campos tras la baja = %v, want %v", got, want)
	}
	if n := nullPaths(t, raw, ""); len(n) != 0 {
		t.Errorf("null tras la baja en %v", n)
	}
	got, _ := repo.ByID(ctx, u.ID)
	if got.Email != anon || got.DisplayName != domain.DeletedDisplayName || got.IsActive() || got.DeactivatedAt == nil ||
		got.PasswordHash != "" || got.Phone != nil || got.Role != "" || got.RatingCount != 0 || got.SuspendedUntil != nil ||
		got.AcceptedTermsAt.IsZero() || got.Version != u.Version+2 { // cerrojo + baja
		t.Errorf("cuenta tras la baja = %+v", got)
	}

	// Otra baja, o una cuenta que no existe, no es una baja nueva.
	if err := repo.Deactivate(ctx, u.ID, anon, now); !domain.Is(err, domain.ErrUserNotFound) {
		t.Errorf("segunda Deactivate = %v, want ErrUserNotFound", err)
	}
	if ok, _ := repo.LockActive(ctx, u.ID); ok {
		t.Error("LockActive sobre una cuenta ya dada de baja debe ser false")
	}
	// La cuenta de otra persona no cambia.
	if o, _ := repo.ByID(ctx, other.ID); !o.IsActive() || o.PasswordHash == "" || o.Version != other.Version {
		t.Errorf("otra cuenta cambió: %+v", o)
	}

	// deletionPending: aparece en la lista y se quita con $unset, nunca null.
	pend, err := repo.ListPending(ctx, 10)
	if err != nil || !slices.Equal(pend, []string{u.ID}) {
		t.Fatalf("ListPending = %v, %v", pend, err)
	}
	if ok, err := repo.ClearIfDone(ctx, "no-es-id", now); err != nil || ok {
		t.Errorf("ClearIfDone id inválido = %v, %v", ok, err)
	}
	for i, wantCleared := range []bool{true, false} {
		ok, err := repo.ClearIfDone(ctx, u.ID, now)
		if err != nil || ok != wantCleared {
			t.Errorf("ClearIfDone #%d = %v, %v, want %v", i+1, ok, err, wantCleared)
		}
	}
	raw = rawByID(t, repo.col, u.ID)
	if _, err := raw.LookupErr("deletionPending"); err == nil {
		t.Error("ClearIfDone debe quitar el campo deletionPending")
	}
	if n := nullPaths(t, raw, ""); len(n) != 0 {
		t.Errorf("null tras ClearIfDone en %v", n)
	}
	if pend, _ := repo.ListPending(ctx, 10); len(pend) != 0 {
		t.Errorf("ListPending tras limpiar = %v", pend)
	}

	// CA-16: el correo original vuelve a registrarse.
	if _, err := repo.Create(ctx, newUser("ana@example.com")); err != nil {
		t.Errorf("el correo original debe quedar libre: %v", err)
	}
}

// AddRating y Suspend sobre una cuenta eliminada no hacen nada, no son error y
// no recrean campos; sobre una inexistente siguen devolviendo ErrUserNotFound.
func TestIntegrationAdminOpsSkipDeactivatedAccounts(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepo(newSchemaDB(t))
	now := time.Now().UTC().Truncate(time.Millisecond)
	u, err := repo.Create(ctx, newUser("ana@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Deactivate(ctx, u.ID, domain.AnonymizedEmail(u.ID), now); err != nil {
		t.Fatal(err)
	}
	before := rawByID(t, repo.col, u.ID)

	if err := repo.AddRating(ctx, u.ID, 5); err != nil {
		t.Errorf("AddRating sobre cuenta eliminada = %v, want nil", err)
	}
	if err := repo.Suspend(ctx, u.ID, now.Add(time.Hour)); err != nil {
		t.Errorf("Suspend sobre cuenta eliminada = %v, want nil", err)
	}
	after := rawByID(t, repo.col, u.ID)
	if !slices.Equal(before, after) {
		t.Errorf("la cuenta eliminada cambió: antes %v, después %v", fieldNames(t, before), fieldNames(t, after))
	}
	for _, f := range []string{"ratingSum", "ratingCount", "suspendedUntil"} {
		if _, err := after.LookupErr(f); err == nil {
			t.Errorf("%s fue recreado", f)
		}
	}
	if err := repo.AddRating(ctx, "0123456789abcdef01234567", 5); !domain.Is(err, domain.ErrUserNotFound) {
		t.Errorf("AddRating inexistente = %v", err)
	}
	if err := repo.Suspend(ctx, "0123456789abcdef01234567", now); !domain.Is(err, domain.ErrUserNotFound) {
		t.Errorf("Suspend inexistente = %v", err)
	}
}

// PublicUsers de una cuenta eliminada: «Cuenta eliminada», sin promedio y con
// cuenta 0, también con un documento anterior que aún conserve las sumas.
func TestIntegrationPublicUsersOfDeletedAccount(t *testing.T) {
	ctx := context.Background()
	db := newSchemaDB(t)
	repo := NewUserRepo(db)
	now := time.Now().UTC().Truncate(time.Millisecond)
	u := newUser("ana@example.com")
	u.RatingSum, u.RatingCount = 9, 2
	u, err := repo.Create(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Deactivate(ctx, u.ID, domain.AnonymizedEmail(u.ID), now); err != nil {
		t.Fatal(err)
	}
	// Documento de una baja anterior a la spec 021: conserva las sumas.
	legacy := bson.D{
		{Key: "_id", Value: bson.NewObjectID()}, {Key: "email", Value: "deleted+old@vaiven.invalid"}, {Key: "passwordHash", Value: "h"},
		{Key: "displayName", Value: "Cuenta eliminada"}, {Key: "role", Value: "user"}, {Key: "status", Value: "deactivated"},
		{Key: "acceptedTermsAt", Value: now}, {Key: "ratingSum", Value: int64(9)}, {Key: "ratingCount", Value: int64(2)},
		{Key: "createdAt", Value: now}, {Key: "updatedAt", Value: now}, {Key: "version", Value: int64(2)}, {Key: "deactivatedAt", Value: now},
	}
	if _, err := repo.col.InsertOne(ctx, legacy); err != nil {
		t.Fatalf("una baja anterior debe seguir cumpliendo el esquema: %v", err)
	}
	legacyID := legacy[0].Value.(bson.ObjectID).Hex()

	dir, err := repo.PublicUsers(ctx, []string{u.ID, legacyID})
	if err != nil || len(dir) != 2 {
		t.Fatalf("PublicUsers = %+v, %v", dir, err)
	}
	for _, id := range []string{u.ID, legacyID} {
		p := dir[id]
		if p.DisplayName != domain.DeletedDisplayName || p.RatingAverage != nil || p.RatingCount != 0 || p.MemberSince.IsZero() {
			t.Errorf("perfil de %s = %+v", id, p)
		}
	}
}

// El esquema condicional por estado (spec 021, plan §5.2): active exige el
// conjunto completo; deactivated solo el mínimo; deletionPending nunca es null.
func TestIntegrationUsersSchemaIsConditional(t *testing.T) {
	ctx := context.Background()
	db := newSchemaDB(t)
	users := db.Collection(UsersCollection)
	now := time.Now().UTC()
	minimal := func() bson.M {
		return bson.M{
			"_id": bson.NewObjectID(), "email": "deleted+x@vaiven.invalid", "displayName": "Cuenta eliminada",
			"status": "deactivated", "deactivatedAt": now, "acceptedTermsAt": now,
			"createdAt": now, "updatedAt": now, "version": int64(2),
		}
	}
	if _, err := users.InsertOne(ctx, minimal()); err != nil {
		t.Errorf("deactivated mínimo debe pasar: %v", err)
	}
	withMark := minimal()
	withMark["email"], withMark["deletionPending"] = "deleted+y@vaiven.invalid", true
	if _, err := users.InsertOne(ctx, withMark); err != nil {
		t.Errorf("deactivated con deletionPending: true debe pasar: %v", err)
	}
	nullMark := minimal()
	nullMark["email"], nullMark["deletionPending"] = "deleted+z@vaiven.invalid", nil
	_, err := users.InsertOne(ctx, nullMark)
	assertRejected(t, err, "deletionPending null")
	noDeactivatedAt := minimal()
	noDeactivatedAt["email"] = "deleted+w@vaiven.invalid"
	delete(noDeactivatedAt, "deactivatedAt")
	_, err = users.InsertOne(ctx, noDeactivatedAt)
	assertRejected(t, err, "deactivated sin deactivatedAt")

	active := validUserDoc("activa@example.com")
	delete(active, "passwordHash")
	_, err = users.InsertOne(ctx, active)
	assertRejected(t, err, "active sin passwordHash")
	activeNoRole := validUserDoc("activa2@example.com")
	delete(activeNoRole, "role")
	_, err = users.InsertOne(ctx, activeNoRole)
	assertRejected(t, err, "active sin role")
	if _, err := users.InsertOne(ctx, validUserDoc("activa3@example.com")); err != nil {
		t.Errorf("active completa debe pasar: %v", err)
	}
}
