package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	chat "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	items "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/port"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

const (
	me      = "6730f1c2a9b3e4d5f6a7b8c1"
	owner   = "6730f1c2a9b3e4d5f6a7b8c2"
	other   = "6730f1c2a9b3e4d5f6a7b8c3"
	admin   = "6730f1c2a9b3e4d5f6a7b8ff"
	itemID  = "6731a0000000000000000002"
	msgID   = "6731c0000000000000000003"
	repID   = "6731e0000000000000000001"
	ghostID = "6731a0000000000000000999"
)

var t0 = time.Date(2026, 11, 17, 10, 0, 0, 999_000_000, time.UTC)

// --- dobles ---

type fakeRepo struct {
	reports  map[string]domain.Report
	order    []string
	existing *domain.Report
	err      error
	created  []domain.Report
	listArgs struct {
		status domain.Status
		after  string
		limit  int
	}
	resolved struct {
		id     string
		status domain.Status
		res    domain.Resolution
	}
	resolveErr error
}

func newRepo() *fakeRepo { return &fakeRepo{reports: map[string]domain.Report{}} }

func (f *fakeRepo) NewID() string { return repID }
func (f *fakeRepo) Create(_ context.Context, r domain.Report) (domain.Report, bool, error) {
	if f.err != nil {
		return domain.Report{}, false, f.err
	}
	if f.existing != nil {
		return *f.existing, false, nil
	}
	f.created = append(f.created, r)
	return r, true, nil
}
func (f *fakeRepo) ByID(_ context.Context, id string) (domain.Report, error) {
	if r, ok := f.reports[id]; ok {
		return r, nil
	}
	return domain.Report{}, domain.ErrReportNotFound
}
func (f *fakeRepo) List(_ context.Context, st domain.Status, after string, limit int) ([]domain.Report, error) {
	f.listArgs.status, f.listArgs.after, f.listArgs.limit = st, after, limit
	if f.err != nil {
		return nil, f.err
	}
	var out []domain.Report
	for _, id := range f.order {
		out = append(out, f.reports[id])
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (f *fakeRepo) Resolve(_ context.Context, id string, st domain.Status, res domain.Resolution) (domain.Report, error) {
	f.resolved.id, f.resolved.status, f.resolved.res = id, st, res
	if f.resolveErr != nil {
		return domain.Report{}, f.resolveErr
	}
	r := f.reports[id]
	r.Status, r.Resolution = st, &res
	return r, nil
}

type fakeItems struct {
	snaps      map[string]port.Snapshot
	blocked    []string
	blockErr   error
	snapErr    error
	snapCalled int
}

func (f *fakeItems) Block(_ context.Context, id string) error {
	if f.blockErr != nil {
		return f.blockErr
	}
	f.blocked = append(f.blocked, id)
	return nil
}
func (f *fakeItems) Snapshot(_ context.Context, id string) (port.Snapshot, error) {
	f.snapCalled++
	if f.snapErr != nil {
		return port.Snapshot{}, f.snapErr
	}
	if s, ok := f.snaps[id]; ok {
		return s, nil
	}
	return port.Snapshot{}, items.ErrNotFound
}

type fakeMessages struct {
	refs map[string]chat.MessageRef
	err  error
}

func (f fakeMessages) Reference(_ context.Context, id string) (chat.MessageRef, bool, error) {
	r, ok := f.refs[id]
	return r, ok, f.err
}

type fakeDir struct {
	m   map[string]users.PublicUser
	err error
}

func (f fakeDir) PublicUsers(context.Context, []string) (map[string]users.PublicUser, error) {
	return f.m, f.err
}

type fakeAdmin struct {
	suspended map[string]time.Time
	err       error
}

func (f *fakeAdmin) Suspend(_ context.Context, id string, until time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.suspended[id] = until
	return nil
}
func (f *fakeAdmin) AddRating(context.Context, string, int) error { return nil }

type fakeNotifier struct {
	events []struct {
		user string
		e    notify.Event
	}
	err error
}

func (f *fakeNotifier) Notify(_ context.Context, u string, e notify.Event) error {
	f.events = append(f.events, struct {
		user string
		e    notify.Event
	}{u, e})
	return f.err
}

type env struct {
	s    *Service
	repo *fakeRepo
	it   *fakeItems
	msgs fakeMessages
	adm  *fakeAdmin
	nt   *fakeNotifier
	logs *bytes.Buffer
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{
		repo: newRepo(),
		it:   &fakeItems{snaps: map[string]port.Snapshot{itemID: {ID: itemID, OwnerID: owner, Title: "Taladro", Status: "published"}}},
		msgs: fakeMessages{refs: map[string]chat.MessageRef{msgID: {ID: msgID, SenderID: owner, Participants: [2]string{me, owner}, Body: "Hola, ¿lo necesitas?"}}},
		adm:  &fakeAdmin{suspended: map[string]time.Time{}},
		nt:   &fakeNotifier{},
		logs: &bytes.Buffer{},
	}
	dir := fakeDir{m: map[string]users.PublicUser{
		me: {ID: me, DisplayName: "Ana"}, owner: {ID: owner, DisplayName: "Beto"}, other: {ID: other, DisplayName: "Carla"},
	}}
	s, err := NewService(Deps{Repo: e.repo, Items: e.it, Messages: e.msgs, Users: dir, Admin: e.adm, Notifier: e.nt,
		Log: slog.New(slog.NewTextHandler(e.logs, nil)), Now: func() time.Time { return t0 }})
	if err != nil {
		t.Fatal(err)
	}
	e.s = s
	return e
}

func isCode(err error, c shared.Code) bool {
	var de *shared.Error
	return errors.As(err, &de) && de.Code == c
}

func openReport(tt domain.TargetType, target string) domain.Report {
	return domain.Report{ID: repID, ReporterID: me, TargetType: tt, TargetID: target, Reason: domain.ReasonProhibited, Status: domain.StatusOpen, CreatedAt: t0}
}

// --- servicio ---

func TestNewServiceNeedsDeps(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	e := newEnv(t)
	s, err := NewService(Deps{Repo: e.repo, Items: e.it, Messages: e.msgs, Users: fakeDir{}, Admin: e.adm, Notifier: e.nt, Log: slog.Default()})
	if err != nil || s.d.Now == nil {
		t.Errorf("NewService = %v", err)
	}
}

// --- Create ---

func TestCreateItemReport(t *testing.T) {
	e := newEnv(t)
	d := "  Ofrece algo prohibido  "
	r, created, err := e.s.Create(context.Background(), me, Input{TargetType: "item", TargetID: itemID, Reason: "prohibited_content", Details: &d})
	if err != nil || !created {
		t.Fatalf("Create: %v created=%v", err, created)
	}
	if r.ID != repID || r.ReporterID != me || r.Status != domain.StatusOpen || *r.Details != "Ofrece algo prohibido" || !r.CreatedAt.Equal(t0.Truncate(time.Millisecond)) {
		t.Errorf("reporte = %+v", r)
	}
}

func TestCreateDuplicateReturnsExisting(t *testing.T) { // RF-08
	e := newEnv(t)
	prev := openReport(domain.TargetItem, itemID)
	prev.ID = "6731e0000000000000000009"
	e.repo.existing = &prev
	r, created, err := e.s.Create(context.Background(), me, Input{TargetType: "item", TargetID: itemID, Reason: "spam"})
	if err != nil || created || r.ID != prev.ID {
		t.Errorf("duplicado: %+v created=%v err=%v", r, created, err)
	}
}

func TestCreateMessageOnlyParticipants(t *testing.T) { // CA-06
	e := newEnv(t)
	// Participante: puede reportar el mensaje de la otra parte.
	if _, created, err := e.s.Create(context.Background(), me, Input{TargetType: "message", TargetID: msgID, Reason: "inappropriate"}); err != nil || !created {
		t.Fatalf("participante: %v created=%v", err, created)
	}
	// No participante: 404, sin revelar que existe.
	_, _, err := e.s.Create(context.Background(), other, Input{TargetType: "message", TargetID: msgID, Reason: "inappropriate"})
	if !errors.Is(err, domain.ErrTargetNotFound) || !isCode(err, shared.CodeNotFound) {
		t.Errorf("no participante = %v, quiero 404", err)
	}
	// Inexistente: mismo 404.
	if _, _, err := e.s.Create(context.Background(), me, Input{TargetType: "message", TargetID: "6731c0000000000000000999", Reason: "spam"}); !errors.Is(err, domain.ErrTargetNotFound) {
		t.Errorf("inexistente = %v", err)
	}
	// Quien lo escribió no lo reporta.
	if _, _, err := e.s.Create(context.Background(), owner, Input{TargetType: "message", TargetID: msgID, Reason: "spam"}); !errors.Is(err, domain.ErrOwnTarget) {
		t.Errorf("propio = %v", err)
	}
	if len(e.repo.created) != 1 {
		t.Errorf("solo debe existir el reporte del participante, hay %d", len(e.repo.created))
	}
}

func TestCreateTargets(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	cases := []struct {
		name string
		by   string
		in   Input
		want error
	}{
		{"usuario", me, Input{TargetType: "user", TargetID: other, Reason: "spam"}, nil},
		{"usuario inexistente", me, Input{TargetType: "user", TargetID: "6730f1c2a9b3e4d5f6a7b999", Reason: "spam"}, domain.ErrTargetNotFound},
		{"yo mismo", me, Input{TargetType: "user", TargetID: me, Reason: "spam"}, domain.ErrOwnTarget},
		{"objeto inexistente", me, Input{TargetType: "item", TargetID: ghostID, Reason: "spam"}, domain.ErrTargetNotFound},
		{"objeto propio", owner, Input{TargetType: "item", TargetID: itemID, Reason: "spam"}, domain.ErrOwnTarget},
		{"decisión de moderación", me, Input{TargetType: "moderation_decision", TargetID: "rechazo-2026-1", Reason: "other"}, nil}, // RF-09
	}
	for _, c := range cases {
		_, _, err := e.s.Create(ctx, c.by, c.in)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%s: err=%v, quiero %v", c.name, err, c.want)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	long := strings.Repeat("a", 501)
	for name, in := range map[string]Input{
		"tipo":    {TargetType: "booking", TargetID: itemID, Reason: "spam"},
		"motivo":  {TargetType: "item", TargetID: itemID, Reason: "odio"},
		"id":      {TargetType: "item", TargetID: "x", Reason: "spam"},
		"detalle": {TargetType: "item", TargetID: itemID, Reason: "spam", Details: &long},
	} {
		if _, _, err := e.s.Create(context.Background(), me, in); !isCode(err, shared.CodeValidation) {
			t.Errorf("%s: debe ser validation_failed, fue %v", name, err)
		}
	}
	if len(e.repo.created) != 0 {
		t.Error("una petición inválida no se guarda")
	}
}

func TestCreateDependencyErrors(t *testing.T) {
	boom := errors.New("boom")
	e := newEnv(t)
	e.it.snapErr = boom
	if _, _, err := e.s.Create(context.Background(), me, Input{TargetType: "item", TargetID: itemID, Reason: "spam"}); !errors.Is(err, boom) {
		t.Errorf("objeto: %v", err)
	}
	e = newEnv(t)
	e.msgs.err = boom
	e.s.d.Messages = e.msgs
	if _, _, err := e.s.Create(context.Background(), me, Input{TargetType: "message", TargetID: msgID, Reason: "spam"}); !errors.Is(err, boom) {
		t.Errorf("mensaje: %v", err)
	}
	e = newEnv(t)
	e.s.d.Users = fakeDir{err: boom}
	if _, _, err := e.s.Create(context.Background(), me, Input{TargetType: "user", TargetID: other, Reason: "spam"}); !errors.Is(err, boom) {
		t.Errorf("usuario: %v", err)
	}
	e = newEnv(t)
	e.repo.err = boom
	if _, _, err := e.s.Create(context.Background(), me, Input{TargetType: "item", TargetID: itemID, Reason: "spam"}); !errors.Is(err, boom) {
		t.Errorf("repositorio: %v", err)
	}
}

// --- List ---

func TestListQueue(t *testing.T) {
	e := newEnv(t)
	long := strings.Repeat("palabra ", 30)
	e.msgs.refs[msgID] = chat.MessageRef{ID: msgID, SenderID: owner, Participants: [2]string{me, owner}, Body: long}
	e.s.d.Messages = e.msgs
	e.repo.reports = map[string]domain.Report{
		"1": withID(openReport(domain.TargetItem, itemID), "6731e0000000000000000001"),
		"2": withID(openReport(domain.TargetMessage, msgID), "6731e0000000000000000002"),
		"3": withID(openReport(domain.TargetUser, other), "6731e0000000000000000003"),
		"4": withID(openReport(domain.TargetModerationDecision, "ref-7"), "6731e0000000000000000004"),
		"5": withID(openReport(domain.TargetItem, ghostID), "6731e0000000000000000005"),
	}
	e.repo.order = []string{"1", "2", "3", "4", "5"}
	p, err := e.s.List(context.Background(), "", "", 4)
	if err != nil {
		t.Fatal(err)
	}
	if e.repo.listArgs.status != domain.StatusOpen || e.repo.listArgs.limit != 5 {
		t.Errorf("args = %+v", e.repo.listArgs)
	}
	if len(p.Items) != 4 || p.Next == nil || *p.Next != "6731e0000000000000000004" {
		t.Fatalf("página = %d items next=%v", len(p.Items), p.Next)
	}
	if p.Items[0].TargetSummary != "Taladro" || p.Items[0].Reporter.DisplayName != "Ana" {
		t.Errorf("objeto: %+v", p.Items[0])
	}
	if s := p.Items[1].TargetSummary; !strings.HasSuffix(s, "…") || len([]rune(s)) != summaryRunes {
		t.Errorf("la vista previa debe recortarse a %d: %q", summaryRunes, s)
	}
	if p.Items[2].TargetSummary != "Carla" {
		t.Errorf("usuario: %q", p.Items[2].TargetSummary)
	}
	if p.Items[3].TargetSummary != "Decisión de moderación ref-7" {
		t.Errorf("decisión: %q", p.Items[3].TargetSummary)
	}
	last, _ := e.s.List(context.Background(), "open", "x", 10)
	if last.Next != nil || len(last.Items) != 5 || last.Items[4].TargetSummary != UnavailableSummary {
		t.Errorf("última página: next=%v n=%d resumen=%q", last.Next, len(last.Items), last.Items[len(last.Items)-1].TargetSummary)
	}
	if e.repo.listArgs.after != "x" {
		t.Error("after debe llegar al repositorio")
	}
}

func withID(r domain.Report, id string) domain.Report { r.ID = id; return r }

func TestListParamsAndErrors(t *testing.T) {
	e := newEnv(t)
	if _, err := e.s.List(context.Background(), "cerrado", "", 1); !isCode(err, shared.CodeValidation) {
		t.Errorf("estado inválido: %v", err)
	}
	_, _ = e.s.List(context.Background(), "dismissed", "", 999)
	if e.repo.listArgs.status != domain.StatusDismissed || e.repo.listArgs.limit != MaxPage+1 {
		t.Errorf("máximo: %+v", e.repo.listArgs)
	}
	_, _ = e.s.List(context.Background(), "", "", 0)
	if e.repo.listArgs.limit != DefaultPage+1 {
		t.Errorf("por omisión: %+v", e.repo.listArgs)
	}
	boom := errors.New("boom")
	e.repo.err = boom
	if _, err := e.s.List(context.Background(), "", "", 1); !errors.Is(err, boom) {
		t.Errorf("repositorio: %v", err)
	}
}

func TestListDependencyErrorsAndDeletedReporter(t *testing.T) {
	boom := errors.New("boom")
	for name, mutate := range map[string]func(e *env){
		"perfiles": func(e *env) { e.s.d.Users = fakeDir{err: boom} },
		"objeto":   func(e *env) { e.it.snapErr = boom },
		"mensaje":  func(e *env) { e.msgs.err = boom; e.s.d.Messages = e.msgs },
	} {
		e := newEnv(t)
		e.repo.reports = map[string]domain.Report{"1": openReport(domain.TargetItem, itemID), "2": openReport(domain.TargetMessage, msgID)}
		e.repo.reports["2"] = withID(e.repo.reports["2"], "6731e0000000000000000002")
		e.repo.order = []string{"1", "2"}
		mutate(e)
		if _, err := e.s.List(context.Background(), "", "", 5); !errors.Is(err, boom) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Quien reportó ya no existe: se muestra como cuenta eliminada.
	e := newEnv(t)
	r := openReport(domain.TargetModerationDecision, "r1")
	r.ReporterID = "6730f1c2a9b3e4d5f6a7b777"
	e.repo.reports = map[string]domain.Report{"1": r}
	e.repo.order = []string{"1"}
	p, err := e.s.List(context.Background(), "", "", 5)
	if err != nil || p.Items[0].Reporter.DisplayName != "Cuenta eliminada" {
		t.Errorf("reportante eliminado: %+v %v", p, err)
	}
	// Mensaje que ya no existe.
	e = newEnv(t)
	e.repo.reports = map[string]domain.Report{"1": openReport(domain.TargetMessage, "6731c0000000000000000999")}
	e.repo.order = []string{"1"}
	if p, _ := e.s.List(context.Background(), "", "", 5); p.Items[0].TargetSummary != UnavailableSummary {
		t.Errorf("mensaje faltante: %q", p.Items[0].TargetSummary)
	}
	// Persona reportada que ya no existe.
	e = newEnv(t)
	e.repo.reports = map[string]domain.Report{"1": openReport(domain.TargetUser, "6730f1c2a9b3e4d5f6a7b888")}
	e.repo.order = []string{"1"}
	if p, _ := e.s.List(context.Background(), "", "", 5); p.Items[0].TargetSummary != UnavailableSummary {
		t.Errorf("persona faltante: %q", p.Items[0].TargetSummary)
	}
}

// --- Resolve ---

func TestResolveHideItem(t *testing.T) { // CA-05, RF-10, RF-11
	e := newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetItem, itemID)
	v, err := e.s.Resolve(context.Background(), admin, repID, "hide_item", "  Incumple la política.  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.it.blocked) != 1 || e.it.blocked[0] != itemID {
		t.Errorf("Block = %v", e.it.blocked)
	}
	if e.repo.resolved.status != domain.StatusActioned || e.repo.resolved.res.Action != domain.ActionHideItem ||
		e.repo.resolved.res.Note != "Incumple la política." || e.repo.resolved.res.ByAdminID != admin || !e.repo.resolved.res.At.Equal(t0.Truncate(time.Millisecond)) {
		t.Errorf("resolución = %+v", e.repo.resolved)
	}
	if len(e.nt.events) != 1 || e.nt.events[0].user != owner || e.nt.events[0].e.Type != notify.TypeContentRemoved || e.nt.events[0].e.Data["itemId"] != itemID {
		t.Errorf("avisos = %+v", e.nt.events)
	}
	if v.Report.Status != domain.StatusActioned || v.TargetSummary != "Taladro" || v.Reporter.DisplayName != "Ana" {
		t.Errorf("vista = %+v", v)
	}
}

func TestResolveSuspendUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	want := t0.Truncate(time.Millisecond).Add(7 * 24 * time.Hour)
	cases := map[string]struct {
		rep      domain.Report
		offender string
	}{
		"persona": {openReport(domain.TargetUser, other), other},
		"objeto":  {openReport(domain.TargetItem, itemID), owner},
		"mensaje": {openReport(domain.TargetMessage, msgID), owner},
	}
	for name, c := range cases {
		e.repo.reports[repID] = c.rep
		e.nt.events = nil
		if _, err := e.s.Resolve(ctx, admin, repID, "suspend_user", "Reincidente"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := e.adm.suspended[c.offender]; !got.Equal(want) {
			t.Errorf("%s: suspendido hasta %v, quiero %v", name, got, want)
		}
		if len(e.nt.events) != 1 || e.nt.events[0].user != c.offender || e.nt.events[0].e.Type != notify.TypeSanctionApplied {
			t.Errorf("%s: avisos = %+v", name, e.nt.events)
		}
		if e.repo.resolved.status != domain.StatusActioned {
			t.Errorf("%s: estado final %s", name, e.repo.resolved.status)
		}
	}
	if len(e.it.blocked) != 0 {
		t.Error("suspend_user no bloquea el objeto")
	}
}

func TestResolveDismiss(t *testing.T) {
	e := newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetModerationDecision, "ref-1")
	if _, err := e.s.Resolve(context.Background(), admin, repID, "dismiss", "Sin falta"); err != nil {
		t.Fatal(err)
	}
	if e.repo.resolved.status != domain.StatusDismissed || len(e.nt.events) != 0 || len(e.it.blocked) != 0 || len(e.adm.suspended) != 0 {
		t.Errorf("dismiss no debe actuar sobre nada: %+v", e)
	}
}

func TestResolveValidationAndStateErrors(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.repo.reports[repID] = openReport(domain.TargetItem, itemID)
	if _, err := e.s.Resolve(ctx, admin, repID, "ban", "nota valida"); !isCode(err, shared.CodeValidation) {
		t.Errorf("acción inválida: %v", err)
	}
	if _, err := e.s.Resolve(ctx, admin, repID, "dismiss", "x"); !isCode(err, shared.CodeValidation) {
		t.Errorf("nota corta: %v", err)
	}
	if _, err := e.s.Resolve(ctx, admin, "6731e0000000000000000777", "dismiss", "nota valida"); !errors.Is(err, domain.ErrReportNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	closed := openReport(domain.TargetItem, itemID)
	closed.Status = domain.StatusDismissed
	e.repo.reports["6731e0000000000000000008"] = withID(closed, "6731e0000000000000000008")
	if _, err := e.s.Resolve(ctx, admin, "6731e0000000000000000008", "hide_item", "nota valida"); !errors.Is(err, domain.ErrAlreadyResolved) {
		t.Errorf("ya resuelto: %v", err)
	}
	// Acciones que no aplican al tipo.
	e.repo.reports[repID] = openReport(domain.TargetUser, other)
	if _, err := e.s.Resolve(ctx, admin, repID, "hide_item", "nota valida"); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Errorf("hide_item sobre usuario: %v", err)
	}
	e.repo.reports[repID] = openReport(domain.TargetModerationDecision, "r")
	if _, err := e.s.Resolve(ctx, admin, repID, "suspend_user", "nota valida"); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Errorf("suspend_user sobre decisión: %v", err)
	}
	if len(e.it.blocked) != 0 || len(e.adm.suspended) != 0 || e.repo.resolved.id != "" {
		t.Error("ninguna resolución inválida debe tocar nada")
	}
}

func TestResolveActionFailures(t *testing.T) {
	boom := errors.New("boom")
	ctx := context.Background()

	// El objeto ya no existe: no se puede ocultar (404) y el reporte sigue abierto.
	e := newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetItem, ghostID)
	if _, err := e.s.Resolve(ctx, admin, repID, "hide_item", "nota valida"); !errors.Is(err, domain.ErrTargetNotFound) || e.repo.resolved.id != "" {
		t.Errorf("hide_item sobre objeto eliminado: %v resolved=%q", err, e.repo.resolved.id)
	}
	// Block falla.
	e = newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetItem, itemID)
	e.it.blockErr = boom
	if _, err := e.s.Resolve(ctx, admin, repID, "hide_item", "nota valida"); !errors.Is(err, boom) || e.repo.resolved.id != "" {
		t.Errorf("Block falla: %v", err)
	}
	e.it.blockErr = items.ErrNotFound
	if _, err := e.s.Resolve(ctx, admin, repID, "hide_item", "nota valida"); !errors.Is(err, domain.ErrTargetNotFound) {
		t.Errorf("Block not_found: %v", err)
	}
	// Snapshot falla con error interno.
	e = newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetItem, itemID)
	e.it.snapErr = boom
	if _, err := e.s.Resolve(ctx, admin, repID, "hide_item", "nota valida"); !errors.Is(err, boom) {
		t.Errorf("Snapshot falla: %v", err)
	}
	if _, err := e.s.Resolve(ctx, admin, repID, "suspend_user", "nota valida"); !errors.Is(err, boom) {
		t.Errorf("Snapshot falla (suspend): %v", err)
	}
	// Suspend falla / persona inexistente.
	e = newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetUser, other)
	e.adm.err = boom
	if _, err := e.s.Resolve(ctx, admin, repID, "suspend_user", "nota valida"); !errors.Is(err, boom) {
		t.Errorf("Suspend falla: %v", err)
	}
	e.adm.err = shared.NewNotFound("No encontramos a esa persona.")
	if _, err := e.s.Resolve(ctx, admin, repID, "suspend_user", "nota valida"); !errors.Is(err, domain.ErrTargetNotFound) {
		t.Errorf("Suspend not_found: %v", err)
	}
	// Mensaje: lectura falla o ya no existe.
	e = newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetMessage, msgID)
	e.msgs.err = boom
	e.s.d.Messages = e.msgs
	if _, err := e.s.Resolve(ctx, admin, repID, "suspend_user", "nota valida"); !errors.Is(err, boom) {
		t.Errorf("Reference falla: %v", err)
	}
	e = newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetMessage, "6731c0000000000000000999")
	if _, err := e.s.Resolve(ctx, admin, repID, "suspend_user", "nota valida"); !errors.Is(err, domain.ErrTargetNotFound) {
		t.Errorf("mensaje faltante: %v", err)
	}
}

