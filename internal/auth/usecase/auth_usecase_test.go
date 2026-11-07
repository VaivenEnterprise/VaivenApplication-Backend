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

func TestNewServiceRequiresDeps(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Fatal("NewService sin dependencias debe fallar")
	}
	e := newEnv(t, func(d *Deps) { d.Bookings, d.Now, d.NewFamilyID = nil, nil, nil })
	if e.svc.d.Bookings == nil || e.svc.d.Now == nil || e.svc.d.NewFamilyID == nil {
		t.Error("NewService no puso los valores por defecto")
	}
	id, err := randomID()
	if err != nil || len(id) != 32 {
		t.Errorf("randomID = %q, %v", id, err)
	}
}

func TestRegisterOK(t *testing.T) {
	e := newEnv(t)
	res := e.mustRegister(t)

	u := res.User
	if u.Email != "ana@example.com" || u.DisplayName != "Ana" || u.Role != domain.RoleUser ||
		u.Status != domain.StatusActive || u.PasswordHash != "hash:una-contrasena-larga" || u.Version != 1 {
		t.Errorf("usuario = %+v", u)
	}
	wantNow := e.now.Truncate(time.Millisecond)
	if !u.CreatedAt.Equal(wantNow) || !u.AcceptedTermsAt.Equal(wantNow) {
		t.Errorf("fechas = %v / %v, want %v (UTC, ms)", u.CreatedAt, u.AcceptedTermsAt, wantNow)
	}
	tk := res.Tokens
	if tk.AccessToken != "access:"+u.ID+":user" || tk.RefreshToken != "rt-1" || tk.ExpiresIn != 15*time.Minute {
		t.Errorf("tokens = %+v", tk)
	}
	stored, err := e.refresh.ByHash(context.Background(), "h(rt-1)")
	if err != nil || stored.UserID != u.ID || stored.FamilyID == "" || !stored.ExpiresAt.Equal(wantNow.Add(720*time.Hour)) {
		t.Errorf("refresh guardado = %+v, %v", stored, err)
	}
	if _, ok := e.refresh.byHash["rt-1"]; ok {
		t.Error("se guardó el refresh token en claro (plan P-03)")
	}
}

func TestRegisterValidation(t *testing.T) {
	tests := []struct {
		name   string
		edit   func(*RegisterInput)
		fields []string
	}{
		{"correo inválido", func(in *RegisterInput) { in.Email = "ana" }, []string{"email"}},
		{"contraseña corta", func(in *RegisterInput) { in.Password = "corta" }, []string{"password"}},
		{"contraseña larga", func(in *RegisterInput) { in.Password = strings.Repeat("a", 73) }, []string{"password"}},
		{"nombre corto", func(in *RegisterInput) { in.DisplayName = "  A " }, []string{"displayName"}},
		{"sin términos", func(in *RegisterInput) { in.AcceptTerms = false }, []string{"acceptTerms"}},
		{"menor de edad", func(in *RegisterInput) { in.IsAdult = false }, []string{"isAdult"}},
		{"todo mal", func(in *RegisterInput) { *in = RegisterInput{} }, []string{"email", "password", "displayName", "acceptTerms", "isAdult"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			in := validRegister()
			tt.edit(&in)
			_, err := e.svc.Register(context.Background(), in)
			var de *shared.Error
			if !errors.As(err, &de) || de.Code != shared.CodeValidation {
				t.Fatalf("err = %v, want validation_failed", err)
			}
			var got []string
			for _, d := range de.Details {
				got = append(got, d.Field)
			}
			if strings.Join(got, ",") != strings.Join(tt.fields, ",") {
				t.Errorf("campos = %v, want %v", got, tt.fields)
			}
			noSecrets(t, err.Error(), in.Password)
			if len(e.users.byID) != 0 {
				t.Error("se creó la cuenta pese a la validación")
			}
		})
	}
}

func TestRegisterDuplicateEmailIgnoresCase(t *testing.T) { // CA-02
	e := newEnv(t)
	e.mustRegister(t)
	in := validRegister()
	in.Email = "ANA@EXAMPLE.COM"
	_, err := e.svc.Register(context.Background(), in)
	if !domain.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
	var de *shared.Error
	if errors.As(err, &de); de.Err != nil {
		t.Error("el error de conflicto expone la causa interna del repositorio")
	}
}

