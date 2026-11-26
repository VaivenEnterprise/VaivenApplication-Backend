package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	searchdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

var errBoom = errors.New("falla de prueba")

// store es un doble en memoria de todos los puertos. fail[op] hace fallar esa
// operación.
type store struct {
	mu sync.Mutex
	cm sync.Mutex // protege calls (las operaciones pueden ser concurrentes)

	views    []domain.Event
	requests []domain.Event
	active   map[string][]string // persona → objetos en reserva activa
	items    map[string]itemRow
	gone     map[string]bool // cuentas dadas de baja

	neighbors map[string]map[string][]domain.Neighbor // buildID → objeto → vecinos
	popular   map[string]map[string][]string          // buildID → clave → objetos
	current   string
	builtAt   time.Time
	lockOwner string
	lockUntil time.Time
	resets    map[string]time.Time

	hidden map[string]bool // objetos que la búsqueda no deja ver
	fail   map[string]bool
	calls  map[string]int
}

type itemRow struct {
	owner, status string
	feats         domain.ItemFeatures
}

func newStore() *store {
	return &store{
		active: map[string][]string{}, items: map[string]itemRow{}, gone: map[string]bool{},
		neighbors: map[string]map[string][]domain.Neighbor{}, popular: map[string]map[string][]string{},
		resets: map[string]time.Time{}, hidden: map[string]bool{}, fail: map[string]bool{}, calls: map[string]int{},
	}
}

func (s *store) op(name string) error {
	s.cm.Lock()
	defer s.cm.Unlock()
	s.calls[name]++
	if s.fail[name] {
		return fmt.Errorf("%s: %w", name, errBoom)
	}
	return nil
}

func (s *store) addItem(id, owner, status, category, municipality string) {
	s.items[id] = itemRow{owner: owner, status: status, feats: domain.ItemFeatures{
		ID: id, OwnerID: owner, Title: "Título " + id, Category: category, Modality: "rent", PricePerDayCents: 5000, Municipality: municipality,
	}}
}

// --- Interactions ---

func (s *store) Record(_ context.Context, e domain.Event) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.op("Record"); err != nil {
		return false, err
	}
	for _, v := range s.views {
		if v.UserID == e.UserID && v.ItemID == e.ItemID && v.Bucket().Equal(e.Bucket()) {
			return false, nil
		}
	}
	s.views = append(s.views, e)
	return true, nil
}

func newestFirst(events []domain.Event) {
	slices.SortFunc(events, func(a, b domain.Event) int { return b.At.Compare(a.At) })
}

func (s *store) Recent(_ context.Context, userID string, limit int) ([]domain.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.op("InteractionsRecent"); err != nil {
		return nil, err
	}
	var out []domain.Event
	for _, v := range s.views {
		if v.UserID == userID {
			out = append(out, v)
		}
	}
	newestFirst(out)
	return out[:min(limit, len(out))], nil
}

func (s *store) Since(_ context.Context, since time.Time, limit int) ([]domain.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.op("InteractionsSince"); err != nil {
		return nil, err
	}
	return since2(s.views, since, limit), nil
}

func since2(events []domain.Event, since time.Time, limit int) []domain.Event {
	var out []domain.Event
	for _, e := range events {
		if !e.At.Before(since) {
			out = append(out, e)
		}
	}
	newestFirst(out)
	return out[:min(limit, len(out))]
}

func (s *store) DeleteUser(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.op("DeleteUser"); err != nil {
		return err
	}
	s.views = slices.DeleteFunc(s.views, func(e domain.Event) bool { return e.UserID == userID })
	return nil
}

func (s *store) DeleteUsers(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.op("DeleteUsers"); err != nil {
		return err
	}
	s.views = slices.DeleteFunc(s.views, func(e domain.Event) bool { return slices.Contains(ids, e.UserID) })
	return nil
}

// --- Requests (otro tipo para no chocar con los métodos de Interactions) ---

type requests struct{ *store }

func (r requests) Since(_ context.Context, since time.Time, limit int) ([]domain.Event, error) {
	if err := r.op("RequestsSince"); err != nil {
		return nil, err
	}
	return since2(r.requests, since, limit), nil
}

func (r requests) Recent(_ context.Context, userID string, since time.Time, limit int) ([]domain.Event, error) {
	if err := r.op("RequestsRecent"); err != nil {
		return nil, err
	}
	var out []domain.Event
	for _, e := range r.requests {
		if e.UserID == userID && (since.IsZero() || e.At.After(since)) {
			out = append(out, e)
		}
	}
	newestFirst(out)
	return out[:min(limit, len(out))], nil
}

func (r requests) ActiveItemIDs(_ context.Context, userID string) ([]string, error) {
	if err := r.op("ActiveItemIDs"); err != nil {
		return nil, err
	}
	return r.active[userID], nil
}

// --- Items ---

func (s *store) ItemState(_ context.Context, id string) (string, string, bool, error) {
	if err := s.op("ItemState"); err != nil {
		return "", "", false, err
	}
	row, ok := s.items[id]
	return row.owner, row.status, ok, nil
}

func (s *store) ActiveFeatures(context.Context) ([]domain.ItemFeatures, error) {
	if err := s.op("ActiveFeatures"); err != nil {
		return nil, err
	}
	var out []domain.ItemFeatures
	for _, id := range sortedIDs(s.items) {
		if s.items[id].status == "active" {
			out = append(out, s.items[id].feats)
		}
	}
	return out, nil
}

