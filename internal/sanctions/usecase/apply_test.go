package usecase

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

const (
	adminID    = "6730f1c2a9b3e4d5f6a7b8c4"
	userID     = "6730f1c2a9b3e4d5f6a7b8c1"
	incidentID = "6731b0000000000000000001"
)

var now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

type log struct{ calls []string }

type fakeRepo struct {
	log   *log
	saved []domain.Sanction
	err   error
}

func (f *fakeRepo) NewID() string { return "6732c0000000000000000001" }
func (f *fakeRepo) Create(_ context.Context, s domain.Sanction) (domain.Sanction, error) {
	f.log.calls = append(f.log.calls, "create")
	if f.err != nil {
		return domain.Sanction{}, f.err
	}
	f.saved = append(f.saved, s)
	return s, nil
}

type fakeIncidents struct {
	exists bool
	err    error
}

func (f fakeIncidents) Exists(context.Context, string) (bool, error) { return f.exists, f.err }

type fakeDirectory struct {
	known bool
	err   error
}

func (f fakeDirectory) PublicUsers(_ context.Context, ids []string) (map[string]users.PublicUser, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]users.PublicUser{}
	if f.known {
		for _, id := range ids {
			out[id] = users.PublicUser{ID: id, DisplayName: "Ana"}
		}
	}
	return out, nil
}

type fakeAdmin struct {
	log   *log
	until *time.Time
	who   string
	err   error
}

func (f *fakeAdmin) Suspend(_ context.Context, id string, until time.Time) error {
	f.log.calls = append(f.log.calls, "suspend")
	f.who, f.until = id, &until
	return f.err
}
func (f *fakeAdmin) AddRating(context.Context, string, int) error {
	panic("las sanciones no tocan calificaciones")
}

type fakeNotifier struct {
	log  *log
	sent []notify.Event
	to   []string
	err  error
}

func (f *fakeNotifier) Notify(_ context.Context, to string, e notify.Event) error {
	f.log.calls = append(f.log.calls, "notify")
	f.to, f.sent = append(f.to, to), append(f.sent, e)
	return f.err
}

type env struct {
	svc  *Service
	log  *log
	repo *fakeRepo
	adm  *fakeAdmin
	not  *fakeNotifier
	logs *bytes.Buffer
	d    Deps
}

func newEnv(t *testing.T) *env {
	t.Helper()
	l := &log{}
	e := &env{log: l, repo: &fakeRepo{log: l}, adm: &fakeAdmin{log: l}, not: &fakeNotifier{log: l}, logs: &bytes.Buffer{}}
	e.d = Deps{
		Repo: e.repo, Incidents: fakeIncidents{exists: true}, Directory: fakeDirectory{known: true},
		Admin: e.adm, Notifier: e.not, Log: slog.New(slog.NewTextHandler(e.logs, nil)), Now: func() time.Time { return now },
	}
	e.rebuild(t)
	return e
}

func (e *env) rebuild(t *testing.T) {
	t.Helper()
	svc, err := NewService(e.d)
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
}

func inDays(n int) *time.Time { t := now.Add(time.Duration(n) * 24 * time.Hour); return &t }

func TestNewServiceRequiresDeps(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	e := newEnv(t)
	e.d.Now = nil
	e.rebuild(t)
	if e.svc.d.Now == nil {
		t.Error("Now por omisión")
	}
}

func TestApplySuspensionSuspendsThenSavesThenNotifies(t *testing.T) { // CA-05, CA-06
	e := newEnv(t)
	end := inDays(7)
	inc := incidentID
	s, err := e.svc.Apply(context.Background(), adminID, userID, Input{Type: domain.TypeSuspension, Reason: "Daño reiterado.", EndsAt: end, IncidentID: &inc})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(e.log.calls, ","); got != "suspend,create,notify" {
		t.Errorf("orden = %s, quiero suspend,create,notify", got)
	}
	if e.adm.who != userID || !e.adm.until.Equal(*end) {
		t.Errorf("Suspend(%s, %v)", e.adm.who, e.adm.until)
	}
	if s.Type != domain.TypeSuspension || s.UserID != userID || s.CreatedByAdminID != adminID || !s.CreatedAt.Equal(now) ||
		s.IncidentID == nil || *s.IncidentID != incidentID || len(e.repo.saved) != 1 {
		t.Errorf("sanción = %+v", s)
	}
	if e.not.to[0] != userID || e.not.sent[0].Type != EventApplied || e.not.sent[0].Data["type"] != "suspension" ||
		e.not.sent[0].Data["incidentId"] != incidentID || strings.Contains(e.not.sent[0].Body, "Daño") {
		t.Errorf("aviso = %+v a %v (el motivo no viaja en el aviso)", e.not.sent[0], e.not.to)
	}
}

