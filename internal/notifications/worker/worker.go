// Package worker envía los push en segundo plano (spec 011, T007; plan P-01 y
// P-03): recibe los avisos por un canal con buffer, junta los mensajes en
// lotes de hasta 100 y los manda a Expo. El fallo del envío nunca afecta a
// quien notificó (Art. IX); un token DeviceNotRegistered se elimina (CA-02).
package worker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/expo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

// Tamaños por omisión (plan P-01 y RF-05).
const (
	DefaultBuffer = 500
	BatchSize     = expo.MaxBatch
)

// Tokens lee y borra tokens de push (adapter/mongo.TokenRepo).
type Tokens interface {
	TokensOf(ctx context.Context, userID string) ([]string, error)
	DeleteToken(ctx context.Context, token string) error
}

// Sender envía un lote a Expo y devuelve un ticket por mensaje
// (adapter/expo.Client).
type Sender interface {
	Send(ctx context.Context, msgs []expo.Message) ([]expo.Ticket, error)
}

// Deps son las dependencias del Worker.
type Deps struct {
	Tokens Tokens
	Sender Sender
	Log    *slog.Logger
	// Buffer es el tamaño del canal; 0 usa DefaultBuffer.
	Buffer int
}

type job struct {
	userID string
	push   notify.Push
}

// Worker cumple notify.Pusher. Enqueue no bloquea; Run procesa la cola.
type Worker struct {
	d  Deps
	ch chan job
}

var _ notify.Pusher = (*Worker)(nil)

// New crea el worker. Falla si falta una dependencia obligatoria.
func New(d Deps) (*Worker, error) {
	if d.Tokens == nil || d.Sender == nil || d.Log == nil {
		return nil, errors.New("worker: faltan dependencias")
	}
	if d.Buffer <= 0 {
		d.Buffer = DefaultBuffer
	}
	return &Worker{d: d, ch: make(chan job, d.Buffer)}, nil
}

// Enqueue encola el push de userID. Si el canal está lleno lo descarta y
// devuelve false: el aviso ya está guardado (plan P-01).
func (w *Worker) Enqueue(userID string, p notify.Push) bool {
	select {
	case w.ch <- job{userID: userID, push: p}:
		return true
	default:
		return false
	}
}

// Run procesa la cola hasta que ctx termine. Cada vuelta toma un aviso y suma
// los que ya estén esperando hasta completar un lote de 100 mensajes.
func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-w.ch:
			msgs := w.messages(ctx, j)
			for len(msgs) < BatchSize {
				select {
				case next := <-w.ch:
					msgs = append(msgs, w.messages(ctx, next)...)
				default:
					goto flush
				}
			}
		flush:
			w.sendAll(ctx, msgs)
		}
	}
}

// messages arma un mensaje por cada token de la persona. Un fallo al leer los
// tokens se registra y se descarta ese push.
func (w *Worker) messages(ctx context.Context, j job) []expo.Message {
	tokens, err := w.d.Tokens.TokensOf(ctx, j.userID)
	if err != nil {
		w.d.Log.WarnContext(ctx, "push: no se pudieron leer los tokens", "error", err)
		return nil
	}
	out := make([]expo.Message, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, expo.Message{To: t, Title: j.push.Title, Body: j.push.Body, Data: j.push.Data, Sound: "default"})
	}
	return out
}

// sendAll envía msgs en lotes de hasta BatchSize.
func (w *Worker) sendAll(ctx context.Context, msgs []expo.Message) {
	for len(msgs) > 0 {
		n := len(msgs)
		if n > BatchSize {
			n = BatchSize
		}
		w.send(ctx, msgs[:n])
		msgs = msgs[n:]
	}
}

// send manda un lote y procesa los tickets. Los registros no incluyen tokens
// ni textos (Art. VI).
func (w *Worker) send(ctx context.Context, batch []expo.Message) {
	tickets, err := w.d.Sender.Send(ctx, batch)
	if err != nil {
		w.d.Log.WarnContext(ctx, "push: falló el envío del lote", "messages", len(batch), "error", err)
		return
	}
	failed := 0
	for i, t := range tickets {
		if t.Error == "" || i >= len(batch) {
			continue
		}
		if t.Error == expo.DeviceNotRegistered {
			if err := w.d.Tokens.DeleteToken(ctx, batch[i].To); err != nil {
				w.d.Log.WarnContext(ctx, "push: no se pudo eliminar un token inválido", "error", err)
			}
			continue
		}
		failed++
		w.d.Log.WarnContext(ctx, "push: Expo rechazó un mensaje", "error", t.Error)
	}
	if failed > 0 {
		w.d.Log.WarnContext(ctx, "push: lote con rechazos", "messages", len(batch), "rejected", failed)
	}
}
