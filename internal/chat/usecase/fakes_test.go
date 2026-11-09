package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

type memConvs struct {
	byID map[string]*domain.Conversation
	seq  int
	fail map[string]error
}

func (m *memConvs) Open(_ context.Context, bookingID, owner, req string, now time.Time) (domain.Conversation, error) {
	if err := m.fail["Open"]; err != nil {
		return domain.Conversation{}, err
	}
	for _, c := range m.byID {
		if c.BookingID == bookingID {
			return *c, nil
		}
	}
	m.seq++
	c := &domain.Conversation{ID: fmt.Sprintf("%024x", m.seq), BookingID: bookingID, Participants: [2]string{owner, req},
		Unread: map[string]int{}, LastMessageAt: now.Add(time.Duration(m.seq) * time.Second), CreatedAt: now}
	m.byID[c.ID] = c
	return *c, nil
}

func (m *memConvs) ByID(_ context.Context, id string) (domain.Conversation, error) {
	c, ok := m.byID[id]
	if !ok {
		return domain.Conversation{}, domain.ErrNotFound
	}
	cp := *c
	cp.Unread = map[string]int{}
	for k, v := range c.Unread {
		cp.Unread[k] = v
	}
	return cp, nil
}

func (m *memConvs) ListByParticipant(_ context.Context, userID string, after *domain.Cursor, limit int) ([]domain.Conversation, error) {
	if err := m.fail["List"]; err != nil {
		return nil, err
	}
	var out []domain.Conversation
	for _, c := range m.byID {
		if !c.IsParticipant(userID) {
			continue
		}
		if after != nil && !c.LastMessageAt.Before(after.At) && (!c.LastMessageAt.Equal(after.At) || c.ID >= after.ID) {
			continue
		}
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastMessageAt.Equal(out[j].LastMessageAt) {
			return out[i].LastMessageAt.After(out[j].LastMessageAt)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memConvs) RecordMessage(_ context.Context, id, receiver string, lm domain.LastMessage) (int, error) {
	if err := m.fail["Record"]; err != nil {
		return 0, err
	}
	c := m.byID[id]
	prev := c.Unread[receiver]
	c.Unread[receiver]++
	c.LastMessage, c.LastMessageAt = &lm, lm.At
	return prev, nil
}

func (m *memConvs) MarkRead(_ context.Context, id, user string) error {
	m.byID[id].Unread[user] = 0
	return nil
}

func (m *memConvs) UnreadTotal(_ context.Context, user string) (int, error) {
	n := 0
	for _, c := range m.byID {
		n += c.Unread[user]
	}
	return n, nil
}

type memMsgs struct {
	list []domain.Message
	seq  int
	fail map[string]error
	race bool // Insert devuelve duplicado tras guardar (envío simultáneo)
}

func (m *memMsgs) Insert(_ context.Context, msg domain.Message) (domain.Message, error) {
	if err := m.fail["Insert"]; err != nil {
		return domain.Message{}, err
	}
	if msg.ClientID != nil {
		for _, x := range m.list {
			if x.ConversationID == msg.ConversationID && x.SenderID == msg.SenderID && x.ClientID != nil && *x.ClientID == *msg.ClientID {
				return domain.Message{}, domain.ErrDuplicateClientID
			}
		}
	}
	m.seq++
	msg.ID = fmt.Sprintf("%024x", m.seq)
	m.list = append(m.list, msg)
	if m.race {
		return domain.Message{}, domain.ErrDuplicateClientID
	}
	return msg, nil
}

func (m *memMsgs) ByClientID(_ context.Context, conv, sender, cid string) (domain.Message, bool, error) {
	if err := m.fail["ByClientID"]; err != nil {
		return domain.Message{}, false, err
	}
	for _, x := range m.list {
		if x.ConversationID == conv && x.SenderID == sender && x.ClientID != nil && *x.ClientID == cid {
			return x, true, nil
		}
	}
	return domain.Message{}, false, nil
}

func (m *memMsgs) Before(_ context.Context, conv, before string, limit int) ([]domain.Message, error) {
	if err := m.fail["Before"]; err != nil {
		return nil, err
	}
	var out []domain.Message
	for i := len(m.list) - 1; i >= 0 && len(out) < limit; i-- {
		x := m.list[i]
		if x.ConversationID == conv && (before == "" || x.ID < before) {
			out = append(out, x)
		}
	}
	return out, nil
}

func (m *memMsgs) After(_ context.Context, conv, after string, limit int) ([]domain.Message, error) {
	if err := m.fail["After"]; err != nil {
		return nil, err
	}
	var out []domain.Message
	for _, x := range m.list {
		if x.ConversationID == conv && x.ID > after && len(out) < limit {
			out = append(out, x)
		}
	}
	return out, nil
}

// fakeSender hace en memoria lo que adapter/mongo.Sender hace en una
// transacción: guarda el mensaje y luego actualiza la conversación.
type fakeSender struct {
	convs *memConvs
	msgs  *memMsgs
}

func (f fakeSender) Send(ctx context.Context, m domain.Message, receiver string, lm domain.LastMessage) (domain.Message, int, error) {
	saved, err := f.msgs.Insert(ctx, m)
	if err != nil {
		return domain.Message{}, 0, err
	}
	prev, err := f.convs.RecordMessage(ctx, m.ConversationID, receiver, lm)
	if err != nil {
		return domain.Message{}, 0, err
	}
	return saved, prev, nil
}

type fakeBookings struct {
	m   map[string]domain.BookingInfo
	err error
}

func (f *fakeBookings) ByIDs(_ context.Context, ids []string) (map[string]domain.BookingInfo, error) {
	out := map[string]domain.BookingInfo{}
	for _, id := range ids {
		if b, ok := f.m[id]; ok {
			out[id] = b
		}
	}
	return out, f.err
}

type fakeAccounts struct {
	inactive map[string]bool
	err      error
}

func (f *fakeAccounts) Active(_ context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = !f.inactive[id]
	}
	return out, f.err
}

type fakeUsers struct{ err error }

func (f fakeUsers) PublicUsers(_ context.Context, ids []string) (map[string]users.PublicUser, error) {
	out := map[string]users.PublicUser{}
	for _, id := range ids {
		if id != "ghost" {
			out[id] = users.PublicUser{ID: id, DisplayName: "Persona " + id}
		}
	}
	return out, f.err
}

type fakeImages struct{ err error }

func (fakeImages) Signature(context.Context, string, images.Purpose) (images.UploadSignature, error) {
	return images.UploadSignature{}, nil
}
func (fakeImages) Resolve(context.Context, string, images.Purpose, string, []images.Ref) ([]images.Image, error) {
	return nil, nil
}
func (f fakeImages) URLs(i images.Image) (images.URLs, error) {
	return images.URLs{ThumbURL: "t/" + i.PublicID}, f.err
}

type fakeMod struct{ err error }

func (f fakeMod) Check(_ context.Context, text string) (moderation.Result, error) {
	if text == "prohibido" {
		return moderation.Result{Reason: "contiene contenido prohibido"}, f.err
	}
	if text == "raro" {
		return moderation.Result{}, f.err
	}
	return moderation.Result{Allowed: true}, f.err
}

type fakeNotifier struct {
	sent []string
	err  error
}

func (f *fakeNotifier) Notify(_ context.Context, to string, e notify.Event) error {
	f.sent = append(f.sent, to+":"+e.Type)
	return f.err
}

type env struct {
	svc      *Service
	convs    *memConvs
	msgs     *memMsgs
	bookings *fakeBookings
	accounts *fakeAccounts
	not      *fakeNotifier
	logs     *bytes.Buffer
}

var t0 = time.Date(2026, 11, 9, 17, 0, 0, 0, time.UTC)

func newEnv(t *testing.T, mutate ...func(*Deps)) env {
	t.Helper()
	e := env{
		convs:    &memConvs{byID: map[string]*domain.Conversation{}, fail: map[string]error{}},
		msgs:     &memMsgs{fail: map[string]error{}},
		bookings: &fakeBookings{m: map[string]domain.BookingInfo{}},
		accounts: &fakeAccounts{inactive: map[string]bool{}},
		not:      &fakeNotifier{},
		logs:     &bytes.Buffer{},
	}
	d := Deps{Conversations: e.convs, Messages: e.msgs, Sender: fakeSender{e.convs, e.msgs}, Bookings: e.bookings, Accounts: e.accounts, Users: fakeUsers{},
		Images: fakeImages{}, Moderation: fakeMod{}, Notifier: e.not, Log: slog.New(slog.NewTextHandler(e.logs, nil)),
		Now: func() time.Time { return t0 }}
	for _, m := range mutate {
		m(&d)
	}
	svc, err := NewService(d)
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

// booking registra una reserva y abre su conversación.
func (e env) booking(t *testing.T, id, status string) string {
	t.Helper()
	e.bookings.m[id] = domain.BookingInfo{ID: id, Status: status, OwnerID: "own", RequesterID: "req", ItemID: "i1",
		ItemTitle: "Taladro", ItemPhoto: &images.Image{PublicID: "p1"}}
	cid, err := e.svc.Open(context.Background(), id, "own", "req")
	if err != nil {
		t.Fatal(err)
	}
	return cid
}

func codeOf(t *testing.T, err error) shared.Code {
	t.Helper()
	var de *shared.Error
	if !errors.As(err, &de) {
		t.Fatalf("err = %v", err)
	}
	return de.Code
}
