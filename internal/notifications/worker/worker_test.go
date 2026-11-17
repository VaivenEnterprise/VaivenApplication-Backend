package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/expo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

type fakeTokens struct {
	mu      sync.Mutex
	byUser  map[string][]string
	deleted []string
	err     error
	delErr  error
}

func (f *fakeTokens) TokensOf(_ context.Context, u string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byUser[u], f.err
}
func (f *fakeTokens) DeleteToken(_ context.Context, t string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, t)
	return f.delErr
}
func (f *fakeTokens) Deleted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

type fakeSender struct {
	mu      sync.Mutex
	batches [][]expo.Message
	reply   func(batch []expo.Message) ([]expo.Ticket, error)
	got     chan struct{}
}

func newSender() *fakeSender { return &fakeSender{got: make(chan struct{}, 100)} }

func (f *fakeSender) Send(_ context.Context, m []expo.Message) ([]expo.Ticket, error) {
	f.mu.Lock()
	f.batches = append(f.batches, append([]expo.Message(nil), m...))
	reply := f.reply
	f.mu.Unlock()
	defer func() { f.got <- struct{}{} }()
	if reply != nil {
		return reply(m)
	}
	return make([]expo.Ticket, len(m)), nil
}
func (f *fakeSender) Batches() [][]expo.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]expo.Message(nil), f.batches...)
}

func (f *fakeSender) wait(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-f.got:
		case <-time.After(2 * time.Second):
			t.Fatalf("el envío %d no llegó", i+1)
		}
	}
}

// lockedBuffer permite leer los registros desde la prueba mientras el worker
// escribe en otra goroutine.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func quiet() (*slog.Logger, *lockedBuffer) {
	b := &lockedBuffer{}
	return slog.New(slog.NewTextHandler(b, nil)), b
}

func tokens(n int, prefix string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("ExponentPushToken[%s%d]", prefix, i)
	}
	return out
}

var push = notify.Push{Type: "booking_requested", Title: "Nueva solicitud", Body: "Alguien quiere uno de tus objetos.", Data: map[string]string{"bookingId": "b1"}}

func start(t *testing.T, tk Tokens, s Sender, buffer int) (*Worker, *lockedBuffer, func()) {
	t.Helper()
	log, buf := quiet()
	w, err := New(Deps{Tokens: tk, Sender: s, Log: log, Buffer: buffer})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	return w, buf, func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Run no terminó al cancelar el contexto")
		}
	}
}

