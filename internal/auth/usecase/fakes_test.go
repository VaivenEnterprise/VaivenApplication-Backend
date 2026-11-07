package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

var errBoom = errors.New("falla simulada")

// fakeUsers es un repositorio de cuentas en memoria.
type fakeUsers struct {
	mu             sync.Mutex
	byID           map[string]domain.User
	next           int
	failAll        error // si no es nil, todas las operaciones fallan
	failUpdateHash error // solo UpdatePasswordHash falla
}

func newFakeUsers() *fakeUsers { return &fakeUsers{byID: map[string]domain.User{}} }

func (f *fakeUsers) Create(_ context.Context, u domain.User) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAll != nil {
		return domain.User{}, f.failAll
	}
	for _, x := range f.byID {
		if x.Email == u.Email {
			return domain.User{}, domain.ErrEmailTaken.Wrap(errors.New("E11000"))
		}
	}
	f.next++
	u.ID = fmt.Sprintf("%024x", f.next)
	f.byID[u.ID] = u
	return u, nil
}

func (f *fakeUsers) ByEmail(_ context.Context, email string) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAll != nil {
		return domain.User{}, f.failAll
	}
	for _, x := range f.byID {
		if x.Email == email {
			return x, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (f *fakeUsers) ByID(_ context.Context, id string) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAll != nil {
		return domain.User{}, f.failAll
	}
	u, ok := f.byID[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (f *fakeUsers) UpdateProfile(_ context.Context, id string, p ProfileChanges, now time.Time) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAll != nil {
		return domain.User{}, f.failAll
	}
	u, ok := f.byID[id]
	if !ok || !u.IsActive() {
		return domain.User{}, domain.ErrUserNotFound
	}
	if p.DisplayName != nil {
		u.DisplayName = *p.DisplayName
	}
	if p.ClearPhone {
		u.Phone = nil
	} else if p.Phone != nil {
		v := *p.Phone
		u.Phone = &v
	}
	u.UpdatedAt = now
	u.Version++
	f.byID[id] = u
	return u, nil
}

func (f *fakeUsers) UpdatePasswordHash(_ context.Context, id, hash string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAll != nil {
		return f.failAll
	}
	if f.failUpdateHash != nil {
		return f.failUpdateHash
	}
	u, ok := f.byID[id]
	if !ok || !u.IsActive() {
		return domain.ErrUserNotFound
	}
	u.PasswordHash, u.UpdatedAt = hash, now
	u.Version++
	f.byID[id] = u
	return nil
}

func (f *fakeUsers) Deactivate(_ context.Context, id, anonymizedEmail string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAll != nil {
		return f.failAll
	}
	u, ok := f.byID[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.Email, u.DisplayName, u.Phone = anonymizedEmail, domain.DeletedDisplayName, nil
	u.Status, u.DeactivatedAt, u.UpdatedAt = domain.StatusDeactivated, &now, now
	f.byID[id] = u
	return nil
}

// fakeRefresh es un repositorio de refresh tokens en memoria.
type fakeRefresh struct {
	mu         sync.Mutex
	byHash     map[string]domain.RefreshToken
	next       int
	failCreate error
	failFind   error
	failMark   error
	failRevoke error
	markRace   bool // MarkUsed devuelve false (otra petición ganó)
}

func newFakeRefresh() *fakeRefresh { return &fakeRefresh{byHash: map[string]domain.RefreshToken{}} }

func (f *fakeRefresh) Create(_ context.Context, t domain.RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCreate != nil {
		return f.failCreate
	}
	f.next++
	t.ID = fmt.Sprintf("%024x", f.next)
	f.byHash[t.TokenHash] = t
	return nil
}

func (f *fakeRefresh) ByHash(_ context.Context, hash string) (domain.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failFind != nil {
		return domain.RefreshToken{}, f.failFind
	}
	t, ok := f.byHash[hash]
	if !ok {
		return domain.RefreshToken{}, domain.ErrInvalidSession
	}
	return t, nil
}

func (f *fakeRefresh) MarkUsed(_ context.Context, id string, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failMark != nil {
		return false, f.failMark
	}
	if f.markRace {
		return false, nil
	}
	for h, t := range f.byHash {
		if t.ID == id && t.UsedAt == nil && t.RevokedAt == nil {
			t.UsedAt = &at
			f.byHash[h] = t
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRefresh) revoke(match func(domain.RefreshToken) bool, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRevoke != nil {
		return f.failRevoke
	}
	for h, t := range f.byHash {
		if match(t) && t.RevokedAt == nil {
			t.RevokedAt = &at
			f.byHash[h] = t
		}
	}
	return nil
}

func (f *fakeRefresh) RevokeFamily(_ context.Context, familyID string, at time.Time) error {
	return f.revoke(func(t domain.RefreshToken) bool { return t.FamilyID == familyID }, at)
}

func (f *fakeRefresh) RevokeAllForUser(_ context.Context, userID string, at time.Time) error {
	return f.revoke(func(t domain.RefreshToken) bool { return t.UserID == userID }, at)
}

// fakeHasher "hashea" anteponiendo un prefijo; cuenta las verificaciones.
type fakeHasher struct {
	mu         sync.Mutex
	verifies   int
	encodings  []string // hashes contra los que se verificó, en orden
	failHash   error
	failVerify error
	noDecoy    bool
}

// fakeDecoy es el hash señuelo del doble: nunca coincide con "hash:"+pw.
const fakeDecoy = "decoy:valor-desconocido"

// NeedsRehash marca como antiguos los hashes "legacy:<pw>".
func (h *fakeHasher) NeedsRehash(encoded string) bool { return strings.HasPrefix(encoded, "legacy:") }

func (h *fakeHasher) DecoyHash() string {
	if h.noDecoy {
		return ""
	}
	return fakeDecoy
}

func (h *fakeHasher) Hash(_ context.Context, pw string) (string, error) {
	if h.failHash != nil {
		return "", h.failHash
	}
	return "hash:" + pw, nil
}

// Verify respeta la cancelación como el Hasher real.
func (h *fakeHasher) Verify(ctx context.Context, pw, encoded string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	h.mu.Lock()
	h.verifies++
	h.encodings = append(h.encodings, encoded)
	h.mu.Unlock()
	if h.failVerify != nil {
		return false, h.failVerify
	}
	return encoded == "hash:"+pw || encoded == "legacy:"+pw, nil
}

// fakeAccess emite tokens de acceso legibles.
type fakeAccess struct{ fail error }

func (a fakeAccess) Issue(userID, role string) (string, error) {
	if a.fail != nil {
		return "", a.fail
	}
	return "access:" + userID + ":" + role, nil
}

func (fakeAccess) TTL() time.Duration { return 15 * time.Minute }

// fakeBookings simula el puerto de reservas activas.
type fakeBookings struct {
	active bool
	err    error
}

func (b fakeBookings) HasActiveBookings(context.Context, string) (bool, error) {
	return b.active, b.err
}

// env reúne el servicio y sus dobles.
type env struct {
	svc     *Service
	users   *fakeUsers
	refresh *fakeRefresh
	hasher  *fakeHasher
	now     time.Time
}

func newEnv(t *testing.T, mutate ...func(*Deps)) *env {
	t.Helper()
	e := &env{
		users:   newFakeUsers(),
		refresh: newFakeRefresh(),
		hasher:  &fakeHasher{},
		now:     time.Date(2026, 11, 6, 12, 0, 0, 123456789, time.UTC),
	}
	var n int
	d := Deps{
		Users:   e.users,
		Refresh: e.refresh,
		Hasher:  e.hasher,
		Access:  fakeAccess{},
		NewRefresh: func() (string, string, error) {
			n++
			tok := fmt.Sprintf("rt-%d", n)
			return tok, "h(" + tok + ")", nil
		},
		HashRefresh: func(tok string) string { return "h(" + tok + ")" },
		RefreshTTL:  720 * time.Hour,
		Now:         func() time.Time { return e.now },
	}
	for _, m := range mutate {
		m(&d)
	}
	svc, err := NewService(d)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	e.svc = svc
	return e
}

func validRegister() RegisterInput {
	return RegisterInput{Email: "  Ana@Example.com ", Password: "una-contrasena-larga", DisplayName: " Ana ", AcceptTerms: true, IsAdult: true}
}

// mustRegister registra a Ana y devuelve el resultado.
func (e *env) mustRegister(t *testing.T) AuthResult {
	t.Helper()
	res, err := e.svc.Register(context.Background(), validRegister())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return res
}

// noSecrets falla si s contiene la contraseña o algún token.
func noSecrets(t *testing.T, s string, secrets ...string) {
	t.Helper()
	for _, x := range secrets {
		if x != "" && strings.Contains(s, x) {
			t.Errorf("%q revela %q", s, x)
		}
	}
}
