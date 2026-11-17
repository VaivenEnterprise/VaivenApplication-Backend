package imagefetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
)

func server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.jpg":
			w.Header().Set("Content-Type", "image/jpeg; charset=binary")
			_, _ = w.Write([]byte("\xff\xd8\xff\xe0 jpeg"))
		case "/sin-tipo.png":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 20)))
		case "/texto":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("hola"))
		case "/grande":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(make([]byte, MaxBytes+1))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetch(t *testing.T) {
	srv := server(t)
	c := srv.Client()
	img, err := Fetch(context.Background(), c, srv.URL+"/ok.jpg", true)
	if err != nil || img.MIMEType != "image/jpeg" || !strings.Contains(string(img.Data), "jpeg") {
		t.Fatalf("ok: %+v %v", img, err)
	}
	if img, err := Fetch(context.Background(), c, srv.URL+"/sin-tipo.png", true); err != nil || img.MIMEType != "image/png" {
		t.Errorf("tipo detectado: %+v %v", img, err)
	}
	for _, path := range []string{"/texto", "/grande", "/no-existe"} {
		if _, err := Fetch(context.Background(), c, srv.URL+path, true); err == nil {
			t.Errorf("%s debe fallar", path)
		}
	}
}

func TestFetchRejectsNonHTTPS(t *testing.T) {
	srv := server(t)
	for _, u := range []string{srv.URL + "/ok.jpg", "ftp://x/y", "nada", "https://"} {
		if _, err := Fetch(context.Background(), srv.Client(), u, false); err == nil || !strings.Contains(err.Error(), "https") {
			t.Errorf("%s: err = %v", u, err)
		}
	}
}

func TestFetchNetworkError(t *testing.T) {
	srv := server(t)
	url := srv.URL + "/ok.jpg"
	srv.Close()
	if _, err := Fetch(context.Background(), http.DefaultClient, url, true); !errors.Is(err, aidomain.ErrUnavailable) {
		t.Errorf("err = %v", err)
	}
}
