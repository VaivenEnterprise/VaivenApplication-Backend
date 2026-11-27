//go:build integration

package mongo

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	authdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

func (e *env) rawRating(t *testing.T, id string) bson.Raw {
	t.Helper()
	oid, _ := bson.ObjectIDFromHex(id)
	var raw bson.Raw
	if err := e.repo.col.FindOne(context.Background(), bson.D{{Key: "_id", Value: oid}}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

// Spec 021, T017 (D-3, CA-08): se quita el comentario de lo que escribió la
// persona; las estrellas y los promedios ajenos no cambian; lo que recibió, lo
// que escribieron otras personas y los perfiles quedan intactos.
func TestIntegrationEraserRemovesOnlyAuthoredComments(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	ana, beto, carla := e.seedUser(t, "Ana"), e.seedUser(t, "Beto"), e.seedUser(t, "Carla")
	er := NewEraser(e.db)

	given1, err := e.repo.Create(ctx, e.rating("b1", ana, beto, 5))
	if err != nil {
		t.Fatal(err)
	}
	given2, err := e.repo.Create(ctx, e.rating("b2", ana, carla, 3))
	if err != nil {
		t.Fatal(err)
	}
	received, err := e.repo.Create(ctx, e.rating("b3", carla, ana, 4))
	if err != nil {
		t.Fatal(err)
	}
	// Una de Ana sin comentario desde el principio: no se toca.
	noComment := e.rating("b4", ana, beto, 2)
	noComment.Comment = nil
	given3, err := e.repo.Create(ctx, noComment)
	if err != nil {
		t.Fatal(err)
	}
	betoSum, betoCount := e.profile(t, beto)
	carlaSum, carlaCount := e.profile(t, carla)
	anaSum, anaCount := e.profile(t, ana)
	receivedBefore := e.rawRating(t, received.ID)
	untouched := e.rawRating(t, given3.ID)

	at := time.Now().UTC().Truncate(time.Millisecond).Add(time.Hour)
	res, err := er.Erase(ctx, ana, at)
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts["ratings.commentsRemoved"] != 2 {
		t.Fatalf("resultado = %+v", res)
	}
	for _, id := range []string{given1.ID, given2.ID} {
		raw := e.rawRating(t, id)
		if _, err := raw.LookupErr("comment"); err == nil {
			t.Errorf("%s: el campo comment debe desaparecer", id)
		}
		if raw.Lookup("score").Int32() == 0 || raw.Lookup("raterId").StringValue() != ana {
			t.Errorf("%s: las estrellas y los ids se conservan: %v", id, raw)
		}
		if got := raw.Lookup("updatedAt").Time().UTC(); !got.Equal(at) {
			t.Errorf("%s: updatedAt = %v, want %v", id, got, at)
		}
		for _, el := range mustElements(t, raw) {
			if el.Value().Type == bson.TypeNull {
				t.Errorf("%s: %s es null", id, el.Key())
			}
		}
	}
	if got := e.rawRating(t, received.ID); string(got) != string(receivedBefore) {
		t.Error("lo que Ana recibió no debe cambiar")
	}
	if got := e.rawRating(t, given3.ID); string(got) != string(untouched) {
		t.Error("una calificación sin comentario no debe cambiar")
	}
	// CA-08: promedios ajenos (y el propio) sin cambios.
	for name, want := range map[string][2]int64{beto: {betoSum, betoCount}, carla: {carlaSum, carlaCount}, ana: {anaSum, anaCount}} {
		if s, c := e.profile(t, name); s != want[0] || c != want[1] {
			t.Errorf("perfil %s = %d/%d, want %d/%d", name, s, c, want[0], want[1])
		}
	}

	again, err := er.Erase(ctx, ana, at.Add(time.Minute))
	if err != nil || again.Counts["ratings.commentsRemoved"] != 0 {
		t.Errorf("segunda ejecución = %+v, %v", again, err)
	}
	if er.Name() != "ratings" {
		t.Errorf("Name = %q", er.Name())
	}
}

func mustElements(t *testing.T, raw bson.Raw) []bson.RawElement {
	t.Helper()
	els, err := raw.Elements()
	if err != nil {
		t.Fatal(err)
	}
	return els
}

// El índice {raterId} existe y el borrado de comentarios no recorre la colección.
func TestIntegrationRaterIndexServesTheEraser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i := 0; i < 30; i++ {
		if _, err := e.repo.Create(ctx, e.rating("b"+string(rune('a'+i)), e.seedUser(t, "x"), e.seedUser(t, "y"), 4)); err != nil {
			t.Fatal(err)
		}
	}
	var plan bson.M
	err := e.db.RunCommand(ctx, bson.D{{Key: "explain", Value: bson.D{
		{Key: "find", Value: RatingsCollection}, {Key: "filter", Value: bson.D{{Key: "raterId", Value: "x"}}},
	}}}).Decode(&plan)
	if err != nil {
		t.Fatal(err)
	}
	if !containsIndexName(plan, "rater") {
		t.Errorf("el plan no usa el índice rater: %v", plan["queryPlanner"])
	}
}

func containsIndexName(plan bson.M, name string) bool {
	js, err := bson.MarshalExtJSON(plan, false, false)
	return err == nil && strings.Contains(string(js), `"indexName":"`+name+`"`)
}

// Una calificación posterior a una cuenta eliminada se guarda en ratings, pero
// no recrea campos en users (el promedio de esa cuenta ya no se muestra).
func TestIntegrationRatingAfterDeletionDoesNotRecreateUserFields(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	ana, beto := e.seedUser(t, "Ana"), e.seedUser(t, "Beto")
	if err := e.users.Deactivate(ctx, ana, authdomain.AnonymizedEmail(ana), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	saved, err := e.repo.Create(ctx, e.rating("b1", beto, ana, 5))
	if err != nil {
		t.Fatalf("calificar a una cuenta eliminada: %v", err)
	}
	if got := e.rawRating(t, saved.ID); got.Lookup("score").Int32() != 5 {
		t.Errorf("la estrella se guarda: %v", got)
	}
	oid, _ := bson.ObjectIDFromHex(ana)
	var raw bson.Raw
	if err := e.db.Collection(authmongo.UsersCollection).FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ratingSum", "ratingCount"} {
		if _, err := raw.LookupErr(f); err == nil {
			t.Errorf("%s fue recreado en la cuenta eliminada", f)
		}
	}
}
