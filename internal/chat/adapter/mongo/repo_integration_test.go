//go:build integration

package mongo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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

// Ids de prueba con forma de ObjectID (unreadField solo acepta hexadecimales).
var (
	own  = bson.NewObjectID().Hex()
	req  = bson.NewObjectID().Hex()
	otro = bson.NewObjectID().Hex()
)

func send(t *testing.T, s *Sender, conv, from, to, body string, cid *string) (domain.Message, int, error) {
	t.Helper()
	return s.Send(context.Background(), domain.Message{ConversationID: conv, SenderID: from, Body: body, ClientID: cid, CreatedAt: now0},
		to, domain.LastMessage{Preview: body, At: now0, SenderID: from})
}

func TestIntegrationUnreadCounters(t *testing.T) {
	ctx := context.Background()
	db, convs, _ := newDB(t)
	s := NewSender(db)
	c, _ := convs.Open(ctx, "b1", own, req, now0)
	c2, _ := convs.Open(ctx, "b2", req, otro, now0)
	for i := range 3 {
		m, prev, err := send(t, s, c.ID, req, own, fmt.Sprint(i), nil)
		if err != nil || prev != i || m.Seq != int64(i+1) {
			t.Fatalf("Send %d = seq %d prev %d %v", i, m.Seq, prev, err)
		}
	}
	_, _, _ = send(t, s, c2.ID, otro, req, "x", nil)
	if n, _ := convs.UnreadTotal(ctx, own); n != 3 { // CA-06
		t.Errorf("own = %d", n)
	}
	if n, _ := convs.UnreadTotal(ctx, req); n != 1 {
		t.Errorf("req = %d (solo el de c2)", n)
	}
	if n, _ := convs.UnreadTotal(ctx, otro); n != 0 {
		t.Errorf("otro = %d", n)
	}
	if err := convs.MarkRead(ctx, c.ID, own); err != nil {
		t.Fatal(err)
	}
	if n, _ := convs.UnreadTotal(ctx, own); n != 0 {
		t.Errorf("tras leer = %d", n)
	}
	got, _ := convs.ByID(ctx, c.ID)
	if got.LastMessage.Preview != "2" {
		t.Errorf("último = %+v", got.LastMessage)
	}
	// Solo ids hexadecimales en la ruta unread.<id> (observación 6).
	for _, bad := range []string{"a.b", "$x", "", "own", strings.ToUpper(own)} {
		if _, _, err := send(t, s, c.ID, req, bad, "x", nil); err == nil {
			t.Errorf("receptor %q debe rechazarse", bad)
		}
		if err := convs.MarkRead(ctx, c.ID, bad); err == nil {
			t.Errorf("MarkRead %q debe rechazarse", bad)
		}
		if _, err := convs.UnreadTotal(ctx, bad); err == nil {
			t.Errorf("UnreadTotal %q debe rechazarse", bad)
		}
	}
	if _, _, err := send(t, s, bson.NewObjectID().Hex(), req, own, "x", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("conversación inexistente = %v", err)
	}
	if _, _, err := send(t, s, "zz", req, own, "x", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Error("id inválido")
	}
	if err := convs.MarkRead(ctx, "zz", own); !errors.Is(err, domain.ErrNotFound) {
		t.Error("MarkRead id inválido")
	}

	// Listado por último mensaje.
	list, err := convs.ListByParticipant(ctx, req, nil, 1)
	if err != nil || len(list) != 1 || list[0].ID != c2.ID {
		t.Fatalf("primera página = %+v %v", list, err)
	}
	next, _ := convs.ListByParticipant(ctx, req, &domain.Cursor{At: list[0].LastMessageAt, ID: list[0].ID}, 5)
	if len(next) != 1 || next[0].ID != c.ID {
		t.Errorf("segunda página = %+v", next)
	}
	if _, err := convs.ListByParticipant(ctx, req, &domain.Cursor{ID: "x"}, 5); err == nil {
		t.Error("cursor inválido")
	}
}