func TestNewNeedsDeps(t *testing.T) {
	if _, err := New(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	log, _ := quiet()
	w, err := New(Deps{Tokens: &fakeTokens{}, Sender: newSender(), Log: log})
	if err != nil || cap(w.ch) != DefaultBuffer || DefaultBuffer != 500 {
		t.Errorf("buffer por omisión = %d, %v", cap(w.ch), err)
	}
}

func TestEnqueueDoesNotBlockWhenFull(t *testing.T) { // P-01: buffer de 500 sin bloquear el negocio
	log, _ := quiet()
	w, _ := New(Deps{Tokens: &fakeTokens{}, Sender: newSender(), Log: log}) // sin Run: nadie consume
	for i := 0; i < DefaultBuffer; i++ {
		if !w.Enqueue("u1", push) {
			t.Fatalf("el push %d debe caber", i)
		}
	}
	done := make(chan bool)
	go func() { done <- w.Enqueue("u1", push) }()
	select {
	case ok := <-done:
		if ok {
			t.Error("con el canal lleno Enqueue debe devolver false")
		}
	case <-time.After(time.Second):
		t.Fatal("Enqueue se bloqueó con el canal lleno")
	}
}

func TestSendsOneMessagePerTokenWithData(t *testing.T) { // RF-06
	tk := &fakeTokens{byUser: map[string][]string{"u1": tokens(2, "a")}}
	s := newSender()
	w, _, stop := start(t, tk, s, 0)
	defer stop()
	w.Enqueue("u1", push)
	s.wait(t, 1)
	b := s.Batches()
	if len(b) != 1 || len(b[0]) != 2 {
		t.Fatalf("lotes = %+v", b)
	}
	m := b[0][0]
	if m.To != "ExponentPushToken[a0]" || m.Title != push.Title || m.Body != push.Body || m.Data["bookingId"] != "b1" || m.Sound != "default" {
		t.Errorf("mensaje = %+v", m)
	}
}

func TestNoTokensNoSend(t *testing.T) {
	tk := &fakeTokens{byUser: map[string][]string{}}
	s := newSender()
	w, _, stop := start(t, tk, s, 0)
	w.Enqueue("sin-tokens", push)
	time.Sleep(50 * time.Millisecond)
	stop()
	if len(s.Batches()) != 0 {
		t.Error("sin tokens no se llama a Expo")
	}
}

func TestBatchesNeverExceed100(t *testing.T) { // RF-05
	tk := &fakeTokens{byUser: map[string][]string{"u1": tokens(250, "a")}}
	s := newSender()
	w, _, stop := start(t, tk, s, 0)
	defer stop()
	w.Enqueue("u1", push)
	s.wait(t, 3)
	b := s.Batches()
	sizes := []int{len(b[0]), len(b[1]), len(b[2])}
	if len(b) != 3 || sizes[0] != 100 || sizes[1] != 100 || sizes[2] != 50 {
		t.Errorf("tamaños de lote = %v", sizes)
	}
}

func TestCoalescesQueuedJobsIntoOneBatch(t *testing.T) {
	tk := &fakeTokens{byUser: map[string][]string{}}
	for i := 0; i < 30; i++ {
		tk.byUser[fmt.Sprintf("u%d", i)] = tokens(2, fmt.Sprintf("u%d-", i))
	}
	log, _ := quiet()
	s := newSender()
	w, _ := New(Deps{Tokens: tk, Sender: s, Log: log})
	for i := 0; i < 30; i++ { // se encolan antes de arrancar: 60 mensajes en espera
		w.Enqueue(fmt.Sprintf("u%d", i), push)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	s.wait(t, 1)
	time.Sleep(50 * time.Millisecond)
	b := s.Batches()
	if len(b) != 1 || len(b[0]) != 60 {
		t.Errorf("se esperaba un lote de 60, hubo %d lotes (%d en el primero)", len(b), len(b[0]))
	}
}

func TestDeviceNotRegisteredDeletesToken(t *testing.T) { // CA-02
	tk := &fakeTokens{byUser: map[string][]string{"u1": {"ExponentPushToken[bad]", "ExponentPushToken[good]"}}}
	s := newSender()
	s.reply = func(_ []expo.Message) ([]expo.Ticket, error) {
		return []expo.Ticket{{Status: "error", Error: expo.DeviceNotRegistered}, {Status: "ok"}}, nil
	}
	w, logs, stop := start(t, tk, s, 0)
	defer stop()
	w.Enqueue("u1", push)
	s.wait(t, 1)
	waitFor(t, func() bool { return len(tk.Deleted()) == 1 })
	if d := tk.Deleted(); d[0] != "ExponentPushToken[bad]" {
		t.Errorf("borrados = %v", d)
	}
	if strings.Contains(logs.String(), "ExponentPushToken") {
		t.Errorf("los registros no deben incluir tokens: %s", logs)
	}
}

func TestOtherTicketErrorsAreLoggedWithoutPII(t *testing.T) {
	tk := &fakeTokens{byUser: map[string][]string{"u1": {"ExponentPushToken[x]"}}}
	s := newSender()
	s.reply = func([]expo.Message) ([]expo.Ticket, error) {
		return []expo.Ticket{{Status: "error", Error: "MessageRateExceeded"}}, nil
	}
	w, logs, stop := start(t, tk, s, 0)
	w.Enqueue("u1", push)
	s.wait(t, 1)
	waitFor(t, func() bool { return strings.Contains(logs.String(), "rejected=1") })
	stop()
	out := logs.String()
	if !strings.Contains(out, "MessageRateExceeded") || strings.Contains(out, "ExponentPushToken") || strings.Contains(out, push.Body) || strings.Contains(out, "u1") {
		t.Errorf("registro inesperado: %s", out)
	}
	if len(tk.Deleted()) != 0 {
		t.Error("solo DeviceNotRegistered borra tokens")
	}
}

func TestFailuresDoNotStopTheWorker(t *testing.T) {
	tk := &fakeTokens{byUser: map[string][]string{"u1": {"ExponentPushToken[a]"}, "u2": {"ExponentPushToken[b]"}}}
	s := newSender()
	calls := 0
	s.reply = func(_ []expo.Message) ([]expo.Ticket, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("Expo caído")
		}
		return []expo.Ticket{{Status: "error", Error: expo.DeviceNotRegistered}}, nil
	}
	tk.delErr = errors.New("mongo caído")
	w, logs, stop := start(t, tk, s, 0)
	defer stop()
	w.Enqueue("u1", push)
	s.wait(t, 1)
	w.Enqueue("u2", push) // el siguiente aviso se procesa aunque el primero falló
	s.wait(t, 1)
	waitFor(t, func() bool { return strings.Contains(logs.String(), "no se pudo eliminar") })
	if !strings.Contains(logs.String(), "falló el envío del lote") {
		t.Errorf("debe registrar el fallo del lote: %s", logs)
	}
}

func TestTokenReadFailureDropsOnlyThatPush(t *testing.T) {
	tk := &fakeTokens{err: errors.New("mongo caído")}
	s := newSender()
	w, logs, stop := start(t, tk, s, 0)
	w.Enqueue("u1", push)
	waitFor(t, func() bool { return strings.Contains(logs.String(), "no se pudieron leer") })
	stop()
	if len(s.Batches()) != 0 {
		t.Error("sin tokens legibles no se envía nada")
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	_, _, stop := start(t, &fakeTokens{}, newSender(), 0)
	stop() // falla si Run no termina
}

func TestExtraTicketsAreIgnored(t *testing.T) {
	tk := &fakeTokens{byUser: map[string][]string{"u1": {"ExponentPushToken[a]"}}}
	s := newSender()
	s.reply = func([]expo.Message) ([]expo.Ticket, error) {
		return []expo.Ticket{{Status: "ok"}, {Status: "error", Error: expo.DeviceNotRegistered}}, nil
	}
	w, _, stop := start(t, tk, s, 0)
	defer stop()
	w.Enqueue("u1", push)
	s.wait(t, 1)
	time.Sleep(30 * time.Millisecond)
	if len(tk.Deleted()) != 0 {
		t.Error("un ticket sin mensaje no debe borrar nada")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("la condición no se cumplió a tiempo")
}
