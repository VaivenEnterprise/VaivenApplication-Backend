package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// Spec 018, C-12 y C-02: revocación inmediata y eventos de seguridad.

type fakeRevocations struct {
	sessions []string
	users    []string
	ttl      time.Duration
	fail     error
}

func (f *fakeRevocations) RevokeSession(_ context.Context, sid string, ttl time.Duration) error {
	if f.fail != nil {
		return f.fail
	}
	f.sessions = append(f.sessions, sid)
	f.ttl = ttl
	return nil
}

func (f *fakeRevocations) RevokeUserBefore(_ context.Context, userID string, _ time.Time, ttl time.Duration) error {
	if f.fail != nil {
		return f.fail
	}
	f.users = append(f.users, userID)
	f.ttl = ttl
	return nil
}

type recordedEvents []SecurityEvent

func (r *recordedEvents) Record(_ context.Context, e SecurityEvent) { *r = append(*r, e) }

func (r recordedEvents) names() string {
	var n []string
	for _, e := range r {
		n = append(n, e.Name)
	}
	return strings.Join(n, ",")
}

func withRevocation(rev *fakeRevocations, ev *recordedEvents) func(*Deps) {
	return func(d *Deps) {
		d.Revocations = rev
		d.Events = ev
	}
}

func TestLogoutRevokesSessionAccessImmediately(t *testing.T) {
	rev, ev := &fakeRevocations{}, &recordedEvents{}
	e := newEnv(t, withRevocation(rev, ev))
	reg := e.mustRegister(t)
	if err := e.svc.Logout(context.Background(), reg.Tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if len(rev.sessions) != 1 || rev.sessions[0] == "" || rev.ttl != 15*time.Minute {
		t.Errorf("revocación de sesión = %+v", rev)
	}
	// Un logout repetido o con un token desconocido no revoca nada más.
	_ = e.svc.Logout(context.Background(), "desconocido")
	if len(rev.sessions) != 1 {
		t.Errorf("logout de token desconocido revocó: %v", rev.sessions)
	}
}

func TestLogoutFailsIfAccessCannotBeRevoked(t *testing.T) {
	boom := errors.New("redis caído")
	rev := &fakeRevocations{fail: boom}
	e := newEnv(t, withRevocation(rev, &recordedEvents{}))
	reg := e.mustRegister(t)
	if err := e.svc.Logout(context.Background(), reg.Tokens.RefreshToken); !errors.Is(err, boom) {
		t.Errorf("logout sin poder revocar el acceso = %v; no debe fingir éxito", err)
	}
}

func TestRefreshReuseRevokesAccessAndEmitsEvents(t *testing.T) {
	rev, ev := &fakeRevocations{}, &recordedEvents{}
	e := newEnv(t, withRevocation(rev, ev))
	reg := e.mustRegister(t)
	ctx := context.Background()
	if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if len(rev.sessions) != 0 || len(*ev) != 0 {
		t.Fatalf("una rotación normal no revoca ni alerta: %v %v", rev.sessions, ev.names())
	}
	if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); !domain.Is(err, domain.ErrInvalidSession) {
		t.Fatalf("reutilizar = %v", err)
	}
	if len(rev.sessions) != 1 {
		t.Errorf("la reutilización revoca el acceso de la sesión: %v", rev.sessions)
	}
	if ev.names() != "auth.refresh_reuse,auth.family_revoked" {
		t.Errorf("eventos = %s", ev.names())
	}
	for _, x := range *ev {
		if x.Subject != "user:"+reg.User.ID || strings.Contains(x.Subject, "@") {
			t.Errorf("sujeto = %q", x.Subject)
		}
	}
	if (*ev)[0].Severity != "high" {
		t.Errorf("severidad de la reutilización = %s", (*ev)[0].Severity)
	}
}

func TestDeactivateRevokesAllAccess(t *testing.T) {
	rev := &fakeRevocations{}
	e := newEnv(t, withRevocation(rev, &recordedEvents{}))
	reg := e.mustRegister(t)
	if err := e.svc.Deactivate(context.Background(), reg.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatal(err)
	}
	if len(rev.users) != 1 || rev.users[0] != reg.User.ID || rev.ttl != 15*time.Minute {
		t.Errorf("revocación por usuario = %+v", rev)
	}
	rev2 := &fakeRevocations{fail: errBoom}
	e2 := newEnv(t, withRevocation(rev2, &recordedEvents{}))
	reg2 := e2.mustRegister(t)
	if err := e2.svc.Deactivate(context.Background(), reg2.User.ID, "una-contrasena-larga"); !errors.Is(err, errBoom) {
		t.Errorf("baja sin poder revocar el acceso = %v", err)
	}
}

func TestLoginFailedEvents(t *testing.T) {
	ev := &recordedEvents{}
	e := newEnv(t, withRevocation(&fakeRevocations{}, ev))
	reg := e.mustRegister(t)
	ctx := context.Background()
	_, _ = e.svc.Login(ctx, LoginInput{Email: "ana@example.com", Password: "incorrecta"})
	_, _ = e.svc.Login(ctx, LoginInput{Email: "nadie@example.com", Password: "incorrecta"})
	_, _ = e.svc.Login(ctx, LoginInput{Email: "", Password: ""})
	if ev.names() != "auth.login_failed,auth.login_failed,auth.login_failed" {
		t.Fatalf("eventos = %s", ev.names())
	}
	if (*ev)[0].Subject != "user:"+reg.User.ID || (*ev)[1].Subject != "unknown" || (*ev)[2].Subject != "unknown" {
		t.Errorf("sujetos = %+v", *ev)
	}
	if _, err := e.svc.Login(ctx, LoginInput{Email: "ana@example.com", Password: "una-contrasena-larga"}); err != nil {
		t.Fatal(err)
	}
	if len(*ev) != 3 {
		t.Error("un login correcto no emite login_failed")
	}
}

func TestWithoutRevocationsBehavesAsBefore(t *testing.T) {
	e := newEnv(t) // sin Revocations ni Events
	reg := e.mustRegister(t)
	if err := e.svc.Logout(context.Background(), reg.Tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
}