func TestApplyWarningOnlyNotifies(t *testing.T) {
	e := newEnv(t)
	s, err := e.svc.Apply(context.Background(), adminID, userID, Input{Type: domain.TypeWarning, Reason: "Primera advertencia."})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(e.log.calls, ","); got != "create,notify" {
		t.Errorf("orden = %s; una advertencia no suspende", got)
	}
	if s.EndsAt != nil || s.IncidentID != nil || e.not.sent[0].Data["type"] != "warning" {
		t.Errorf("sanción = %+v aviso = %+v", s, e.not.sent[0])
	}
	if _, has := e.not.sent[0].Data["incidentId"]; has {
		t.Error("sin incidencia no hay incidentId en el aviso")
	}
}

func TestApplyRejections(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("caído")
	cases := []struct {
		name  string
		setup func(*env)
		in    Input
		check func(*testing.T, error)
	}{
		{"datos inválidos", func(*env) {}, Input{Type: "ban", Reason: "x"}, func(t *testing.T, err error) {
			if err == nil {
				t.Error("quería validation_failed")
			}
		}},
		{"suspensión sin fecha", func(*env) {}, Input{Type: domain.TypeSuspension, Reason: "Motivo válido."}, func(t *testing.T, err error) {
			if err == nil {
				t.Error("quería validation_failed")
			}
		}},
		{"persona inexistente", func(e *env) { e.d.Directory = fakeDirectory{known: false} }, Input{Type: domain.TypeWarning, Reason: "Motivo válido."}, func(t *testing.T, err error) {
			if err == nil || !strings.Contains(err.Error(), "persona") {
				t.Errorf("quería 404 de persona: %v", err)
			}
		}},
		{"incidencia inexistente", func(e *env) { e.d.Incidents = fakeIncidents{exists: false} }, Input{Type: domain.TypeWarning, Reason: "Motivo válido.", IncidentID: strPtr(incidentID)}, func(t *testing.T, err error) {
			if err == nil {
				t.Error("quería validation_failed por incidentId")
			}
		}},
		{"falla el directorio", func(e *env) { e.d.Directory = fakeDirectory{err: boom} }, Input{Type: domain.TypeWarning, Reason: "Motivo válido."}, func(t *testing.T, err error) {
			if !errors.Is(err, boom) {
				t.Errorf("err = %v", err)
			}
		}},
		{"falla la búsqueda de incidencia", func(e *env) { e.d.Incidents = fakeIncidents{err: boom} }, Input{Type: domain.TypeWarning, Reason: "Motivo válido.", IncidentID: strPtr(incidentID)}, func(t *testing.T, err error) {
			if !errors.Is(err, boom) {
				t.Errorf("err = %v", err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			c.setup(e)
			e.rebuild(t)
			_, err := e.svc.Apply(ctx, adminID, userID, c.in)
			c.check(t, err)
			if len(e.log.calls) != 0 {
				t.Errorf("no debe suspender, guardar ni avisar: %v", e.log.calls)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestApplyInfrastructureFailures(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("caído")
	in := Input{Type: domain.TypeSuspension, Reason: "Motivo válido.", EndsAt: inDays(3)}

	e := newEnv(t)
	e.adm.err = boom
	if _, err := e.svc.Apply(ctx, adminID, userID, in); !errors.Is(err, boom) {
		t.Errorf("Suspend: %v", err)
	}
	if got := strings.Join(e.log.calls, ","); got != "suspend" || len(e.repo.saved) != 0 {
		t.Errorf("si no se pudo suspender no queda registro ni aviso: %s", got)
	}

	e = newEnv(t)
	e.repo.err = boom
	if _, err := e.svc.Apply(ctx, adminID, userID, in); !errors.Is(err, boom) {
		t.Errorf("Create: %v", err)
	}
	if got := strings.Join(e.log.calls, ","); got != "suspend,create" {
		t.Errorf("sin guardar no se avisa: %s", got)
	}

	e = newEnv(t)
	e.not.err = boom
	if _, err := e.svc.Apply(ctx, adminID, userID, in); err != nil {
		t.Errorf("un fallo al avisar no deshace la sanción: %v", err)
	}
	if !strings.Contains(e.logs.String(), "no se pudo notificar") {
		t.Errorf("logs = %q", e.logs.String())
	}
}
