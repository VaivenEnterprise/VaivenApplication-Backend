package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/domain"
)

var t0 = time.Date(2026, 11, 17, 10, 0, 0, 123_000_000, time.UTC)

type fakeRepo struct {
	rows       []domain.Notification // más recientes primero
	err        error
	gotLimit   int
	gotAfter   string
	unread     int64
	markedIDs  []string
	markedUser string
	markedAt   time.Time
	allUser    string
	allCalled  bool
}

func (f *fakeRepo) List(_ context.Context, _, after string, limit int) ([]domain.Notification, error) {
	f.gotAfter, f.gotLimit = after, limit
	if f.err != nil {
		return nil, f.err
	}
	n := limit
	if n > len(f.rows) {
		n = len(f.rows)
	}
	return f.rows[:n], nil
}
func (f *fakeRepo) CountUnread(context.Context, string) (int64, error) { return f.unread, f.err }
func (f *fakeRepo) MarkRead(_ context.Context, u string, ids []string, at time.Time) error {
	f.markedUser, f.markedIDs, f.markedAt = u, ids, at
	return f.err
}
func (f *fakeRepo) MarkAllRead(_ context.Context, u string, at time.Time) error {
	f.allCalled, f.allUser, f.markedAt = true, u, at
	return f.err
}

type fakeTokens struct {
	upserted []domain.PushToken
	deleted  [][2]string
	err      error
}

func (f *fakeTokens) Upsert(_ context.Context, t domain.PushToken) error {
	f.upserted = append(f.upserted, t)
	return f.err
}
func (f *fakeTokens) Delete(_ context.Context, u, tok string) error {
	f.deleted = append(f.deleted, [2]string{u, tok})
	return f.err
}

func newSvc(t *testing.T) (*Service, *fakeRepo, *fakeTokens) {
	t.Helper()
	r, tk := &fakeRepo{}, &fakeTokens{}
	s, err := NewService(Deps{Repo: r, Tokens: tk, Now: func() time.Time { return t0 }})
	if err != nil {
		t.Fatal(err)
	}
	return s, r, tk
}

func isValidation(err error) bool {
	var de *shared.Error
	return errors.As(err, &de) && de.Code == shared.CodeValidation
}

func TestNewServiceNeedsDeps(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	s, err := NewService(Deps{Repo: &fakeRepo{}, Tokens: &fakeTokens{}})
	if err != nil || s.d.Now == nil {
		t.Errorf("NewService = %v, Now nil=%v", err, s.d.Now == nil)
	}
	if s.now().Location() != time.UTC {
		t.Error("now debe ser UTC")
	}
}

func rows(n int) []domain.Notification {
	out := make([]domain.Notification, n)
	for i := range out {
		out[i] = domain.Notification{ID: fmt.Sprintf("%024x", n-i)}
	}
	return out
}

func TestListPagination(t *testing.T) {
	s, r, _ := newSvc(t)
	r.rows = rows(5)

	p, err := s.List(context.Background(), "u1", "", 2)
	if err != nil || len(p.Items) != 2 || p.Next == nil || *p.Next != p.Items[1].ID {
		t.Fatalf("List = %+v, %v", p, err)
	}
	if r.gotLimit != 3 {
		t.Errorf("debe pedir limit+1, pidió %d", r.gotLimit)
	}
	p, _ = s.List(context.Background(), "u1", "abc", 10)
	if len(p.Items) != 5 || p.Next != nil || r.gotAfter != "abc" {
		t.Errorf("última página = %+v after=%q", p, r.gotAfter)
	}
	// Límites por omisión y máximo.
	_, _ = s.List(context.Background(), "u1", "", 0)
	if r.gotLimit != DefaultPage+1 {
		t.Errorf("limit por omisión = %d", r.gotLimit-1)
	}
	_, _ = s.List(context.Background(), "u1", "", 999)
	if r.gotLimit != MaxPage+1 {
		t.Errorf("limit máximo = %d", r.gotLimit-1)
	}
	r.err = errors.New("boom")
	if _, err := s.List(context.Background(), "u1", "", 1); err == nil {
		t.Error("el error del repositorio debe subir")
	}
}