func (s *store) Briefs(_ context.Context, ids []string) (map[string]domain.ItemBrief, error) {
	if err := s.op("Briefs"); err != nil {
		return nil, err
	}
	out := map[string]domain.ItemBrief{}
	for _, id := range ids {
		if row, ok := s.items[id]; ok {
			out[id] = domain.ItemBrief{Title: row.feats.Title, Category: row.feats.Category}
		}
	}
	return out, nil
}

func (s *store) Deactivated(_ context.Context, ids []string) ([]string, error) {
	if err := s.op("Deactivated"); err != nil {
		return nil, err
	}
	var out []string
	for _, id := range ids {
		if s.gone[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// --- Model ---

func (s *store) Save(_ context.Context, buildID string, n map[string][]domain.Neighbor, p map[string][]string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.op("Save"); err != nil {
		return err
	}
	s.neighbors[buildID], s.popular[buildID] = n, p
	return nil
}

func (s *store) DeleteOtherBuilds(_ context.Context, keep string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.op("DeleteOtherBuilds"); err != nil {
		return err
	}
	for b := range s.neighbors {
		if b != keep {
			delete(s.neighbors, b)
			delete(s.popular, b)
		}
	}
	return nil
}

func (s *store) Neighbors(_ context.Context, buildID string, ids []string) (map[string][]domain.Neighbor, error) {
	if err := s.op("Neighbors"); err != nil {
		return nil, err
	}
	out := map[string][]domain.Neighbor{}
	for _, id := range ids {
		if n, ok := s.neighbors[buildID][id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

func (s *store) Popular(_ context.Context, buildID string, keys []string) (map[string][]string, error) {
	if err := s.op("Popular"); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, k := range keys {
		if p, ok := s.popular[buildID][k]; ok {
			out[k] = p
		}
	}
	return out, nil
}

// --- Meta ---

type meta struct{ *store }

func (m meta) TryLock(_ context.Context, owner string, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.op("TryLock"); err != nil {
		return false, err
	}
	if m.lockUntil.After(now) {
		return false, nil
	}
	m.lockOwner, m.lockUntil = owner, now.Add(domain.LockTTL)
	return true, nil
}

func (m meta) Publish(_ context.Context, owner, buildID string, at time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.op("Publish"); err != nil {
		return false, err
	}
	if m.lockOwner != owner {
		return false, nil
	}
	m.current, m.builtAt, m.lockOwner, m.lockUntil = buildID, at, "", time.Time{}
	return true, nil
}

func (m meta) Unlock(_ context.Context, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.op("Unlock")
	if m.lockOwner == owner {
		m.lockOwner, m.lockUntil = "", time.Time{}
	}
	return nil
}

func (m meta) Current(context.Context) (string, time.Time, bool, error) {
	if err := m.op("Current"); err != nil {
		return "", time.Time{}, false, err
	}
	return m.current, m.builtAt, m.current != "", nil
}

func (m meta) SetReset(_ context.Context, userID string, at time.Time) error {
	if err := m.op("SetReset"); err != nil {
		return err
	}
	m.resets[userID] = at
	return nil
}

func (m meta) Resets(context.Context) (map[string]time.Time, error) {
	if err := m.op("Resets"); err != nil {
		return nil, err
	}
	return m.resets, nil
}

func (m meta) ResetAt(_ context.Context, userID string) (time.Time, bool, error) {
	if err := m.op("ResetAt"); err != nil {
		return time.Time{}, false, err
	}
	at, ok := m.resets[userID]
	return at, ok, nil
}

// --- Cards: la búsqueda solo deja ver objetos activos, ajenos y no ocultos ---

type cards struct{ *store }

func (c cards) Cards(_ context.Context, viewer string, ids []string) ([]searchusecase.CardView, error) {
	if err := c.op("Cards"); err != nil {
		return nil, err
	}
	out := []searchusecase.CardView{}
	for _, id := range ids {
		row, ok := c.items[id]
		if !ok || row.status != "active" || row.owner == viewer || c.hidden[id] {
			continue
		}
		out = append(out, searchusecase.CardView{Card: searchdomain.Card{ID: id, Title: row.feats.Title, Category: row.feats.Category}})
	}
	return out, nil
}

func sortedIDs[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// --- Servicio de prueba ---

var now0 = time.Date(2026, 11, 17, 18, 0, 0, 0, time.UTC)

type harness struct {
	*store
	svc      *Service
	launched int // recálculos perezosos lanzados
	now      time.Time
	ids      int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{store: newStore(), now: now0}
	svc, err := NewService(Deps{
		Interactions: h.store, Requests: requests{h.store}, Items: h.store, Model: h.store, Meta: meta{h.store}, Cards: cards{h.store},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Enabled: true,
		Now:   func() time.Time { return h.now },
		NewID: func() string { h.ids++; return fmt.Sprintf("id%d", h.ids) },
		Go:    func(func()) { h.launched++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	h.svc = svc
	return h
}

// publish deja un modelo vigente construido ahora.
func (h *harness) publish(neighbors map[string][]domain.Neighbor, popular map[string][]string) {
	h.neighbors["b0"], h.popular["b0"] = neighbors, popular
	h.current, h.builtAt = "b0", h.now
}

func oid(n int) string { return fmt.Sprintf("%024x", n) }
