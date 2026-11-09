package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

func TestNewService(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias")
	}
	e := newEnv(t, func(d *Deps) { d.Now = nil })
	if e.svc.now().IsZero() {
		t.Error("Now por defecto")
	}
}

func TestOpenCA01(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a, _ := e.svc.Open(ctx, "b1", "own", "req")
	b, _ := e.svc.Open(ctx, "b1", "own", "req")
	if a == "" || a != b || len(e.convs.byID) != 1 {
		t.Errorf("Open idempotente: %s %s", a, b)
	}
	e.convs.fail["Open"] = errors.New("mongo")
	if _, err := e.svc.Open(ctx, "b2", "own", "req"); err == nil {
		t.Error("error del repositorio")
	}
}

func TestSendAndUnreadCA06(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	cid := e.booking(t, "b1", "accepted")
	for i, body := range []string{" hola ", "¿a las 5?", strings.Repeat("x", 100)} {
		m, created, err := e.svc.Send(ctx, "req", cid, body, nil)
		if err != nil || !created || m.SenderID != "req" {
			t.Fatalf("Send %d = %+v %v %v", i, m, created, err)
		}
		if i == 0 && m.Body != "hola" {
			t.Errorf("cuerpo recortado = %q", m.Body)
		}
	}
	// RF-12: un solo aviso mientras haya no leídos.
	if len(e.not.sent) != 1 || e.not.sent[0] != "own:message_received" {
		t.Errorf("avisos = %v", e.not.sent)
	}
	if n, _ := e.svc.UnreadCount(ctx, "own"); n != 3 {
		t.Errorf("own = %d", n)
	}
	if n, _ := e.svc.UnreadCount(ctx, "req"); n != 0 {
		t.Errorf("req = %d", n)
	}
	page, _ := e.svc.List(ctx, "own", nil, 0)
	if len(page.Items) != 1 || page.Items[0].Unread != 3 || len([]rune(page.Items[0].Conversation.LastMessage.Preview)) != 81 {
		t.Errorf("lista = %+v", page.Items)
	}
	if err := e.svc.Read(ctx, "own", cid); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.UnreadCount(ctx, "own"); n != 0 {
		t.Errorf("tras leer = %d", n)
	}
	// Tras leer, el siguiente mensaje vuelve a avisar.
	_, _, _ = e.svc.Send(ctx, "req", cid, "otro", nil)
	if len(e.not.sent) != 2 {
		t.Errorf("avisos tras leer = %v", e.not.sent)
	}
	// Quien envía no recibe nada en su contador.
	page, _ = e.svc.List(ctx, "req", nil, 0)
	if page.Items[0].Unread != 0 {
		t.Error("el remitente no acumula no leídos")
	}
}

func TestSendRulesCA02CA03CA04CA05(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	cid := e.booking(t, "b1", "pending")

	if _, _, err := e.svc.Send(ctx, "otro", cid, "hola", nil); !errors.Is(err, domain.ErrNotFound) { // CA-02
		t.Errorf("ajeno = %v", err)
	}
	if _, _, err := e.svc.Send(ctx, "req", cid, strings.Repeat("a", 1001), nil); codeOf(t, err) != shared.CodeValidation { // CA-05
		t.Error("1001 caracteres")
	}
	if _, _, err := e.svc.Send(ctx, "req", cid, "   ", nil); codeOf(t, err) != shared.CodeValidation {
		t.Error("solo espacios")
	}
	long := strings.Repeat("x", 65)
	if _, _, err := e.svc.Send(ctx, "req", cid, "hola", &long); codeOf(t, err) != shared.CodeValidation {
		t.Error("clientId largo")
	}
	// CA-03: el mismo clientId dos veces → un solo mensaje.
	k := "k1"
	m1, created1, err := e.svc.Send(ctx, "req", cid, "hola", &k)
	m2, created2, err2 := e.svc.Send(ctx, "req", cid, "hola", &k)
	if err != nil || err2 != nil || !created1 || created2 || m1.ID != m2.ID || len(e.msgs.list) != 1 {
		t.Errorf("idempotencia = %v %v %v %v", created1, created2, err, err2)
	}
	// Moderación (RF-10).
	_, _, err = e.svc.Send(ctx, "req", cid, "prohibido", nil)
	var de *shared.Error
	if !errors.As(err, &de) || de.Details[0].Issue != "contiene contenido prohibido" {
		t.Errorf("moderación = %v", err)
	}
	if _, _, err := e.svc.Send(ctx, "req", cid, "raro", nil); !errors.As(err, &de) || de.Details[0].Issue != "contiene contenido no permitido" {
		t.Error("moderación sin motivo")
	}
	// CA-04: reserva cancelada → 409, pero el historial se lee.
	e.bookings.m["b1"] = domain.BookingInfo{ID: "b1", Status: "cancelled", OwnerID: "own", RequesterID: "req"}
	if _, _, err := e.svc.Send(ctx, "req", cid, "hola", nil); !errors.Is(err, domain.ErrClosed) {
		t.Errorf("cerrada = %v", err)
	}
	if p, err := e.svc.Messages(ctx, "req", cid, MessageQuery{}); err != nil || len(p.Items) != 1 {
		t.Errorf("historial = %v", err)
	}
	// Repetir un clientId ya enviado devuelve el mensaje aunque esté cerrada.
	if m, created, err := e.svc.Send(ctx, "req", cid, "hola", &k); err != nil || created || m.ID != m1.ID {
		t.Error("reintento tras cerrar")
	}
	// RF-07: contraparte dada de baja.
	e.bookings.m["b1"] = domain.BookingInfo{ID: "b1", Status: "accepted", OwnerID: "own", RequesterID: "req"}
	e.accounts.inactive["own"] = true
	if _, _, err := e.svc.Send(ctx, "req", cid, "hola", nil); !errors.Is(err, domain.ErrClosed) {
		t.Errorf("baja = %v", err)
	}
}

