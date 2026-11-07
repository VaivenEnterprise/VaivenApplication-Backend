package cloudinary

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

const (
	adminTestCloud  = "nube-de-prueba"
	adminTestKey    = "llave-de-prueba"
	adminTestSecret = "secreto-de-prueba"
)

func adminTestClient(t *testing.T, h http.HandlerFunc, opts ...AdminOption) *AdminClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewAdminClient(adminTestCloud, adminTestKey, adminTestSecret,
		append([]AdminOption{WithAdminBaseURL(srv.URL + "/")}, opts...)...)
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	return c
}

func adminAssertUnavailable(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("err = %v; quiero domain.ErrUnavailable", err)
	}
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeUnavailable || string(de.Code) != "service_unavailable" {
		t.Fatalf("errors.As = %+v; quiero código service_unavailable", de)
	}
	if strings.Contains(err.Error(), adminTestSecret) {
		t.Fatalf("el error revela el secreto: %v", err)
	}
}

func TestAdminGetResourceOK(t *testing.T) {
	c := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("método = %s", r.Method)
		}
		want := "/v1_1/" + adminTestCloud + "/resources/image/authenticated/vaiven/evidence/u1/foto%20uno%3F"
		if got := r.URL.EscapedPath(); got != want {
			t.Errorf("ruta = %q; quiero %q", got, want)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != adminTestKey || pass != adminTestSecret {
			t.Errorf("autenticación básica = %q, %q, %v", user, pass, ok)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"public_id":"vaiven/evidence/u1/foto uno?","type":"authenticated",`+
			`"format":"jpg","width":800,"height":600,"bytes":123456,"otro":"ignorado"}`)
	})

	got, err := c.GetResource(context.Background(), "authenticated", "vaiven/evidence/u1/foto uno?")
	if err != nil {
		t.Fatalf("GetResource: %v", err)
	}
	want := Resource{PublicID: "vaiven/evidence/u1/foto uno?", Type: "authenticated", Format: "jpg",
		Width: 800, Height: 600, Bytes: 123456}
	if got != want {
		t.Errorf("Resource = %+v; quiero %+v", got, want)
	}
}

func TestAdminGetResourceUploadType(t *testing.T) {
	c := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/resources/image/upload/") {
			t.Errorf("ruta = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"public_id":"a","type":"upload","format":"png","width":1,"height":2,"bytes":3}`)
	})
	got, err := c.GetResource(context.Background(), "upload", "a")
	if err != nil || got.Format != "png" || got.Bytes != 3 {
		t.Fatalf("GetResource = %+v, %v", got, err)
	}
}

func TestAdminGetResourceNotFound(t *testing.T) {
	c := adminTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"Resource not found"}}`, http.StatusNotFound)
	})
	_, err := c.GetResource(context.Background(), "upload", "no/existe")
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("err = %v; quiero ErrResourceNotFound", err)
	}
	if errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("404 no debe ser service_unavailable")
	}
}

func TestAdminGetResourceUnavailableStatuses(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusServiceUnavailable,
		http.StatusBadGateway, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := adminTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			})
			_, err := c.GetResource(context.Background(), "upload", "a/b")
			adminAssertUnavailable(t, err)
		})
	}
}

func TestAdminGetResourceCredentialsRejected(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := adminTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			})
			_, err := c.GetResource(context.Background(), "upload", "a")
			adminAssertUnavailable(t, err)
			if !strings.Contains(err.Error(), "credenciales rechazadas") {
				t.Errorf("err = %v; quiero «credenciales rechazadas»", err)
			}
		})
	}
}

func TestAdminGetResourceOtherClientError(t *testing.T) {
	c := adminTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	_, err := c.GetResource(context.Background(), "upload", "a")
	if err == nil || errors.Is(err, domain.ErrUnavailable) || errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("err = %v; quiero error genérico", err)
	}
	var de *shared.Error
	if errors.As(err, &de) {
		t.Fatalf("400 no debe ser error de dominio: %v", err)
	}
}

func TestAdminGetResourceInvalidJSON(t *testing.T) {
	c := adminTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"public_id":`)
	})
	_, err := c.GetResource(context.Background(), "upload", "a")
	adminAssertUnavailable(t, err)
}

