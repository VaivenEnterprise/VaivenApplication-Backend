package cloudinary

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/clock"
)

var fixedNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func newTestBuilder(t *testing.T) *URLBuilder {
	t.Helper()
	b, err := NewURLBuilder("nube-demo", "clave-publica", testSecret, clock.Fixed{T: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNewURLBuilder(t *testing.T) {
	if _, err := NewURLBuilder("nube", "clave", "", nil); !errors.Is(err, errMissingConfig) {
		t.Fatalf("err = %v, want errMissingConfig", err)
	}
	b, err := NewURLBuilder("nube", "clave", testSecret, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.clock.(clock.System); !ok {
		t.Fatalf("reloj nil debe ser clock.System, es %T", b.clock)
	}
	if b.ttl != 10*time.Minute {
		t.Fatalf("ttl = %v", b.ttl)
	}
}

func TestURLsPublic(t *testing.T) {
	b := newTestBuilder(t)
	got, err := b.URLs(domain.Image{PublicID: "vaiven/item_photo/u1/abc def", Format: "jpg", Delivery: domain.DeliveryPublic})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.URLs{
		URL:      "https://res.cloudinary.com/nube-demo/image/upload/c_limit,w_1200/vaiven/item_photo/u1/abc%20def.jpg",
		ThumbURL: "https://res.cloudinary.com/nube-demo/image/upload/c_fill,w_300,h_300/vaiven/item_photo/u1/abc%20def.jpg",
	}
	if got != want {
		t.Fatalf("URLs = %+v\nwant %+v", got, want)
	}
}

func TestURLsAuthenticated(t *testing.T) {
	b := newTestBuilder(t)
	img := domain.Image{PublicID: "vaiven/evidence/u1/x1", Format: "png", Delivery: domain.DeliveryAuthenticated}
	got, err := b.URLs(img)
	if err != nil {
		t.Fatal(err)
	}
	if got.ThumbURL != got.URL {
		t.Fatal("la miniatura privada debe ser la misma URL")
	}
	if strings.Contains(got.URL, testSecret) {
		t.Fatal("la URL contiene el secreto (CA-04)")
	}
	u, err := url.Parse(got.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "api.cloudinary.com" || u.Path != "/v1_1/nube-demo/image/download" {
		t.Fatalf("URL base inesperada: %s", got.URL)
	}
	q := u.Query()
	ts, _ := strconv.ParseInt(q.Get("timestamp"), 10, 64)
	exp, _ := strconv.ParseInt(q.Get("expires_at"), 10, 64)
	if ts != fixedNow.Unix() || exp-ts != 600 {
		t.Fatalf("timestamp=%d expires_at=%d", ts, exp)
	}
	if q.Get("type") != "authenticated" || q.Get("api_key") != "clave-publica" ||
		q.Get("public_id") != img.PublicID || q.Get("format") != "png" {
		t.Fatalf("query inesperada: %v", q)
	}
	params := map[string]string{}
	for k := range q {
		if k != "api_key" && k != "signature" {
			params[k] = q.Get(k)
		}
	}
	if want := Sign(params, testSecret); q.Get("signature") != want {
		t.Fatalf("signature = %s, want %s", q.Get("signature"), want)
	}
}

func TestURLsErrors(t *testing.T) {
	b := newTestBuilder(t)
	cases := []struct {
		img  domain.Image
		want error
	}{
		{domain.Image{Format: "jpg", Delivery: domain.DeliveryPublic}, errMissingImageData},
		{domain.Image{PublicID: "a", Delivery: domain.DeliveryPublic}, errMissingImageData},
		{domain.Image{PublicID: "a", Format: "jpg", Delivery: "otra"}, errUnknownDelivery},
	}
	for _, c := range cases {
		if _, err := b.URLs(c.img); !errors.Is(err, c.want) {
			t.Fatalf("URLs(%+v) err = %v, want %v", c.img, err, c.want)
		}
	}
}
