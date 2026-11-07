package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
)

func ptr(s string) *string { return &s }

func TestRefreshRotates(t *testing.T) {
	e := newEnv(t)
	reg := e.mustRegister(t)
	ctx := context.Background()

	tk, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if tk.RefreshToken == reg.Tokens.RefreshToken || tk.AccessToken == "" || tk.ExpiresIn != 15*time.Minute {
		t.Errorf("tokens = %+v", tk)
	}
	old, _ := e.refresh.ByHash(ctx, "h("+reg.Tokens.RefreshToken+")")
	nw, _ := e.refresh.ByHash(ctx, "h("+tk.RefreshToken+")")
	if old.UsedAt == nil || nw.FamilyID != old.FamilyID || !nw.Usable(e.now) {
		t.Errorf("rotación: viejo=%+v nuevo=%+v", old, nw)
	}
	// El nuevo también rota.
	if _, err := e.svc.Refresh(ctx, tk.RefreshToken); err != nil {
		t.Fatalf("segundo Refresh: %v", err)
	}
}

func TestRefreshReuseRevokesFamily(t *testing.T) { // CA-03
	e := newEnv(t)
	reg := e.mustRegister(t)
	ctx := context.Background()

	tk, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); !domain.Is(err, domain.ErrInvalidSession) {
		t.Fatalf("reutilizar = %v, want ErrInvalidSession", err)
	}
	// La familia quedó revocada: el token nuevo (legítimo) tampoco sirve.
	if _, err := e.svc.Refresh(ctx, tk.RefreshToken); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("token de la familia revocada = %v, want ErrInvalidSession", err)
	}
	// Otra sesión (otra familia) no se toca.
	other, err := e.svc.Login(ctx, LoginInput{Email: "ana@example.com", Password: "una-contrasena-larga"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, err := e.svc.Refresh(ctx, other.Tokens.RefreshToken); err != nil {
		t.Errorf("otra familia = %v, want nil", err)
	}
}

func TestRefreshInvalid(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	reg := e.mustRegister(t)

	for _, tok := range []string{"", "desconocido", strings.Repeat("a", 129)} {
		if _, err := e.svc.Refresh(ctx, tok); !domain.Is(err, domain.ErrInvalidSession) {
			t.Errorf("Refresh(%.10q) = %v", tok, err)
		}
	}

	// Vencido.
	e.now = e.now.Add(721 * time.Hour)
	if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("vencido = %v", err)
	}

	// Carrera: otra petición lo marcó primero → se revoca la familia.
	e = newEnv(t)
	reg = e.mustRegister(t)
	e.refresh.markRace = true
	if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("carrera = %v", err)
	}
	rt, _ := e.refresh.ByHash(ctx, "h("+reg.Tokens.RefreshToken+")")
	if rt.RevokedAt == nil {
		t.Error("la carrera debe revocar la familia")
	}

	// Cuenta dada de baja: la familia se revoca.
	e = newEnv(t)
	reg = e.mustRegister(t)
	u := e.users.byID[reg.User.ID]
	u.Status = domain.StatusDeactivated
	e.users.byID[u.ID] = u
	if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("cuenta de baja = %v", err)
	}

	// Cuenta inexistente.
	e = newEnv(t)
	reg = e.mustRegister(t)
	delete(e.users.byID, reg.User.ID)
	if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("cuenta inexistente = %v", err)
	}
}

func TestRefreshFailures(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(e *env){
		"buscar":  func(e *env) { e.refresh.failFind = errBoom },
		"marcar":  func(e *env) { e.refresh.failMark = errBoom },
		"usuario": func(e *env) { e.users.failAll = errBoom },
		"emitir":  func(e *env) { e.refresh.failCreate = errBoom },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			reg := e.mustRegister(t)
			breakIt(e)
			if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); !errors.Is(err, errBoom) {
				t.Errorf("err = %v, want la falla simulada", err)
			}
		})
	}
	// Si revocar falla al detectar reutilización, se informa la falla.
	e := newEnv(t)
	reg := e.mustRegister(t)
	if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	e.refresh.failRevoke = errBoom
	if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); !errors.Is(err, errBoom) {
		t.Errorf("revocar falla = %v", err)
	}
}

