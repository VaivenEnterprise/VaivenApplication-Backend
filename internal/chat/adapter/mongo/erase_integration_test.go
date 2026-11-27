//go:build integration

package mongo

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
)

// Los ids de usuario del chat son ObjectID en hexadecimal.
const (
	uAna   = "aaaaaaaaaaaaaaaaaaaaaaaa"
	uBeto  = "bbbbbbbbbbbbbbbbbbbbbbbb"
	uCarla = "cccccccccccccccccccccccc"
)

func at(n int) time.Time { return now0.Add(time.Duration(n) * time.Minute) }

func sendAt(t *testing.T, s *Sender, conv, from, to, body string, when time.Time) domain.Message {
	t.Helper()
	m, _, err := s.Send(context.Background(), domain.Message{ConversationID: conv, SenderID: from, Body: body, CreatedAt: when},
		to, domain.LastMessage{Preview: body, At: when, SenderID: from})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Spec 021, T016 (D-1, CA-07): el texto de la persona pasa a «Mensaje eliminado»;
// la contraparte ve el historial completo, en orden y con las mismas fechas, y
// lo de otras conversaciones no cambia.
func TestIntegrationEraserReplacesMessageText(t *testing.T) {
	ctx := context.Background()
	db, convs, msgs := newDB(t)
	sender := NewSender(db)
	er := NewEraser(db)

	c1, err := convs.Open(ctx, "b1", uAna, uBeto, now0)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := convs.Open(ctx, "b2", uAna, uCarla, now0)
	if err != nil {
		t.Fatal(err)
	}
	c3, err := convs.Open(ctx, "b3", uBeto, uCarla, now0) // ajena a Ana
	if err != nil {
		t.Fatal(err)
	}
	// c1: ana, beto, ana (el último es de Ana).
	m1 := sendAt(t, sender, c1.ID, uAna, uBeto, "hola, soy Ana y mi tel es 5512345678", at(1))
	m2 := sendAt(t, sender, c1.ID, uBeto, uAna, "hola Ana", at(2))
	m3 := sendAt(t, sender, c1.ID, uAna, uBeto, "nos vemos en mi casa", at(3))
	// c2: carla escribe al final (el último NO es de Ana).
	sendAt(t, sender, c2.ID, uAna, uCarla, "¿sigue disponible?", at(4))
	sendAt(t, sender, c2.ID, uCarla, uAna, "sí", at(5))
	// c3: ajena.
	sendAt(t, sender, c3.ID, uBeto, uCarla, "mensaje ajeno", at(6))

	before3, _ := msgs.Before(ctx, c3.ID, "", 10)
	conv3Before, _ := convs.ByID(ctx, c3.ID)

	res, err := er.Erase(ctx, uAna, at(10))
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts["messages.erased"] != 3 || res.Counts["conversations.previewErased"] != 1 || len(res.Images)+len(res.Notices) != 0 {
		t.Fatalf("resultado = %+v", res)
	}

	got, err := msgs.Before(ctx, c1.ID, "", 10) // del más nuevo al más viejo
	if err != nil || len(got) != 3 {
		t.Fatalf("historial = %+v, %v", got, err)
	}
	want := []struct {
		id, sender, body string
		seq              int64
		when             time.Time
	}{
		{m3.ID, uAna, DeletedMessageText, 3, at(3)},
		{m2.ID, uBeto, "hola Ana", 2, at(2)},
		{m1.ID, uAna, DeletedMessageText, 1, at(1)},
	}
	for i, w := range want {
		g := got[i]
		if g.ID != w.id || g.SenderID != w.sender || g.Body != w.body || g.Seq != w.seq || !g.CreatedAt.Equal(w.when) {
			t.Errorf("mensaje %d = %+v, want %+v", i, g, w)
		}
	}
	// Extracto: era de Ana → reemplazado; fecha y autor se conservan.
	cv1, _ := convs.ByID(ctx, c1.ID)
	if cv1.LastMessage == nil || cv1.LastMessage.Preview != DeletedMessageText || cv1.LastMessage.SenderID != uAna || !cv1.LastMessage.At.Equal(at(3)) || !cv1.LastMessageAt.Equal(at(3)) {
		t.Errorf("extracto c1 = %+v", cv1.LastMessage)
	}
	if cv1.Unread[uBeto] != 2 {
		t.Errorf("los no leídos no cambian: %v", cv1.Unread)
	}
	// c2: el último mensaje es de Carla, su extracto se conserva; el de Ana sí se borra.
	cv2, _ := convs.ByID(ctx, c2.ID)
	if cv2.LastMessage == nil || cv2.LastMessage.Preview != "sí" {
		t.Errorf("extracto c2 = %+v", cv2.LastMessage)
	}
	g2, _ := msgs.Before(ctx, c2.ID, "", 10)
	if len(g2) != 2 || g2[0].Body != "sí" || g2[1].Body != DeletedMessageText {
		t.Errorf("c2 = %+v", g2)
	}
	// Ajenas intactas.
	g3, _ := msgs.Before(ctx, c3.ID, "", 10)
	conv3After, _ := convs.ByID(ctx, c3.ID)
	if len(g3) != 1 || g3[0] != before3[0] || *conv3After.LastMessage != *conv3Before.LastMessage {
		t.Errorf("la conversación ajena cambió: %+v", g3)
	}

	// Sin nulls y sin cambios en una segunda pasada.
	again, err := er.Erase(ctx, uAna, at(11))
	if err != nil || again.Counts["messages.erased"] != 0 || again.Counts["conversations.previewErased"] != 0 {
		t.Errorf("segunda ejecución = %+v, %v", again, err)
	}
	if none, err := er.Erase(ctx, "nadie", at(11)); err != nil || len(none.Counts) != 0 {
		t.Errorf("sin conversaciones = %+v, %v", none, err)
	}
	if er.Name() != "chat" {
		t.Errorf("Name = %q", er.Name())
	}
}

// El chat con una cuenta eliminada queda de solo lectura: AccountReader no la
// ve activa (el usecase responde 409 «La conversación está cerrada.», RF-08),
// y el historial se sigue leyendo.
func TestIntegrationDeletedAccountClosesConversation(t *testing.T) {
	ctx := context.Background()
	db, convs, msgs := newDB(t)
	now := time.Now().UTC()
	ana, beto := bson.NewObjectID(), bson.NewObjectID()
	for _, u := range []bson.M{
		{"_id": ana, "status": "deactivated", "email": "deleted+x@vaiven.invalid", "displayName": "Cuenta eliminada"},
		{"_id": beto, "status": "active", "email": "beto@example.com", "displayName": "Beto"},
	} {
		if _, err := db.Collection(UsersCollection).InsertOne(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	c, err := convs.Open(ctx, "b1", ana.Hex(), beto.Hex(), now)
	if err != nil {
		t.Fatal(err)
	}
	sendAt(t, NewSender(db), c.ID, ana.Hex(), beto.Hex(), "hola", at(1))
	if _, err := NewEraser(db).Erase(ctx, ana.Hex(), now); err != nil {
		t.Fatal(err)
	}
	active, err := NewAccountReader(db).Active(ctx, []string{ana.Hex(), beto.Hex()})
	if err != nil || active[ana.Hex()] || !active[beto.Hex()] {
		t.Errorf("Active = %v, %v", active, err)
	}
	if h, err := msgs.Before(ctx, c.ID, "", 10); err != nil || len(h) != 1 || h[0].Body != DeletedMessageText {
		t.Errorf("la contraparte sigue leyendo el historial: %+v, %v", h, err)
	}
}

// La baja revierte con la transacción: un error posterior no deja textos
// reemplazados (CA-05).
func TestIntegrationEraserRollsBackWithTheTransaction(t *testing.T) {
	ctx := context.Background()
	db, convs, msgs := newDB(t)
	c, err := convs.Open(ctx, "b1", uAna, uBeto, now0)
	if err != nil {
		t.Fatal(err)
	}
	sendAt(t, NewSender(db), c.ID, uAna, uBeto, "hola", at(1))
	er := NewEraser(db)
	errBoom := context.Canceled
	err = adaptermongo.NewTxRunner(db).Do(ctx, func(tx context.Context) error {
		if _, err := er.Erase(tx, uAna, at(5)); err != nil {
			return err
		}
		return errBoom
	})
	if err != errBoom {
		t.Fatalf("err = %v", err)
	}
	h, _ := msgs.Before(ctx, c.ID, "", 10)
	cv, _ := convs.ByID(ctx, c.ID)
	if len(h) != 1 || h[0].Body != "hola" || cv.LastMessage.Preview != "hola" {
		t.Errorf("el rollback debe dejar el chat intacto: %+v %+v", h, cv.LastMessage)
	}
}
