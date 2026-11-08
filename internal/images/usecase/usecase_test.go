package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

const base = "vaiven/staging"

type fakeSigner struct{ got map[string]string }

func (*fakeSigner) UploadURL() string { return "https://api.cloudinary.com/v1_1/nube/image/upload" }
func (*fakeSigner) APIKey() string    { return "clave-publica" }
func (f *fakeSigner) SignUpload(p map[string]string) string {
	f.got = p
	return "firma(" + p["public_id"] + ")"
}

// fakeResources responde por publicId; err se aplica a todos si no es nil.
type fakeResources struct {
	byID  map[string]ResourceInfo
	types []string
	err   error
}

func (f *fakeResources) Lookup(_ context.Context, typ, id string) (ResourceInfo, error) {
	f.types = append(f.types, typ)
	if f.err != nil {
		return ResourceInfo{}, f.err
	}
	info, ok := f.byID[id]
	if !ok {
		return ResourceInfo{}, ErrResourceNotFound
	}
	return info, nil
}

type fakeURLs struct{ err error }

func (f fakeURLs) URLs(img domain.Image) (domain.URLs, error) {
	return domain.URLs{URL: "u:" + img.PublicID, ThumbURL: "t:" + img.PublicID}, f.err
}

func newSvc(t *testing.T, res *fakeResources, mutate ...func(*Deps)) (*Service, *fakeSigner) {
	t.Helper()
	sg := &fakeSigner{}
	d := Deps{
		Signer: sg, Resources: res, URLs: fakeURLs{}, BaseFolder: "/" + base + "/",
		Now:   func() time.Time { return time.Unix(1762480800, 0) },
		NewID: func() (string, error) { return "id1", nil },
	}
	for _, m := range mutate {
		m(&d)
	}
	s, err := NewService(d)
	if err != nil {
		t.Fatal(err)
	}
	return s, sg
}

func pid(p domain.Purpose, user, id string) string { return domain.NewPublicID(base, p, user, id) }

func ok(format string, bytes int64) ResourceInfo {
	return ResourceInfo{Format: format, Width: 800, Height: 600, Bytes: bytes}
}

func details(t *testing.T, err error) map[string]string {
	t.Helper()
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeValidation {
		t.Fatalf("err = %v, want validation_failed", err)
	}
	out := map[string]string{}
	for _, d := range de.Details {
		out[d.Field] = d.Issue
	}
	return out
}

func TestNewService(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	if _, err := NewService(Deps{Signer: &fakeSigner{}, Resources: &fakeResources{}, URLs: fakeURLs{}, BaseFolder: "/"}); err == nil {
		t.Error("carpeta base vacía debe fallar")
	}
	s, err := NewService(Deps{Signer: &fakeSigner{}, Resources: &fakeResources{}, URLs: fakeURLs{}, BaseFolder: base})
	if err != nil || s.d.MaxBytes != domain.DefaultMaxBytes || s.d.Now == nil || s.d.NewID == nil {
		t.Errorf("valores por defecto: %+v, %v", s, err)
	}
	if id, err := randomID(); err != nil || len(id) != 32 {
		t.Errorf("randomID = %q, %v", id, err)
	}
}

func TestSignature(t *testing.T) { // CA-01, RF-02, RF-03
	s, sg := newSvc(t, &fakeResources{})
	sig, err := s.Signature(context.Background(), "u1", domain.PurposeItemPhoto)
	if err != nil {
		t.Fatal(err)
	}
	if sig.PublicID != "vaiven/staging/item_photo/u1/id1" || sig.Type != "upload" || sig.Timestamp != 1762480800 ||
		sig.AllowedFormats != "jpg,jpeg,png,webp" || sig.APIKey != "clave-publica" || !strings.HasSuffix(sig.UploadURL, "/image/upload") ||
		sig.Signature != "firma("+sig.PublicID+")" {
		t.Errorf("firma = %+v", sig)
	}
	want := map[string]string{"allowed_formats": "jpg,jpeg,png,webp", "overwrite": "false", "public_id": sig.PublicID, "timestamp": "1762480800", "type": "upload"}
	if fmt.Sprint(sg.got) != fmt.Sprint(want) {
		t.Errorf("parámetros firmados = %v, want %v", sg.got, want)
	}

	ev, err := s.Signature(context.Background(), "u1", domain.PurposeEvidence)
	if err != nil || ev.Type != "authenticated" || !strings.Contains(ev.PublicID, "/evidence/u1/") || sg.got["type"] != "authenticated" {
		t.Errorf("evidencia = %+v, %v", ev, err)
	}

	if _, err := s.Signature(context.Background(), "u1", "avatar"); !errors.Is(err, domain.ErrInvalidPurpose) {
		t.Errorf("propósito inválido = %v", err)
	}
	var de *shared.Error
	if _, err := s.Signature(context.Background(), "", domain.PurposeItemPhoto); !errors.As(err, &de) || de.Code != shared.CodeUnauthorized {
		t.Errorf("sin usuario = %v", err)
	}
	bad, _ := newSvc(t, &fakeResources{}, func(d *Deps) { d.NewID = func() (string, error) { return "", errors.New("sin azar") } })
	if _, err := bad.Signature(context.Background(), "u1", domain.PurposeItemPhoto); err == nil {
		t.Error("falla del generador debe propagarse")
	}
}