func TestLogout(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	reg := e.mustRegister(t)

	if err := e.svc.Logout(ctx, reg.Tokens.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := e.svc.Refresh(ctx, reg.Tokens.RefreshToken); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("tras logout el refresh = %v, want ErrInvalidSession", err)
	}
	// Idempotente.
	for _, tok := range []string{reg.Tokens.RefreshToken, "desconocido", ""} {
		if err := e.svc.Logout(ctx, tok); err != nil {
			t.Errorf("Logout(%q) = %v, want nil", tok, err)
		}
	}
	e.refresh.failFind = errBoom
	if err := e.svc.Logout(ctx, "x"); !errors.Is(err, errBoom) {
		t.Errorf("buscar falla = %v", err)
	}
	e.refresh.failFind = nil
	e.refresh.failRevoke = errBoom
	if err := e.svc.Logout(ctx, reg.Tokens.RefreshToken); !errors.Is(err, errBoom) {
		t.Errorf("revocar falla = %v", err)
	}
}

func TestMeAndProfile(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	reg := e.mustRegister(t)
	id := reg.User.ID

	me, err := e.svc.Me(ctx, id)
	if err != nil || me.Email != "ana@example.com" {
		t.Fatalf("Me = %+v, %v", me, err)
	}
	if _, err := e.svc.Me(ctx, "no-existe"); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("Me inexistente = %v", err)
	}

	u, err := e.svc.UpdateProfile(ctx, id, ProfileChanges{DisplayName: ptr("  Ana López "), Phone: ptr("+525512345678")})
	if err != nil || u.DisplayName != "Ana López" || u.Phone == nil || *u.Phone != "+525512345678" {
		t.Fatalf("UpdateProfile = %+v, %v", u, err)
	}
	u, err = e.svc.UpdateProfile(ctx, id, ProfileChanges{ClearPhone: true})
	if err != nil || u.Phone != nil || u.DisplayName != "Ana López" {
		t.Fatalf("borrar teléfono = %+v, %v", u, err)
	}

	for name, in := range map[string]ProfileChanges{
		"vacío":          {},
		"nombre corto":   {DisplayName: ptr(" A ")},
		"teléfono malo":  {Phone: ptr("123")},
		"nombre y tel. ": {DisplayName: ptr(strings.Repeat("a", 41)), Phone: ptr("x")},
	} {
		var de *shared.Error
		if _, err := e.svc.UpdateProfile(ctx, id, in); !errors.As(err, &de) || de.Code != shared.CodeValidation {
			t.Errorf("%s: err = %v, want validation_failed", name, err)
		}
	}
	if _, err := e.svc.UpdateProfile(ctx, "no-existe", ProfileChanges{DisplayName: ptr("Ana")}); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("perfil inexistente = %v", err)
	}

	e.users.failAll = errBoom
	if _, err := e.svc.Me(ctx, id); !errors.Is(err, errBoom) {
		t.Errorf("Me falla = %v", err)
	}
	if _, err := e.svc.UpdateProfile(ctx, id, ProfileChanges{DisplayName: ptr("Ana")}); !errors.Is(err, errBoom) {
		t.Errorf("UpdateProfile falla = %v", err)
	}
	if _, err := e.svc.PublicProfile(ctx, id); !errors.Is(err, errBoom) {
		t.Errorf("PublicProfile falla = %v", err)
	}
}

func TestPublicProfile(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	reg := e.mustRegister(t)

	p, err := e.svc.PublicProfile(ctx, reg.User.ID)
	if err != nil || p.DisplayName != "Ana" || p.RatingAverage != nil || p.RatingCount != 0 || !p.MemberSince.Equal(reg.User.CreatedAt) {
		t.Fatalf("PublicProfile = %+v, %v", p, err)
	}
	if _, err := e.svc.PublicProfile(ctx, "no-existe"); !domain.Is(err, domain.ErrUserNotFound) {
		t.Errorf("inexistente = %v, want ErrUserNotFound", err)
	}
	if err := e.svc.Deactivate(ctx, reg.User.ID, "una-contrasena-larga"); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if p, _ := e.svc.PublicProfile(ctx, reg.User.ID); p.DisplayName != domain.DeletedDisplayName {
		t.Errorf("tras la baja = %q, want %q", p.DisplayName, domain.DeletedDisplayName)
	}
}