func TestUnreadCount(t *testing.T) {
	s, r, _ := newSvc(t)
	r.unread = 7
	if n, err := s.UnreadCount(context.Background(), "u1"); err != nil || n != 7 {
		t.Errorf("UnreadCount = %d, %v", n, err)
	}
}

func TestMarkReadAll(t *testing.T) { // CA-07: después de read all el conteo es 0
	s, r, _ := newSvc(t)
	if err := s.MarkRead(context.Background(), "u1", nil, true); err != nil {
		t.Fatal(err)
	}
	if !r.allCalled || r.allUser != "u1" || !r.markedAt.Equal(t0.Truncate(time.Millisecond)) {
		t.Errorf("MarkAllRead = %+v", r)
	}
}

func TestMarkReadIDs(t *testing.T) {
	s, r, _ := newSvc(t)
	ids := []string{"6731e0000000000000000001", "6731e0000000000000000002"}
	if err := s.MarkRead(context.Background(), "u1", ids, false); err != nil {
		t.Fatal(err)
	}
	if r.markedUser != "u1" || len(r.markedIDs) != 2 || r.allCalled {
		t.Errorf("MarkRead = %+v", r)
	}
}

func TestMarkReadValidation(t *testing.T) {
	s, r, _ := newSvc(t)
	tooMany := make([]string, domain.MaxReadIDs+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("%024x", i+1)
	}
	cases := map[string]struct {
		ids []string
		all bool
	}{
		"ambos":     {[]string{"6731e0000000000000000001"}, true},
		"ninguno":   {nil, false},
		"id malo":   {[]string{"nope"}, false},
		"demasiado": {tooMany, false},
	}
	for name, c := range cases {
		if err := s.MarkRead(context.Background(), "u1", c.ids, c.all); !isValidation(err) {
			t.Errorf("%s: debe ser validation_failed, fue %v", name, err)
		}
	}
	if r.allCalled || r.markedUser != "" {
		t.Error("una petición inválida no debe tocar el repositorio")
	}
}

func TestRegisterToken(t *testing.T) {
	s, _, tk := newSvc(t)
	if err := s.RegisterToken(context.Background(), "u1", " ExponentPushToken[abc] ", "ios"); err != nil {
		t.Fatal(err)
	}
	got := tk.upserted[0]
	if got.Token != "ExponentPushToken[abc]" || got.UserID != "u1" || got.Platform != "ios" || !got.LastSeenAt.Equal(got.CreatedAt) {
		t.Errorf("token guardado = %+v", got)
	}
	for _, c := range [][2]string{{"x", "ios"}, {"ExponentPushToken[abc]", "web"}, {strings.Repeat("a", 300), "ios"}} {
		if err := s.RegisterToken(context.Background(), "u1", c[0], c[1]); !isValidation(err) {
			t.Errorf("RegisterToken(%q,%q) debe ser validation_failed, fue %v", c[0], c[1], err)
		}
	}
	if len(tk.upserted) != 1 {
		t.Error("un token inválido no se guarda")
	}
	tk.err = errors.New("boom")
	if err := s.RegisterToken(context.Background(), "u1", "ExponentPushToken[abc]", "android"); err == nil {
		t.Error("el error del repositorio debe subir")
	}
}

func TestUnregisterToken(t *testing.T) {
	s, _, tk := newSvc(t)
	if err := s.UnregisterToken(context.Background(), "u1", "ExponentPushToken[abc]"); err != nil {
		t.Fatal(err)
	}
	if len(tk.deleted) != 1 || tk.deleted[0] != [2]string{"u1", "ExponentPushToken[abc]"} {
		t.Errorf("Delete = %v", tk.deleted)
	}
	if err := s.UnregisterToken(context.Background(), "u1", "malo"); !isValidation(err) {
		t.Errorf("token inválido: %v", err)
	}
}