func TestRegisterFailures(t *testing.T) {
	tests := []struct {
		name string
		env  func(t *testing.T) *env
	}{
		{"hash falla", func(t *testing.T) *env { e := newEnv(t); e.hasher.failHash = errBoom; return e }},
		{"repositorio falla", func(t *testing.T) *env { e := newEnv(t); e.users.failAll = errBoom; return e }},
		{"guardar refresh falla", func(t *testing.T) *env { e := newEnv(t); e.refresh.failCreate = errBoom; return e }},
		{"token de acceso falla", func(t *testing.T) *env {
			return newEnv(t, func(d *Deps) { d.Access = fakeAccess{fail: errBoom} })
		}},
		{"generador de refresh falla", func(t *testing.T) *env {
			return newEnv(t, func(d *Deps) { d.NewRefresh = func() (string, string, error) { return "", "", errBoom } })
		}},
		{"familia falla", func(t *testing.T) *env {
			return newEnv(t, func(d *Deps) { d.NewFamilyID = func() (string, error) { return "", errBoom } })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.env(t).svc.Register(context.Background(), validRegister())
			if !errors.Is(err, errBoom) {
				t.Fatalf("err = %v, want la falla simulada envuelta", err)
			}
			var de *shared.Error
			if errors.As(err, &de) {
				t.Errorf("una falla interna no debe ser error de dominio: %v", de)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	e := newEnv(t)
	reg := e.mustRegister(t)
	ctx := context.Background()

	res, err := e.svc.Login(ctx, LoginInput{Email: " ANA@example.com", Password: "una-contrasena-larga"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.User.ID != reg.User.ID || res.Tokens.RefreshToken != "rt-2" {
		t.Errorf("Login = %+v", res)
	}
	first, _ := e.refresh.ByHash(ctx, "h(rt-1)")
	second, _ := e.refresh.ByHash(ctx, "h(rt-2)")
	if first.FamilyID == second.FamilyID {
		t.Error("cada inicio de sesión abre una familia nueva")
	}

	// RF-04: el mismo error genérico en todos los casos.
	bad := []LoginInput{
		{Email: "ana@example.com", Password: "otra-contrasena"},
		{Email: "nadie@example.com", Password: "una-contrasena-larga"},
		{Email: "", Password: "x"},
		{Email: "ana@example.com", Password: ""},
		{Email: "ana@example.com", Password: strings.Repeat("a", 1000)},
	}
	for _, in := range bad {
		_, err := e.svc.Login(ctx, in)
		if !domain.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("Login(%q) = %v, want ErrInvalidCredentials", in.Email, err)
		}
		noSecrets(t, err.Error(), in.Password)
	}
}

func TestLoginUnknownEmailStillVerifies(t *testing.T) {
	e := newEnv(t)
	before := e.hasher.verifies
	if _, err := e.svc.Login(context.Background(), LoginInput{Email: "nadie@example.com", Password: "x"}); !domain.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("err = %v", err)
	}
	if e.hasher.verifies != before+1 {
		t.Error("con un correo inexistente también se verifica (hash señuelo) para igualar el tiempo")
	}
}

func TestLoginDeactivatedAccount(t *testing.T) { // CA-05
	e := newEnv(t)
	reg := e.mustRegister(t)
	u := e.users.byID[reg.User.ID]
	u.Status = domain.StatusDeactivated
	e.users.byID[u.ID] = u
	_, err := e.svc.Login(context.Background(), LoginInput{Email: "ana@example.com", Password: "una-contrasena-larga"})
	if !domain.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginFailures(t *testing.T) {
	ctx := context.Background()
	in := LoginInput{Email: "ana@example.com", Password: "una-contrasena-larga"}

	e := newEnv(t)
	e.users.failAll = errBoom
	if _, err := e.svc.Login(ctx, in); !errors.Is(err, errBoom) {
		t.Errorf("repositorio falla: err = %v", err)
	}

	e = newEnv(t)
	e.mustRegister(t)
	e.hasher.failVerify = errBoom
	if _, err := e.svc.Login(ctx, in); !errors.Is(err, errBoom) {
		t.Errorf("verify falla: err = %v", err)
	}

	e = newEnv(t)
	e.mustRegister(t)
	e.refresh.failCreate = errBoom
	if _, err := e.svc.Login(ctx, in); !errors.Is(err, errBoom) {
		t.Errorf("sesión falla: err = %v", err)
	}

	// Si el hash señuelo no se puede calcular, el correo inexistente sigue
	// siendo un error genérico.
	e = newEnv(t)
	e.hasher.failHash = errBoom
	if _, err := e.svc.Login(ctx, LoginInput{Email: "nadie@example.com", Password: "x"}); !domain.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("señuelo falla: err = %v", err)
	}
}