func TestDeactivate(t *testing.T) { // CA-05, POL-17
	ctx := context.Background()
	e := newEnv(t)
	reg := e.mustRegister(t)
	id := reg.User.ID
	if _, err := e.svc.UpdateProfile(ctx, id, ProfileChanges{Phone: ptr("5512345678")}); err != nil {
		t.Fatal(err)
	}

	if err := e.svc.Deactivate(ctx, id, "otra"); !domain.Is(err, domain.ErrWrongPassword) {
		t.Fatalf("contraseña incorrecta = %v, want ErrWrongPassword", err)
	}
	if err := e.svc.Deactivate(ctx, id, "una-contrasena-larga"); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	u := e.users.byID[id]
	if u.Email != domain.AnonymizedEmail(id) || u.DisplayName != domain.DeletedDisplayName || u.Phone != nil ||
		u.Status != domain.StatusDeactivated || u.DeactivatedAt == nil {
		t.Errorf("cuenta tras la baja = %+v", u)
	}
	rt, _ := e.refresh.ByHash(ctx, "h("+reg.Tokens.RefreshToken+")")
	if rt.RevokedAt == nil {
		t.Error("la baja debe revocar las sesiones")
	}
	if _, err := e.svc.Login(ctx, LoginInput{Email: "ana@example.com", Password: "una-contrasena-larga"}); !domain.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("login tras la baja = %v", err)
	}
	if _, err := e.svc.Me(ctx, id); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("Me tras la baja = %v", err)
	}
	if err := e.svc.Deactivate(ctx, id, "una-contrasena-larga"); !domain.Is(err, domain.ErrInvalidSession) {
		t.Errorf("segunda baja = %v", err)
	}
	// El correo original queda libre para una cuenta nueva.
	if _, err := e.svc.Register(ctx, validRegister()); err != nil {
		t.Errorf("registrar de nuevo el correo = %v", err)
	}
}

func TestDeactivateBlockedByBookings(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, func(d *Deps) { d.Bookings = fakeBookings{active: true} })
	reg := e.mustRegister(t)
	if err := e.svc.Deactivate(ctx, reg.User.ID, "una-contrasena-larga"); !domain.Is(err, domain.ErrActiveBookings) {
		t.Fatalf("err = %v, want ErrActiveBookings", err)
	}
	if !e.users.byID[reg.User.ID].IsActive() {
		t.Error("con reservas activas la cuenta no se da de baja")
	}

	e = newEnv(t, func(d *Deps) { d.Bookings = fakeBookings{err: errBoom} })
	reg = e.mustRegister(t)
	if err := e.svc.Deactivate(ctx, reg.User.ID, "una-contrasena-larga"); !errors.Is(err, errBoom) {
		t.Errorf("reservas falla = %v", err)
	}
}

func TestDeactivateFailures(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	reg := e.mustRegister(t)
	e.hasher.failVerify = errBoom
	if err := e.svc.Deactivate(ctx, reg.User.ID, "x"); !errors.Is(err, errBoom) {
		t.Errorf("verify falla = %v", err)
	}

	e = newEnv(t)
	reg = e.mustRegister(t)
	e.refresh.failRevoke = errBoom
	if err := e.svc.Deactivate(ctx, reg.User.ID, "una-contrasena-larga"); !errors.Is(err, errBoom) {
		t.Errorf("revocar falla = %v", err)
	}

	// Falla al guardar la baja (el repositorio deja de responder tras leer).
	e = newEnv(t)
	reg = e.mustRegister(t)
	e.svc.d.Users = failingDeactivate{e.users}
	if err := e.svc.Deactivate(ctx, reg.User.ID, "una-contrasena-larga"); !errors.Is(err, errBoom) {
		t.Errorf("guardar baja falla = %v", err)
	}
}

// failingDeactivate falla solo en Deactivate.
type failingDeactivate struct{ *fakeUsers }

func (failingDeactivate) Deactivate(context.Context, string, string, time.Time) error { return errBoom }