func TestResolveOK(t *testing.T) {
	a, b := pid(domain.PurposeItemPhoto, "u1", "a"), pid(domain.PurposeItemPhoto, "u1", "b")
	res := &fakeResources{byID: map[string]ResourceInfo{a: ok("jpg", 1000), b: ok("webp", domain.DefaultMaxBytes)}}
	s, _ := newSvc(t, res)
	imgs, err := s.Resolve(context.Background(), "u1", domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: a}, {PublicID: b}})
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 2 || imgs[0] != (domain.Image{PublicID: a, Width: 800, Height: 600, Format: "jpg", Bytes: 1000, Delivery: domain.DeliveryPublic}) ||
		imgs[1].Format != "webp" {
		t.Errorf("imágenes = %+v", imgs)
	}
	if strings.Join(res.types, ",") != "upload,upload" {
		t.Errorf("tipos consultados = %v", res.types)
	}

	// Evidencia: se consulta como authenticated y se guarda así.
	e := pid(domain.PurposeEvidence, "u1", "e")
	res = &fakeResources{byID: map[string]ResourceInfo{e: ok("png", 10)}}
	s, _ = newSvc(t, res)
	imgs, err = s.Resolve(context.Background(), "u1", domain.PurposeEvidence, "", []domain.Ref{{PublicID: e}})
	if err != nil || imgs[0].Delivery != domain.DeliveryAuthenticated || res.types[0] != "authenticated" {
		t.Errorf("evidencia = %+v, %v (tipos %v)", imgs, err, res.types)
	}
}

func TestResolveRejects(t *testing.T) { // CA-02, CA-03, POL-13
	mine := func(id string) string { return pid(domain.PurposeItemPhoto, "u1", id) }
	res := &fakeResources{byID: map[string]ResourceInfo{
		mine("ok"):    ok("jpg", 100),
		mine("gif"):   ok("gif", 100),
		mine("big"):   ok("png", domain.DefaultMaxBytes+1),
		mine("cero"):  ok("png", 0),
		mine("ajena"): ok("jpg", 100),
	}}
	s, _ := newSvc(t, res)
	ctx := context.Background()
	refs := func(ids ...string) []domain.Ref {
		var out []domain.Ref
		for _, id := range ids {
			out = append(out, domain.Ref{PublicID: id})
		}
		return out
	}

	tests := []struct {
		name  string
		refs  []domain.Ref
		field string
		issue string
	}{
		{"ninguna", nil, "photos", "envía de 1 a 5 imágenes"},
		{"seis", refs(mine("1"), mine("2"), mine("3"), mine("4"), mine("5"), mine("6")), "photos", "envía de 1 a 5 imágenes"},
		{"de otra persona", refs(pid(domain.PurposeItemPhoto, "u2", "x")), "photos[0].publicId", "no es tuya"},
		{"de otro propósito", refs(pid(domain.PurposeEvidence, "u1", "x")), "photos[0].publicId", "no es tuya"},
		{"repetida", refs(mine("ok"), mine("ok")), "photos[1].publicId", "repetida"},
		{"no existe", refs(mine("nada")), "photos[0].publicId", "no se subió"},
		{"formato", refs(mine("ok"), mine("gif")), "photos[1].publicId", "formato no permitido"},
		{"más de 5 MB", refs(mine("big")), "photos[0].publicId", "más de 5 MB"},
		{"tamaño cero", refs(mine("cero")), "photos[0].publicId", "vacía o dañada"},
		{"publicId larguísimo", refs(mine(strings.Repeat("a", 300))), "photos[0].publicId", "demasiado largo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Resolve(ctx, "u1", domain.PurposeItemPhoto, "photos", tt.refs)
			d := details(t, err)
			if !strings.Contains(d[tt.field], tt.issue) {
				t.Errorf("detalles = %v, want %s: %q", d, tt.field, tt.issue)
			}
		})
	}
	if _, err := s.Resolve(ctx, "u1", "avatar", "photos", refs(mine("ok"))); !errors.Is(err, domain.ErrInvalidPurpose) {
		t.Errorf("propósito inválido = %v", err)
	}
	// La carpeta ajena se rechaza sin consultar a Cloudinary.
	res.types = nil
	_, _ = s.Resolve(ctx, "u1", domain.PurposeItemPhoto, "photos", refs(pid(domain.PurposeItemPhoto, "u2", "ajena")))
	if len(res.types) != 0 {
		t.Error("no debe consultar Cloudinary por imágenes ajenas")
	}
}

func TestResolveUnavailable(t *testing.T) { // RF-08
	a := pid(domain.PurposeItemPhoto, "u1", "a")
	res := &fakeResources{err: fmt.Errorf("cloudinary: 503: %w", domain.ErrUnavailable)}
	s, _ := newSvc(t, res)
	imgs, err := s.Resolve(context.Background(), "u1", domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: a}})
	var de *shared.Error
	if imgs != nil || !errors.As(err, &de) || de.Code != shared.CodeUnavailable {
		t.Errorf("Cloudinary caído = %v, %v; want service_unavailable sin imágenes", imgs, err)
	}
	res.err = errors.New("otro fallo")
	if _, err := s.Resolve(context.Background(), "u1", domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: a}}); err == nil || errors.As(err, &de) && de.Code == shared.CodeValidation {
		t.Errorf("fallo interno = %v", err)
	}
}

func TestURLs(t *testing.T) {
	s, _ := newSvc(t, &fakeResources{})
	u, err := s.URLs(domain.Image{PublicID: "p"})
	if err != nil || u.URL != "u:p" || u.ThumbURL != "t:p" {
		t.Errorf("URLs = %+v, %v", u, err)
	}
	bad, _ := newSvc(t, &fakeResources{}, func(d *Deps) { d.URLs = fakeURLs{err: errors.New("x")} })
	if _, err := bad.URLs(domain.Image{}); err == nil {
		t.Error("el error del constructor debe propagarse")
	}
}
