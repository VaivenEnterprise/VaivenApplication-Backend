package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

type fakeStore struct {
	saved []Record
	err   error
}

func (f *fakeStore) Save(_ context.Context, r Record) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, r)
	return nil
}

type fakeDir struct {
	m   map[string]users.PublicUser
	err error
}

func (f fakeDir) PublicUsers(context.Context, []string) (map[string]users.PublicUser, error) {
	return f.m, f.err
}

type fakePusher struct {
	got  []Push
	to   []string
	full bool
}

func (f *fakePusher) Enqueue(userID string, p Push) bool {
	if f.full {
		return false
	}
	f.to, f.got = append(f.to, userID), append(f.got, p)
	return true
}

var now = time.Date(2026, 11, 17, 10, 0, 0, 999_999_999, time.UTC)

func newSvc(t *testing.T, st Store, dir users.Directory, p Pusher) *Service {
	t.Helper()
	s, err := NewService(ServiceDeps{Store: st, Users: dir, Pusher: p, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func activeDir() fakeDir {
	return fakeDir{m: map[string]users.PublicUser{"u1": {ID: "u1", DisplayName: "Ana"}}}
}

func TestNewServiceNeedsDeps(t *testing.T) {
	if _, err := NewService(ServiceDeps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	s, err := NewService(ServiceDeps{Store: &fakeStore{}, Users: activeDir(), Log: slog.Default()})
	if err != nil || s.d.Now == nil {
		t.Errorf("NewService = %v", err)
	}
}

func TestNotifySavesAndPushesWithTemplate(t *testing.T) { // CA-01
	st, p := &fakeStore{}, &fakePusher{}
	s := newSvc(t, st, activeDir(), p)
	err := s.Notify(context.Background(), "u1", Event{Type: "booking.requested", Data: map[string]string{"bookingId": "b1"}})
	if err != nil {
		t.Fatal(err)
	}
	tpl, _ := TemplateFor(TypeBookingRequested)
	want := Record{UserID: "u1", Type: TypeBookingRequested, Title: tpl.Title, Body: tpl.Body,
		Data: map[string]string{"bookingId": "b1"}, CreatedAt: now.Truncate(time.Millisecond)}
	if len(st.saved) != 1 || !reflect.DeepEqual(st.saved[0], want) {
		t.Fatalf("guardado = %+v, quiero %+v", st.saved, want)
	}
	if len(p.got) != 1 || p.to[0] != "u1" || p.got[0].Data["bookingId"] != "b1" || p.got[0].Type != TypeBookingRequested || p.got[0].Title != tpl.Title {
		t.Errorf("push = %+v para %v", p.got, p.to)
	}
}

func TestNotifyPrefersCallerText(t *testing.T) {
	st := &fakeStore{}
	s := newSvc(t, st, activeDir(), nil)
	if err := s.Notify(context.Background(), "u1", Event{Type: TypeMessageReceived, Title: "Hola", Body: "Texto propio"}); err != nil {
		t.Fatal(err)
	}
	if st.saved[0].Title != "Hola" || st.saved[0].Body != "Texto propio" {
		t.Errorf("guardado = %+v", st.saved[0])
	}
	if st.saved[0].Data == nil {
		t.Error("Data nunca debe ser nil")
	}
}

func TestNotifyKeepsOnlyIDsInData(t *testing.T) { // Art. VI.5
	st, p := &fakeStore{}, &fakePusher{}
	s := newSvc(t, st, activeDir(), p)
	_ = s.Notify(context.Background(), "u1", Event{Type: TypeIncidentOpened, Data: map[string]string{
		"incidentId": "i1", "bookingId": "", "email": "a@b.c", "phone": "55", "conversationId": "c1", "itemId": "x"}})
	want := map[string]string{"incidentId": "i1", "conversationId": "c1", "itemId": "x"}
	if !reflect.DeepEqual(st.saved[0].Data, want) || !reflect.DeepEqual(p.got[0].Data, want) {
		t.Errorf("data = %v / %v, quiero %v", st.saved[0].Data, p.got[0].Data, want)
	}
}

func TestNotifyUnknownTypeIsIgnored(t *testing.T) {
	st := &fakeStore{}
	s := newSvc(t, st, activeDir(), &fakePusher{})
	if err := s.Notify(context.Background(), "u1", Event{Type: "booking.meeting_point"}); err != nil || len(st.saved) != 0 {
		t.Errorf("tipo desconocido: err=%v guardados=%d", err, len(st.saved))
	}
}

func TestNotifySkipsDeletedOrMissingAccounts(t *testing.T) { // RF-05: sin push a cuentas dadas de baja
	for name, dir := range map[string]fakeDir{
		"eliminada":   {m: map[string]users.PublicUser{"u1": {ID: "u1", DisplayName: DeletedName}}},
		"inexistente": {m: map[string]users.PublicUser{}},
	} {
		st, p := &fakeStore{}, &fakePusher{}
		s := newSvc(t, st, dir, p)
		if err := s.Notify(context.Background(), "u1", Event{Type: TypeRatingReceived}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(st.saved) != 0 || len(p.got) != 0 {
			t.Errorf("%s: no debe guardar ni enviar push (%d, %d)", name, len(st.saved), len(p.got))
		}
	}
}

func TestNotifyErrors(t *testing.T) {
	s := newSvc(t, &fakeStore{}, fakeDir{err: errors.New("boom")}, nil)
	if err := s.Notify(context.Background(), "u1", Event{Type: TypeRatingReceived}); err == nil {
		t.Error("error al leer la cuenta debe subir")
	}
	s = newSvc(t, &fakeStore{err: errors.New("boom")}, activeDir(), &fakePusher{})
	if err := s.Notify(context.Background(), "u1", Event{Type: TypeRatingReceived}); err == nil {
		t.Error("error al guardar debe subir")
	}
}

func TestNotifyFullQueueDoesNotFail(t *testing.T) { // P-01: el aviso ya existe aunque el push se descarte
	st := &fakeStore{}
	s := newSvc(t, st, activeDir(), &fakePusher{full: true})
	if err := s.Notify(context.Background(), "u1", Event{Type: TypeRatingReceived}); err != nil || len(st.saved) != 1 {
		t.Errorf("err=%v guardados=%d", err, len(st.saved))
	}
}