func TestResolveRaceAndNotifierFailure(t *testing.T) {
	ctx := context.Background()
	// Otro administrador cerró el reporte entre la lectura y el cierre: 409.
	e := newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetItem, itemID)
	e.repo.resolveErr = domain.ErrAlreadyResolved
	if _, err := e.s.Resolve(ctx, admin, repID, "hide_item", "nota valida"); !errors.Is(err, domain.ErrAlreadyResolved) {
		t.Errorf("carrera: %v", err)
	}
	// Un aviso que falla no deshace la decisión y se registra sin datos personales.
	e = newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetItem, itemID)
	e.nt.err = errors.New("push caído")
	if _, err := e.s.Resolve(ctx, admin, repID, "hide_item", "nota valida"); err != nil {
		t.Fatalf("el fallo del aviso no debe fallar la resolución: %v", err)
	}
	if !strings.Contains(e.logs.String(), "no se pudo enviar el aviso") || strings.Contains(e.logs.String(), owner) {
		t.Errorf("registro = %s", e.logs)
	}
	// Fallo al armar la vista de respuesta.
	e = newEnv(t)
	e.repo.reports[repID] = openReport(domain.TargetModerationDecision, "r")
	e.s.d.Users = fakeDir{err: fmt.Errorf("boom")}
	if _, err := e.s.Resolve(ctx, admin, repID, "dismiss", "nota valida"); err == nil {
		t.Error("el error al leer perfiles debe subir")
	}
}

func TestPreviewShortTextIsKept(t *testing.T) {
	if got := preview("  hola  "); got != "hola" {
		t.Errorf("preview = %q", got)
	}
	if got := preview(strings.Repeat("ñ", summaryRunes)); len([]rune(got)) != summaryRunes || strings.HasSuffix(got, "…") {
		t.Error("un texto de exactamente 80 no se recorta")
	}
}