func TestSendFailures(t *testing.T) {
	ctx := context.Background()
	steps := []struct {
		name string
		set  func(e env)
	}{
		{"accounts", func(e env) { e.accounts.err = errors.New("x") }},
		{"bookings", func(e env) { e.bookings.err = errors.New("x") }},
		{"insert", func(e env) { e.msgs.fail["Insert"] = errors.New("x") }},
		{"record", func(e env) { e.convs.fail["Record"] = errors.New("x") }},
		{"byClientID", func(e env) { e.msgs.fail["ByClientID"] = errors.New("x") }},
	}
	for _, s := range steps {
		e := newEnv(t)
		cid := e.booking(t, "b1", "accepted")
		s.set(e)
		k := "k"
		if _, _, err := e.svc.Send(ctx, "req", cid, "hola", &k); err == nil {
			t.Errorf("%s: debe fallar", s.name)
		}
	}
	e := newEnv(t, func(d *Deps) { d.Moderation = fakeMod{err: errors.New("x")} })
	cid := e.booking(t, "b1", "accepted")
	if _, _, err := e.svc.Send(ctx, "req", cid, "hola", nil); err == nil {
		t.Error("moderación caída")
	}
	// Aviso caído: el mensaje se envía igual y se registra.
	e = newEnv(t)
	cid = e.booking(t, "b1", "accepted")
	e.not.err = errors.New("push")
	if _, _, err := e.svc.Send(ctx, "req", cid, "hola", nil); err != nil || !strings.Contains(e.logs.String(), "no se pudo notificar") {
		t.Errorf("aviso caído = %v", err)
	}
	// Carrera de clientId: el repositorio devuelve duplicado → se devuelve el existente.
	e = newEnv(t)
	cid = e.booking(t, "b1", "accepted")
	e.msgs.race = true
	k := "k"
	if m, created, err := e.svc.Send(ctx, "req", cid, "hola", &k); err != nil || created || m.ID == "" {
		t.Errorf("carrera = %+v %v %v", m, created, err)
	}
	// Carrera de clientId sin mensaje encontrado: error interno, nunca un
	// mensaje vacío (revisión T009, observación 4).
	e = newEnv(t)
	cid = e.booking(t, "b1", "accepted")
	e.msgs.fail["Insert"] = domain.ErrDuplicateClientID
	if m, _, err := e.svc.Send(ctx, "req", cid, "hola", &k); err == nil || m.ID != "" {
		t.Errorf("carrera sin mensaje = %+v %v", m, err)
	}
	e.msgs.fail["Insert"] = nil
	e.msgs.fail["ByClientID"] = nil
	// Conversación huérfana (la solicitud no se guardó): 404.
	e = newEnv(t)
	orphan, _ := e.svc.Open(ctx, "nunca", "own", "req")
	if _, _, err := e.svc.Send(ctx, "req", orphan, "hola", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("huérfana = %v", err)
	}
	if _, _, err := e.svc.Send(ctx, "req", "nada", "hola", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Error("inexistente")
	}
}