// Observaciones 1 y 2: envíos simultáneos de las dos partes quedan con seq
// consecutivos, sin huecos, y cada uno suma exactamente un no leído.
func TestIntegrationConcurrentSendsHaveNoGaps(t *testing.T) {
	ctx := context.Background()
	db, convs, msgs := newDB(t)
	s := NewSender(db)
	c, _ := convs.Open(ctx, "b1", own, req, now0)
	const perSide = 15
	var wg sync.WaitGroup
	for i := range 2 * perSide {
		from, to := req, own
		if i%2 == 1 {
			from, to = own, req
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := send(t, s, c.ID, from, to, fmt.Sprint(i), nil); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	all, err := msgs.Before(ctx, c.ID, "", 100)
	if err != nil || len(all) != 2*perSide {
		t.Fatalf("mensajes = %d %v", len(all), err)
	}
	for i, m := range all { // descendente: 30, 29, …, 1
		if m.Seq != int64(2*perSide-i) {
			t.Fatalf("hueco o repetido en seq: posición %d tiene %d", i, m.Seq)
		}
	}
	got, _ := convs.ByID(ctx, c.ID)
	if got.UnreadFor(own) != perSide || got.UnreadFor(req) != perSide {
		t.Errorf("no leídos = %v", got.Unread)
	}
	// after desde cualquier mensaje devuelve exactamente los siguientes.
	mid := all[perSide] // seq 15
	after, _ := msgs.After(ctx, c.ID, mid.ID, 100)
	if len(after) != perSide || after[0].Seq != mid.Seq+1 {
		t.Errorf("after = %d desde seq %d", len(after), mid.Seq)
	}
}

func TestIntegrationMessagesPaging(t *testing.T) {
	ctx := context.Background()
	db, convs, msgs := newDB(t)
	s := NewSender(db)
	c, _ := convs.Open(ctx, "b1", own, req, now0)
	c2, _ := convs.Open(ctx, "b2", own, req, now0)
	var ids []string
	for i := range 7 {
		m, _, err := send(t, s, c.ID, own, req, fmt.Sprint(i), nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	_, _, _ = send(t, s, c2.ID, own, req, "otra", nil)

	// before: descendente, sin huecos ni repetidos.
	var seen []string
	before := ""
	for range 5 {
		page, err := msgs.Before(ctx, c.ID, before, 3)
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
	after, _ := msgs.After(ctx, c.ID, ids[3], 100)
	if len(after) != 3 || after[0].Body != "4" || after[2].Body != "6" {
		t.Errorf("after = %+v", after)
	}
	if got, _ := msgs.After(ctx, c.ID, ids[6], 100); len(got) != 0 {
		t.Error("nada nuevo")
	}
	// Un cursor que no es de esta conversación (o inválido) se rechaza.
	otherConv, _ := msgs.Before(ctx, c2.ID, "", 1)
	for _, cur := range []string{"zz", bson.NewObjectID().Hex(), otherConv[0].ID} {
		if _, err := msgs.Before(ctx, c.ID, cur, 3); !errors.Is(err, domain.ErrUnknownMessage) {
			t.Errorf("before %s = %v", cur, err)
		}
		if _, err := msgs.After(ctx, c.ID, cur, 3); !errors.Is(err, domain.ErrUnknownMessage) {
			t.Errorf("after %s = %v", cur, err)
		}
	}
}

func TestIntegrationClientIDIdempotency(t *testing.T) {
	ctx := context.Background()
	db, convs, msgs := newDB(t)
	s := NewSender(db)
	c, _ := convs.Open(ctx, "b1", own, req, now0)
	c2, _ := convs.Open(ctx, "b2", own, req, now0)
	cid := "abc"
	m1, _, err := send(t, s, c.ID, own, req, "hola", &cid)
	if err != nil {
		t.Fatal(err)
	}
	// Repetido: aborta la transacción, no suma no leídos ni cambia seq.
	if _, _, err := send(t, s, c.ID, own, req, "hola", &cid); !errors.Is(err, domain.ErrDuplicateClientID) {
		t.Errorf("repetido = %v", err)
	}
	got, _ := convs.ByID(ctx, c.ID)
	if got.UnreadFor(req) != 1 {
		t.Errorf("el repetido no debe sumar no leídos: %v", got.Unread)
	}
	// Mismo clientId de otra persona u otra conversación: permitido. Sin clientId: no choca.
	for _, x := range []struct {
		conv, from, to string
		cid            *string
	}{{c.ID, req, own, &cid}, {c2.ID, own, req, &cid}, {c.ID, own, req, nil}, {c.ID, own, req, nil}} {
		if _, _, err := send(t, s, x.conv, x.from, x.to, "x", x.cid); err != nil {
			t.Errorf("%+v = %v", x, err)
		}
	}
	m, _, _ := send(t, s, c.ID, own, req, "x", nil)
	if m.Seq != 5 {
		t.Errorf("seq tras el repetido = %d, want 5 (sin hueco)", m.Seq)
	}
	found, ok, err := msgs.ByClientID(ctx, c.ID, own, "abc")
	if err != nil || !ok || found.ID != m1.ID || *found.ClientID != "abc" {
		t.Errorf("ByClientID = %+v %v %v", found, ok, err)
	}
	if _, ok, _ := msgs.ByClientID(ctx, c.ID, own, "zzz"); ok {
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
