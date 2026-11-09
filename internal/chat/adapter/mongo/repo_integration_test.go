//go:build integration

package mongo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
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

var now0 = time.Date(2026, 11, 9, 17, 0, 0, 0, time.UTC)

func newDB(t *testing.T) (*drv.Database, *ConversationRepo, *MessageRepo) {
	t.Helper()
	db := testClient.Database(fmt.Sprintf("chat_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	convs, msgs := NewConversationRepo(db), NewMessageRepo(db)
	for _, e := range []interface{ EnsureIndexes(context.Context) error }{convs, msgs, convs, msgs} { // idempotente
		if err := e.EnsureIndexes(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	return db, convs, msgs
}

func TestIntegrationOpenIdempotentCA01(t *testing.T) {
	ctx := context.Background()
	_, convs, _ := newDB(t)
	var wg sync.WaitGroup
	ids := make([]string, 5)
	for i := range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := convs.Open(ctx, "b1", "own", "req", now0)
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = c.ID
		}()
	}
	wg.Wait()
	for _, id := range ids[1:] {
		if id != ids[0] {
			t.Fatalf("ids distintos: %v", ids)
		}
	}
	c, err := convs.ByID(ctx, ids[0])
	if err != nil || c.Participants != [2]string{"own", "req"} || c.BookingID != "b1" || c.LastMessage != nil || len(c.Unread) != 0 {
		t.Errorf("conversación = %+v %v", c, err)
	}
	for _, id := range []string{"zz", bson.NewObjectID().Hex()} {
		if _, err := convs.ByID(ctx, id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ByID(%s) = %v", id, err)
		}
	}
}

func TestIntegrationUnreadCounters(t *testing.T) {
	ctx := context.Background()
	_, convs, _ := newDB(t)
	c, _ := convs.Open(ctx, "b1", "own", "req", now0)
	c2, _ := convs.Open(ctx, "b2", "req", "otro", now0)
	for i := range 3 {
		prev, err := convs.RecordMessage(ctx, c.ID, "own", domain.LastMessage{Preview: fmt.Sprint(i), At: now0.Add(time.Duration(i) * time.Second), SenderID: "req"})
		if err != nil || prev != i {
			t.Fatalf("RecordMessage %d = %d %v", i, prev, err)
		}
	}
	_, _ = convs.RecordMessage(ctx, c2.ID, "req", domain.LastMessage{Preview: "x", At: now0.Add(time.Minute), SenderID: "otro"})
	if n, _ := convs.UnreadTotal(ctx, "own"); n != 3 { // CA-06
		t.Errorf("own = %d", n)
	}
	if n, _ := convs.UnreadTotal(ctx, "req"); n != 1 {
		t.Errorf("req = %d (solo el de c2)", n)
	}
	if n, _ := convs.UnreadTotal(ctx, "nadie"); n != 0 {
		t.Errorf("nadie = %d", n)
	}
	if err := convs.MarkRead(ctx, c.ID, "own"); err != nil {
		t.Fatal(err)
	}
	if n, _ := convs.UnreadTotal(ctx, "own"); n != 0 {
		t.Errorf("tras leer = %d", n)
	}
	got, _ := convs.ByID(ctx, c.ID)
	if got.LastMessage.Preview != "2" || !got.LastMessageAt.Equal(now0.Add(2*time.Second)) {
		t.Errorf("último = %+v", got.LastMessage)
	}
	if _, err := convs.RecordMessage(ctx, c.ID, "a.b", domain.LastMessage{}); err == nil {
		t.Error("id con punto")
	}
	if err := convs.MarkRead(ctx, c.ID, "$x"); err == nil {
		t.Error("id con $")
	}
	if _, err := convs.UnreadTotal(ctx, ""); err == nil {
		t.Error("id vacío")
	}
	if _, err := convs.RecordMessage(ctx, bson.NewObjectID().Hex(), "own", domain.LastMessage{}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("inexistente = %v", err)
	}
	if _, err := convs.RecordMessage(ctx, "zz", "own", domain.LastMessage{}); !errors.Is(err, domain.ErrNotFound) {
		t.Error("id inválido")
	}
	if err := convs.MarkRead(ctx, "zz", "own"); !errors.Is(err, domain.ErrNotFound) {
		t.Error("MarkRead id inválido")
	}

	// Listado por último mensaje.
	list, err := convs.ListByParticipant(ctx, "req", nil, 1)
	if err != nil || len(list) != 1 || list[0].ID != c2.ID {
		t.Fatalf("primera página = %+v %v", list, err)
	}
	next, _ := convs.ListByParticipant(ctx, "req", &domain.Cursor{At: list[0].LastMessageAt, ID: list[0].ID}, 5)
	if len(next) != 1 || next[0].ID != c.ID {
		t.Errorf("segunda página = %+v", next)
	}
	if _, err := convs.ListByParticipant(ctx, "req", &domain.Cursor{ID: "x"}, 5); err == nil {
		t.Error("cursor inválido")
	}
}

func TestIntegrationMessagesPaging(t *testing.T) {
	ctx := context.Background()
	_, _, msgs := newDB(t)
	var ids []string
	for i := range 7 {
		m, err := msgs.Insert(ctx, domain.Message{ConversationID: "c1", SenderID: "own", Body: fmt.Sprint(i), CreatedAt: now0})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	_, _ = msgs.Insert(ctx, domain.Message{ConversationID: "c2", SenderID: "own", Body: "otra"})

	// before: descendente, sin huecos ni repetidos.
	var seen []string
	before := ""
	for range 5 {
		page, err := msgs.Before(ctx, "c1", before, 3)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page {
			seen = append(seen, m.Body)
		}
		if len(page) < 3 {
			break
		}
		before = page[len(page)-1].ID
	}
	if fmt.Sprint(seen) != "[6 5 4 3 2 1 0]" {
		t.Errorf("before = %v", seen)
	}
	// after: ascendente.
	after, _ := msgs.After(ctx, "c1", ids[3], 100)
	if len(after) != 3 || after[0].Body != "4" || after[2].Body != "6" {
		t.Errorf("after = %+v", after)
	}
	if got, _ := msgs.After(ctx, "c1", ids[6], 100); len(got) != 0 {
		t.Error("nada nuevo")
	}
	for _, f := range []func() error{
		func() error { _, err := msgs.Before(ctx, "c1", "zz", 3); return err },
		func() error { _, err := msgs.After(ctx, "c1", "zz", 3); return err },
	} {
		if f() == nil {
			t.Error("cursor inválido")
		}
	}
}

func TestIntegrationClientIDIdempotency(t *testing.T) {
	ctx := context.Background()
	_, _, msgs := newDB(t)
	cid := "abc"
	m1, err := msgs.Insert(ctx, domain.Message{ConversationID: "c1", SenderID: "own", Body: "hola", ClientID: &cid, CreatedAt: now0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := msgs.Insert(ctx, domain.Message{ConversationID: "c1", SenderID: "own", Body: "hola", ClientID: &cid}); !errors.Is(err, domain.ErrDuplicateClientID) {
		t.Errorf("repetido = %v", err)
	}
	// Mismo clientId de otra persona u otra conversación: permitido. Sin clientId: no choca.
	for _, m := range []domain.Message{
		{ConversationID: "c1", SenderID: "req", Body: "x", ClientID: &cid},
		{ConversationID: "c2", SenderID: "own", Body: "x", ClientID: &cid},
		{ConversationID: "c1", SenderID: "own", Body: "x"},
		{ConversationID: "c1", SenderID: "own", Body: "y"},
	} {
		if _, err := msgs.Insert(ctx, m); err != nil {
			t.Errorf("%+v = %v", m, err)
		}
	}
	got, ok, err := msgs.ByClientID(ctx, "c1", "own", "abc")
	if err != nil || !ok || got.ID != m1.ID || *got.ClientID != "abc" {
		t.Errorf("ByClientID = %+v %v %v", got, ok, err)
	}
	if _, ok, _ := msgs.ByClientID(ctx, "c1", "own", "zzz"); ok {
		t.Error("inexistente")
	}
}

func TestIntegrationReaders(t *testing.T) {
	ctx := context.Background()
	db, _, _ := newDB(t)
	res, _ := db.Collection(BookingsCollection).InsertOne(ctx, bson.M{
		"status": "accepted", "ownerId": "own", "requesterId": "req", "itemId": "i1",
		"itemSnapshot":  bson.M{"title": "Taladro", "firstPhoto": bson.M{"publicId": "p1", "format": "jpg", "delivery": "public"}},
		"priceSnapshot": bson.M{"totalCents": 9000},
	})
	bid := res.InsertedID.(bson.ObjectID).Hex()
	infos, err := NewBookingReader(db).ByIDs(ctx, []string{bid, "zz", bson.NewObjectID().Hex()})
	if err != nil || len(infos) != 1 || infos[bid].Status != "accepted" || infos[bid].ItemTitle != "Taladro" || infos[bid].ItemPhoto.PublicID != "p1" {
		t.Errorf("reservas = %+v %v", infos, err)
	}
	if m, _ := NewBookingReader(db).ByIDs(ctx, []string{"zz"}); len(m) != 0 {
		t.Error("ids inválidos")
	}
	a, _ := db.Collection(UsersCollection).InsertOne(ctx, bson.M{"status": "active"})
	d, _ := db.Collection(UsersCollection).InsertOne(ctx, bson.M{"status": "deactivated"})
	aid, did := a.InsertedID.(bson.ObjectID).Hex(), d.InsertedID.(bson.ObjectID).Hex()
	act, err := NewAccountReader(db).Active(ctx, []string{aid, did, "zz"})
	if err != nil || !act[aid] || act[did] {
		t.Errorf("cuentas = %v %v", act, err)
	}
	if m, _ := NewAccountReader(db).Active(ctx, nil); len(m) != 0 {
		t.Error("vacío")
	}
}