func TestMessagesPaging(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	cid := e.booking(t, "b1", "accepted")
	for range 5 {
		_, _, _ = e.svc.Send(ctx, "req", cid, "m", nil)
	}
	p, err := e.svc.Messages(ctx, "own", cid, MessageQuery{Limit: 2})
	if err != nil || len(p.Items) != 2 || p.Next == nil || p.Items[0].ID < p.Items[1].ID {
		t.Fatalf("primera = %+v %v", p, err)
	}
	p2, _ := e.svc.Messages(ctx, "own", cid, MessageQuery{Limit: 2, Before: *p.Next})
	p3, _ := e.svc.Messages(ctx, "own", cid, MessageQuery{Limit: 2, Before: *p2.Next})
	if len(p2.Items) != 2 || len(p3.Items) != 1 || p3.Next != nil {
		t.Errorf("páginas = %d %d", len(p2.Items), len(p3.Items))
	}
	after, _ := e.svc.Messages(ctx, "own", cid, MessageQuery{After: p3.Items[0].ID})
	if len(after.Items) != 4 || after.Next != nil || after.Items[0].ID > after.Items[1].ID {
		t.Errorf("after = %+v", after)
	}
	if p, _ := e.svc.Messages(ctx, "own", cid, MessageQuery{Limit: 500}); len(p.Items) != 5 {
		t.Error("límite máximo")
	}
	if _, err := e.svc.Messages(ctx, "own", cid, MessageQuery{Before: "a", After: "b"}); codeOf(t, err) != shared.CodeValidation {
		t.Error("before + after")
	}
	if _, err := e.svc.Messages(ctx, "otro", cid, MessageQuery{}); !errors.Is(err, domain.ErrNotFound) {
		t.Error("ajeno")
	}
	// Un cursor que no es de la conversación: 400 sobre el parámetro usado.
	e.msgs.fail["After"] = domain.ErrUnknownMessage
	var de *shared.Error
	if _, err := e.svc.Messages(ctx, "own", cid, MessageQuery{After: "x"}); !errors.As(err, &de) || de.Details[0].Field != "after" {
		t.Errorf("after desconocido = %v", err)
	}
	e.msgs.fail["Before"] = domain.ErrUnknownMessage
	if _, err := e.svc.Messages(ctx, "own", cid, MessageQuery{Before: "x"}); !errors.As(err, &de) || de.Details[0].Field != "before" {
		t.Errorf("before desconocido = %v", err)
	}
	e.msgs.fail["After"] = errors.New("x")
	if _, err := e.svc.Messages(ctx, "own", cid, MessageQuery{After: "x"}); err == nil {
		t.Error("error en after")
	}
	e.msgs.fail["Before"] = errors.New("x")
	if _, err := e.svc.Messages(ctx, "own", cid, MessageQuery{}); err == nil {
		t.Error("error del repositorio")
	}
	if err := e.svc.Read(ctx, "otro", cid); !errors.Is(err, domain.ErrNotFound) {
		t.Error("leer ajena")
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	for _, id := range []string{"b1", "b2", "b3"} {
		e.booking(t, id, "accepted")
	}
	e.bookings.m["b2"] = domain.BookingInfo{ID: "b2", Status: "cancelled", OwnerID: "own", RequesterID: "req", ItemTitle: "Sin foto"}
	_, _ = e.svc.Open(ctx, "huerfana", "own", "req")

	var seen []ConversationView
	var after *domain.Cursor
	for range 5 {
		p, err := e.svc.List(ctx, "req", after, 2)
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, p.Items...)
		if p.Next == nil {
			break
		}
		after = p.Next
	}
	if len(seen) != 3 {
		t.Fatalf("vistas = %d (la huérfana se omite)", len(seen))
	}
	for _, v := range seen {
		if v.Counterpart.ID != "own" || v.Counterpart.DisplayName != "Persona own" {
			t.Errorf("contraparte = %+v", v.Counterpart)
		}
		switch v.Booking.ID {
		case "b2":
			if v.CanWrite || v.ThumbURL != nil {
				t.Error("b2 cerrada y sin foto")
			}
		default:
			if !v.CanWrite || *v.ThumbURL != "t/p1" {
				t.Errorf("%s = %+v", v.Booking.ID, v)
			}
		}
	}
	if p, _ := e.svc.List(ctx, "nadie", nil, 500); len(p.Items) != 0 {
		t.Error("vacía")
	}
	e.accounts.inactive["own"] = true
	if p, _ := e.svc.List(ctx, "req", nil, 50); p.Items[0].CanWrite {
		t.Error("contraparte de baja")
	}
	for name, mutate := range map[string]func(*Deps){
		"usuarios": func(d *Deps) { d.Users = fakeUsers{err: errors.New("x")} },
		"imagenes": func(d *Deps) { d.Images = fakeImages{err: errors.New("x")} },
	} {
		e2 := newEnv(t, mutate)
		e2.booking(t, "b1", "accepted")
		if _, err := e2.svc.List(ctx, "req", nil, 5); err == nil {
			t.Errorf("%s: debe fallar", name)
		}
	}
	for name, set := range map[string]func(e env){
		"lista":    func(e env) { e.convs.fail["List"] = errors.New("x") },
		"reservas": func(e env) { e.bookings.err = errors.New("x") },
		"cuentas":  func(e env) { e.accounts.err = errors.New("x") },
	} {
		e2 := newEnv(t)
		e2.booking(t, "b1", "accepted")
		set(e2)
		if _, err := e2.svc.List(ctx, "req", nil, 5); err == nil {
			t.Errorf("%s: debe fallar", name)
		}
	}
	e3 := newEnv(t)
	e3.bookings.m["b9"] = domain.BookingInfo{ID: "b9", Status: "accepted"}
	c, _ := e3.svc.Open(ctx, "b9", "ghost", "req")
	if p, _ := e3.svc.List(ctx, "req", nil, 5); p.Items[0].Counterpart.DisplayName != DeletedName || c == "" {
		t.Error("contraparte eliminada")
	}
}
