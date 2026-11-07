package images

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
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/adapter/cloudinary"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/clock"
)

// testSecret es un valor de prueba de baja entropía (no es un secreto real).
const testSecret = "secreto-de-prueba"

// fakeCloudinary responde la API de administración: 200 para "ok", 404 para
// "nada" y 503 para "caido".
func fakeCloudinary(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ok"):
			_, _ = fmt.Fprint(w, `{"public_id":"x","type":"upload","format":"jpg","width":800,"height":600,"bytes":1234}`)
		case strings.HasSuffix(r.URL.Path, "/caido"):
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newService(t *testing.T) Service {
	t.Helper()
	svc, err := New(Config{
		CloudName: "nube", APIKey: "clave", APISecret: testSecret, BaseFolder: "vaiven/test",
		Clock:        clock.Fixed{T: time.Unix(1762480800, 0)},
		AdminOptions: []cloudinary.AdminOption{cloudinary.WithAdminBaseURL(fakeCloudinary(t).URL)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestFacadeEndToEnd(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	sig, err := svc.Signature(ctx, "u1", domain.PurposeItemPhoto)
	if err != nil || !strings.HasPrefix(sig.PublicID, "vaiven/test/item_photo/u1/") || sig.Signature == "" ||
		strings.Contains(fmt.Sprintf("%+v", sig), testSecret) {
		t.Fatalf("Signature = %+v, %v", sig, err)
	}

	ok := domain.NewPublicID("vaiven/test", domain.PurposeItemPhoto, "u1", "ok")
	imgs, err := svc.Resolve(ctx, "u1", domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: ok}})
	if err != nil || len(imgs) != 1 || imgs[0].Bytes != 1234 || imgs[0].Delivery != domain.DeliveryPublic {
		t.Fatalf("Resolve = %+v, %v", imgs, err)
	}
	u, err := svc.URLs(imgs[0])
	if err != nil || !strings.Contains(u.URL, "/image/upload/c_limit,w_1200/"+ok+".jpg") {
		t.Errorf("URLs = %+v, %v", u, err)
	}

	var de *shared.Error
	nada := domain.NewPublicID("vaiven/test", domain.PurposeItemPhoto, "u1", "nada")
	if _, err := svc.Resolve(ctx, "u1", domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: nada}}); !errors.As(err, &de) || de.Code != shared.CodeValidation {
		t.Errorf("inexistente = %v, want validation_failed", err)
	}
	caido := domain.NewPublicID("vaiven/test", domain.PurposeItemPhoto, "u1", "caido")
	if _, err := svc.Resolve(ctx, "u1", domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: caido}}); !errors.As(err, &de) || de.Code != shared.CodeUnavailable {
		t.Errorf("Cloudinary caído = %v, want service_unavailable", err)
	}
}

func TestNewRequiresConfig(t *testing.T) {
	for name, cfg := range map[string]Config{
		"sin nube":    {APIKey: "k", APISecret: testSecret, BaseFolder: "b"},
		"sin secreto": {CloudName: "n", APIKey: "k", BaseFolder: "b"},
		"sin carpeta": {CloudName: "n", APIKey: "k", APISecret: testSecret},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: New debe fallar", name)
		} else if strings.Contains(err.Error(), testSecret) {
			t.Errorf("%s: el error revela el secreto", name)
		}
	}
}

func TestConfigHidesSecret(t *testing.T) {
	cfg := Config{CloudName: "nube", APIKey: "clave", APISecret: testSecret, BaseFolder: "b"}
	for _, out := range []string{fmt.Sprintf("%v", cfg), fmt.Sprintf("%+v", cfg), fmt.Sprintf("%#v", cfg), cfg.String()} {
		if strings.Contains(out, testSecret) || !strings.Contains(out, "[REDACTED]") {
			t.Errorf("la configuración impresa revela el secreto: %s", out)
		}
	}
}