func TestAdminGetResourceTimeout(t *testing.T) {
	release := make(chan struct{})
	c := adminTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}, WithAdminHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
	defer close(release)

	start := time.Now()
	_, err := c.GetResource(context.Background(), "upload", "a")
	adminAssertUnavailable(t, err)
	if time.Since(start) > 2*time.Second {
		t.Errorf("el timeout tardó demasiado: %v", time.Since(start))
	}
}

func TestAdminGetResourceCanceledContext(t *testing.T) {
	c := adminTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{}`)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.GetResource(ctx, "upload", "a")
	adminAssertUnavailable(t, err)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; quiero context.Canceled", err)
	}
}

func TestAdminGetResourceNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()
	c, err := NewAdminClient(adminTestCloud, adminTestKey, adminTestSecret, WithAdminBaseURL(base))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetResource(context.Background(), "upload", "a")
	adminAssertUnavailable(t, err)
}

func TestAdminGetResourceInvalidInput(t *testing.T) {
	called := false
	c := adminTestClient(t, func(_ http.ResponseWriter, _ *http.Request) { called = true })
	cases := []struct{ typ, id string }{
		{"private", "a"},
		{"", "a"},
		{"upload", ""},
		{"upload", "a//b"},
		{"upload", "a/../b"},
		{"upload", "./a"},
		{"upload", "a/"},
	}
	for _, tc := range cases {
		_, err := c.GetResource(context.Background(), tc.typ, tc.id)
		if err == nil || errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("GetResource(%q, %q) = %v; quiero error de entrada", tc.typ, tc.id, err)
		}
	}
	if called {
		t.Error("no debe llamar a Cloudinary con entrada inválida")
	}
}

func TestAdminGetResourceBadBaseURL(t *testing.T) {
	c, err := NewAdminClient(adminTestCloud, adminTestKey, adminTestSecret, WithAdminBaseURL("://mal"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetResource(context.Background(), "upload", "a"); err == nil {
		t.Fatal("quiero error con URL base inválida")
	}
}

func TestNewAdminClientMissingData(t *testing.T) {
	cases := [][3]string{
		{"", adminTestKey, adminTestSecret},
		{adminTestCloud, "", adminTestSecret},
		{adminTestCloud, adminTestKey, ""},
	}
	for _, tc := range cases {
		c, err := NewAdminClient(tc[0], tc[1], tc[2])
		if err == nil || c != nil {
			t.Errorf("NewAdminClient(%q, %q, ...) = %v, %v; quiero error", tc[0], tc[1], c, err)
			continue
		}
		if strings.Contains(err.Error(), adminTestSecret) {
			t.Errorf("el error revela el secreto: %v", err)
		}
	}
}

func TestNewAdminClientDefaults(t *testing.T) {
	c, err := NewAdminClient(adminTestCloud, adminTestKey, adminTestSecret, WithAdminHTTPClient(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://api.cloudinary.com" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.httpClient == nil || c.httpClient.Timeout != 5*time.Second {
		t.Errorf("httpClient = %+v; quiero Timeout 5s", c.httpClient)
	}
}

func TestAdminClientDoesNotPrintSecret(t *testing.T) {
	c, err := NewAdminClient(adminTestCloud, adminTestKey, adminTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		out := fmt.Sprintf(verb, c)
		if strings.Contains(out, adminTestSecret) {
			t.Errorf("%s revela el secreto: %s", verb, out)
		}
		if !strings.Contains(out, adminTestCloud) {
			t.Errorf("%s = %s; quiero el cloud name", verb, out)
		}
	}
}
