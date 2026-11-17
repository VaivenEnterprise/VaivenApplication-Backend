//go:build integration

package mongo

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

var testClient *mongodb.Client

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "no se pudo iniciar MongoDB con Docker o Podman:", err)
		os.Exit(1)
	}
	uri, err := c.ConnectionString(ctx)
	if err == nil {
		testClient, err = mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "MongoDB:", err)
		_ = c.Terminate(ctx)
		os.Exit(1)
	}
	code := m.Run()
	_ = testClient.Disconnect(ctx)
	_ = c.Terminate(ctx)
	os.Exit(code)
}

type env struct {
	db     *drv.Database
	notifs *NotificationRepo
	tokens *TokenRepo
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testClient.Database(fmt.Sprintf("notifications_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	e := &env{db: db, notifs: NewNotificationRepo(db), tokens: NewTokenRepo(db)}
	if err := e.notifs.EnsureIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.tokens.EnsureIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	return e
}

func save(t *testing.T, e *env, user, typ string) {
	t.Helper()
	err := e.notifs.Save(context.Background(), notify.Record{
		UserID: user, Type: typ, Title: "T", Body: "B", Data: map[string]string{"bookingId": "b1"}, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIndexesIncludeTTL90Days(t *testing.T) { // RNF-01
	e := newEnv(t)
	cur, err := e.db.Collection(NotificationsCollection).Indexes().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var idx []bson.M
	if err := cur.All(context.Background(), &idx); err != nil {
		t.Fatal(err)
	}
	found := map[string]bson.M{}
	for _, i := range idx {
		found[i["name"].(string)] = i
	}
	for _, n := range []string{"user_id", "user_read", "created_ttl"} {
		if found[n] == nil {
			t.Fatalf("falta el índice %s: %v", n, found)
		}
	}
	if got := fmt.Sprint(found["created_ttl"]["expireAfterSeconds"]); got != fmt.Sprint(90*24*60*60) {
		t.Errorf("expireAfterSeconds = %s, quiero 7776000", got)
	}
	tcur, err := e.db.Collection(PushTokensCollection).Indexes().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var tidx []bson.M
	if err := tcur.All(context.Background(), &tidx); err != nil {
		t.Fatal(err)
	}
	var unique bool
	for _, i := range tidx {
		if i["name"] == "token_unique" && i["unique"] == true {
			unique = true
		}
	}
	if !unique {
		t.Errorf("falta el índice único de token: %v", tidx)
	}
}

func TestSaveListAndPaginate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		save(t, e, "u1", "booking_requested")
	}
	save(t, e, "u2", "booking_requested")

	page1, err := e.notifs.List(ctx, "u1", "", 3)
	if err != nil || len(page1) != 3 {
		t.Fatalf("página 1: %v, %d", err, len(page1))
	}
	if page1[0].ID <= page1[1].ID || page1[0].UserID != "u1" || page1[0].ReadAt != nil || page1[0].Data["bookingId"] != "b1" {
		t.Errorf("orden o contenido inesperado: %+v", page1[0])
	}
	page2, err := e.notifs.List(ctx, "u1", page1[2].ID, 3)
	if err != nil || len(page2) != 2 {
		t.Fatalf("página 2: %v, %d", err, len(page2))
	}
	if page2[0].ID >= page1[2].ID {
		t.Error("la página 2 debe empezar después del cursor")
	}
	if _, err := e.notifs.List(ctx, "u1", "no-es-id", 3); err == nil {
		t.Error("un cursor inválido debe fallar")
	}
}

func TestSaveWithNilDataStoresEmptyObject(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.notifs.Save(ctx, notify.Record{UserID: "u1", Type: "rating_received", Title: "T", Body: "B", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	got, _ := e.notifs.List(ctx, "u1", "", 1)
	if got[0].Data == nil || len(got[0].Data) != 0 {
		t.Errorf("Data = %#v, quiero mapa vacío", got[0].Data)
	}
}

func TestUnreadCountAndMarkRead(t *testing.T) { // CA-07
	e := newEnv(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		save(t, e, "u1", "message_received")
	}
	save(t, e, "u2", "message_received")

	if n, _ := e.notifs.CountUnread(ctx, "u1"); n != 3 {
		t.Fatalf("no leídos = %d, quiero 3", n)
	}
	rows, _ := e.notifs.List(ctx, "u1", "", 10)
	other, _ := e.notifs.List(ctx, "u2", "", 10)
	at := time.Now().UTC().Truncate(time.Millisecond)
	// Marca uno propio y uno ajeno: el ajeno no se toca.
	if err := e.notifs.MarkRead(ctx, "u1", []string{rows[0].ID, other[0].ID}, at); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.notifs.CountUnread(ctx, "u1"); n != 2 {
		t.Errorf("tras marcar uno, no leídos = %d, quiero 2", n)
	}
	if n, _ := e.notifs.CountUnread(ctx, "u2"); n != 1 {
		t.Errorf("el aviso ajeno no debe cambiar: %d", n)
	}
	after, _ := e.notifs.List(ctx, "u1", "", 10)
	if after[0].ReadAt == nil || !after[0].ReadAt.Equal(at) {
		t.Errorf("readAt = %v, quiero %v", after[0].ReadAt, at)
	}
	// Volver a marcar no cambia la fecha original.
	if err := e.notifs.MarkRead(ctx, "u1", []string{rows[0].ID}, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	again, _ := e.notifs.List(ctx, "u1", "", 10)
	if !again[0].ReadAt.Equal(at) {
		t.Error("readAt no debe sobrescribirse")
	}

	if err := e.notifs.MarkAllRead(ctx, "u1", at); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.notifs.CountUnread(ctx, "u1"); n != 0 {
		t.Errorf("tras read all, no leídos = %d, quiero 0", n)
	}
	if n, _ := e.notifs.CountUnread(ctx, "u2"); n != 1 {
		t.Error("read all de u1 no debe tocar a u2")
	}
	if err := e.notifs.MarkRead(ctx, "u1", []string{"malo"}, at); err == nil {
		t.Error("un id inválido debe fallar")
	}
}

func token(n int) string { return fmt.Sprintf("ExponentPushToken[device-%d]", n) }

func TestTokenUpsertReassignsToNewUser(t *testing.T) { // RF-04
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := e.tokens.Upsert(ctx, domain.PushToken{Token: token(1), UserID: "u1", Platform: "android", CreatedAt: now, LastSeenAt: now}); err != nil {
		t.Fatal(err)
	}
	later := now.Add(time.Hour)
	if err := e.tokens.Upsert(ctx, domain.PushToken{Token: token(1), UserID: "u2", Platform: "ios", CreatedAt: later, LastSeenAt: later}); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.tokens.TokensOf(ctx, "u1"); len(got) != 0 {
		t.Errorf("u1 ya no debe tener el token: %v", got)
	}
	if got, _ := e.tokens.TokensOf(ctx, "u2"); len(got) != 1 || got[0] != token(1) {
		t.Errorf("u2 debe tenerlo: %v", got)
	}
	var doc bson.M
	if err := e.db.Collection(PushTokensCollection).FindOne(ctx, bson.D{{Key: "token", Value: token(1)}}).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["platform"] != "ios" || !doc["createdAt"].(bson.DateTime).Time().Equal(now) || !doc["lastSeenAt"].(bson.DateTime).Time().Equal(later) {
		t.Errorf("platform/createdAt/lastSeenAt incorrectos: %v", doc)
	}
	if n, _ := e.db.Collection(PushTokensCollection).CountDocuments(ctx, bson.D{}); n != 1 {
		t.Errorf("debe haber un solo documento, hay %d", n)
	}
}

func TestTokenUpsertConcurrentSameToken(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- e.tokens.Upsert(ctx, domain.PushToken{Token: token(7), UserID: fmt.Sprintf("u%d", i), Platform: "ios", CreatedAt: now, LastSeenAt: now})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Upsert concurrente: %v", err)
		}
	}
	if n, _ := e.db.Collection(PushTokensCollection).CountDocuments(ctx, bson.D{}); n != 1 {
		t.Errorf("hay %d documentos, quiero 1", n)
	}
}

func TestTokenDeleteOnlyOwner(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i, u := range []string{"u1", "u2"} {
		if err := e.tokens.Upsert(ctx, domain.PushToken{Token: token(i), UserID: u, Platform: "ios", CreatedAt: now, LastSeenAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.tokens.Delete(ctx, "u1", token(1)); err != nil { // el token 1 es de u2
		t.Fatal(err)
	}
	if got, _ := e.tokens.TokensOf(ctx, "u2"); len(got) != 1 {
		t.Error("u1 no debe poder borrar el token de u2")
	}
	if err := e.tokens.Delete(ctx, "u1", token(0)); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.tokens.TokensOf(ctx, "u1"); len(got) != 0 {
		t.Error("u1 sí puede borrar el suyo")
	}
	if err := e.tokens.Delete(ctx, "u1", token(0)); err != nil {
		t.Errorf("borrar uno inexistente no es error: %v", err)
	}
}

func TestDeleteTokenAnyOwner(t *testing.T) { // CA-02
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := e.tokens.Upsert(ctx, domain.PushToken{Token: token(1), UserID: "u1", Platform: "ios", CreatedAt: now, LastSeenAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := e.tokens.DeleteToken(ctx, token(1)); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.tokens.TokensOf(ctx, "u1"); len(got) != 0 {
		t.Errorf("el token debe haberse borrado: %v", got)
	}
}
