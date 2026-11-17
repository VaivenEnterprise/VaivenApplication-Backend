package cloudinary

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

// fakeSecret es un valor inventado y de baja entropía: solo sirve para comprobar
// que el secreto nunca aparezca en los errores.
const fakeSecret = "valor-de-prueba-falso"

func creds() config.Cloudinary {
	return config.Cloudinary{CloudName: "mi-nube", APIKey: "123456", APISecret: config.Secret(fakeSecret)}
}

func TestPingStatuses(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
		errHas  string
	}{
		{"200 = sin error", http.StatusOK, false, ""},
		{"401 = credenciales rechazadas", http.StatusUnauthorized, true, "credenciales rechazadas"},
		{"403 = credenciales rechazadas", http.StatusForbidden, true, "credenciales rechazadas"},
		{"500 = respuesta inesperada", http.StatusInternalServerError, true, "respuesta inesperada"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath, gotUser, gotPass string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotUser, gotPass, _ = r.BasicAuth()
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}))
			defer srv.Close()

			err := ping(context.Background(), srv.Client(), srv.URL, creds())

			if gotPath != "/v1_1/mi-nube/ping" {
				t.Errorf("ruta = %q", gotPath)
			}
			if gotUser != "123456" || gotPass != fakeSecret {
				t.Errorf("autenticación básica = %q / %q", gotUser, gotPass)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if !strings.Contains(err.Error(), tt.errHas) {
					t.Errorf("err = %q, se esperaba %q", err, tt.errHas)
				}
				if strings.Contains(err.Error(), fakeSecret) {
					t.Errorf("el error revela el secreto: %q", err)
				}
			}
		})
	}
}

func TestPingMissingCredentials(t *testing.T) {
	for _, c := range []config.Cloudinary{
		{APIKey: "k", APISecret: "s"},
		{CloudName: "n", APISecret: "s"},
		{CloudName: "n", APIKey: "k"},
	} {
		if err := Ping(context.Background(), c); err == nil {
			t.Errorf("Ping(%+v): se esperaba error sin hacer la petición", c)
		}
	}
}

func TestPingTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := ping(ctx, srv.Client(), srv.URL, creds())
	if err == nil {
		t.Fatal("se esperaba error por tiempo agotado")
	}
	if strings.Contains(err.Error(), fakeSecret) {
		t.Errorf("el error revela el secreto: %q", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("tardó %v; debe respetar el contexto", d)
	}
}

func TestPingUnreachable(t *testing.T) {
	err := ping(context.Background(), http.DefaultClient, "http://127.0.0.1:1", creds())
	if err == nil || strings.Contains(err.Error(), fakeSecret) {
		t.Errorf("err = %v", err)
	}
}

func TestPingBadURL(t *testing.T) {
	if err := ping(context.Background(), http.DefaultClient, "://mal", creds()); err == nil {
		t.Error("se esperaba error con una URL base inválida")
	}
}
