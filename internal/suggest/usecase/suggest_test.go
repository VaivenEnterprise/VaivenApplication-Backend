package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
)

const (
	base  = "vaiven/staging"
	owner = "6730f0aa9b3e4d5f6a7b8c90"
	other = "6730f0aa9b3e4d5f6a7b8c91"
)

func ownPhoto() string { return base + "/item_photo/" + owner + "/abc123" }

type fakeAI struct {
	calls int
	last  aidomain.Request
	out   string
	err   error
}

func (f *fakeAI) Generate(_ context.Context, req aidomain.Request) ([]byte, error) {
	f.calls++
	f.last = req
	return []byte(f.out), f.err
}

type fakeImages struct {
	resolveErr error
	none       bool
	urlErr     error
}

func (f fakeImages) Resolve(_ context.Context, _ string, _ imagesdomain.Purpose, _ string, refs []imagesdomain.Ref) ([]imagesdomain.Image, error) {
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	if f.none {
		return nil, nil
	}
	return []imagesdomain.Image{{PublicID: refs[0].PublicID}}, nil
}

func (f fakeImages) URLs(img imagesdomain.Image) (imagesdomain.URLs, error) {
	return imagesdomain.URLs{URL: "https://res.example/" + img.PublicID + ".jpg", ThumbURL: "https://res.example/t/" + img.PublicID + ".jpg"}, f.urlErr
}

type fakeUsage struct {
	left  int
	err   error
	day   string
	taken int
}

func (f *fakeUsage) Take(_ context.Context, _, feature, day string, _ int, _ time.Time) (bool, error) {
	f.day = day
	if feature != Feature {
		return false, errors.New("función equivocada")
	}
	if f.err != nil {
		return false, f.err
	}
	if f.left <= 0 {
		return false, nil
	}
	f.left--
	f.taken++
	return true, nil
}

type blockWord string

func (b blockWord) Check(_ context.Context, text string) (moderation.Result, error) {
	if b != "" && strings.Contains(text, string(b)) {
		return moderation.Result{Allowed: false, Reason: "no permitido"}, nil
	}
	return moderation.Result{Allowed: true}, nil
}

type env struct {
	ai    *fakeAI
	usage *fakeUsage
	svc   *Service
}

func newEnv(t *testing.T, mod moderation.Checker, imgs fakeImages, enabled bool) env {
	t.Helper()
	ai := &fakeAI{out: `{"title":"Taladro inalámbrico","category":"herramientas_electricas","description":"Con dos baterías"}`}
	usage := &fakeUsage{left: 2}
	// 23:30 en Ciudad de México del 15 de nov = 05:30 UTC del 16: el día del
	// contador debe ser el 15 (hora de México), no el 16.
	now := time.Date(2026, 11, 16, 5, 30, 0, 0, time.UTC)
	svc := New(Config{Enabled: enabled, DailyLimit: 2, BaseFolder: base},
		Deps{AI: ai, Images: imgs, Usage: usage, Moderation: mod, Now: func() time.Time { return now }})
	return env{ai: ai, usage: usage, svc: svc}
}

func code(err error) shared.Code {
	var de *shared.Error
	if errors.As(err, &de) {
		return de.Code
	}
	return ""
}

func TestSuggestOwnPhoto(t *testing.T) { // CA-01
	e := newEnv(t, nil, fakeImages{}, true)
	s, err := e.svc.Suggest(context.Background(), owner, ownPhoto())
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Taladro inalámbrico" || s.Category != "herramientas_electricas" || s.Description == "" {
		t.Errorf("sugerencia = %+v", s)
	}
	if e.ai.last.ImageURL != "https://res.example/t/"+ownPhoto()+".jpg" || e.ai.last.System == "" || e.ai.last.Schema == nil {
		t.Errorf("petición al proveedor = %+v", e.ai.last)
	}
	if e.usage.day != "2026-11-15" {
		t.Errorf("día del contador = %s, want 2026-11-15 (hora de México)", e.usage.day)
	}
}

func TestSuggestForeignOrEvidencePhotoNeverCallsProvider(t *testing.T) { // CA-02
	e := newEnv(t, nil, fakeImages{}, true)
	for _, id := range []string{
		base + "/item_photo/" + other + "/abc",           // de otra persona
		base + "/evidence/" + owner + "/abc",             // evidencia propia
		"vaiven/production/item_photo/" + owner + "/abc", // otra carpeta de entorno
		"https://evil.example/foto.jpg",                  // URL, no publicId
	} {
		if _, err := e.svc.Suggest(context.Background(), owner, id); code(err) != shared.CodeNotFound {
			t.Errorf("%s: err = %v", id, err)
		}
	}
	if e.ai.calls != 0 || e.usage.taken != 0 {
		t.Errorf("no debe llamar al proveedor ni gastar cupo: llamadas=%d cupo=%d", e.ai.calls, e.usage.taken)
	}
}

func TestSuggestPhotoThatDoesNotExist(t *testing.T) {
	e := newEnv(t, nil, fakeImages{resolveErr: shared.NewValidation("x")}, true)
	if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); code(err) != shared.CodeNotFound || e.ai.calls != 0 {
		t.Errorf("err=%v llamadas=%d", err, e.ai.calls)
	}
	e = newEnv(t, nil, fakeImages{none: true}, true)
	if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); code(err) != shared.CodeNotFound {
		t.Errorf("sin imágenes: err=%v", err)
	}
}

func TestSuggestCloudinaryDownIsUnavailable(t *testing.T) {
	e := newEnv(t, nil, fakeImages{resolveErr: shared.NewUnavailable("Cloudinary no responde")}, true)
	if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); code(err) != shared.CodeUnavailable {
		t.Errorf("err = %v", err)
	}
	e = newEnv(t, nil, fakeImages{resolveErr: errors.New("algo raro")}, true)
	if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); err == nil || code(err) != "" {
		t.Errorf("error inesperado debe ser interno: %v", err)
	}
}

func TestSuggestFilteredTitleIsNotReturned(t *testing.T) { // CA-03
	e := newEnv(t, blockWord("Taladro"), fakeImages{}, true)
	if s, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); code(err) != shared.CodeUnavailable || s.Title != "" {
		t.Errorf("s=%+v err=%v", s, err)
	}
	e = newEnv(t, blockWord("baterías"), fakeImages{}, true)
	s, err := e.svc.Suggest(context.Background(), owner, ownPhoto())
	if err != nil || s.Title == "" || s.Description != "" {
		t.Errorf("una descripción no permitida solo se quita: s=%+v err=%v", s, err)
	}
}

func TestSuggestDisabledOrProviderDown(t *testing.T) { // CA-04
	e := newEnv(t, nil, fakeImages{}, false)
	if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); !errors.Is(err, ErrDisabled) || e.ai.calls != 0 || e.svc.Enabled() {
		t.Errorf("apagada: err=%v llamadas=%d", err, e.ai.calls)
	}
	for _, provErr := range []error{aidomain.ErrUnavailable, aidomain.ErrBlocked, context.DeadlineExceeded} {
		e := newEnv(t, nil, fakeImages{}, true)
		e.ai.err = provErr
		if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); code(err) != shared.CodeUnavailable || !errors.Is(err, provErr) {
			t.Errorf("%v: err = %v", provErr, err)
		}
	}
	e = newEnv(t, nil, fakeImages{}, true)
	e.ai.out = `{"title":"","category":"armas"}`
	if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); code(err) != shared.CodeUnavailable {
		t.Errorf("salida inválida: err = %v", err)
	}
}

func TestSuggestDailyLimit(t *testing.T) { // CA-05
	e := newEnv(t, nil, fakeImages{}, true)
	for i := 0; i < 2; i++ {
		if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); !errors.Is(err, ErrDailyLimit) || e.ai.calls != 2 {
		t.Errorf("tercera: err=%v llamadas=%d", err, e.ai.calls)
	}
	e = newEnv(t, nil, fakeImages{}, true)
	e.usage.err = errors.New("mongo caído")
	if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); err == nil || e.ai.calls != 0 {
		t.Errorf("contador caído: err=%v llamadas=%d", err, e.ai.calls)
	}
}

func TestSuggestValidationAndThumbErrors(t *testing.T) {
	e := newEnv(t, nil, fakeImages{}, true)
	for _, id := range []string{"", "   ", strings.Repeat("a", 256)} {
		if _, err := e.svc.Suggest(context.Background(), owner, id); code(err) != shared.CodeValidation {
			t.Errorf("%q: err = %v", id, err)
		}
	}
	e = newEnv(t, nil, fakeImages{urlErr: errors.New("sin url")}, true)
	if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); err == nil || e.ai.calls != 0 {
		t.Errorf("miniatura fallida: err=%v", err)
	}
}

type failingChecker struct{ onCall int }

func (f *failingChecker) Check(context.Context, string) (moderation.Result, error) {
	f.onCall--
	if f.onCall <= 0 {
		return moderation.Result{}, errors.New("filtro caído")
	}
	return moderation.Result{Allowed: true}, nil
}

func TestSuggestModerationErrors(t *testing.T) {
	for _, n := range []int{1, 2} { // falla al revisar el título y luego la descripción
		e := newEnv(t, &failingChecker{onCall: n}, fakeImages{}, true)
		if _, err := e.svc.Suggest(context.Background(), owner, ownPhoto()); err == nil {
			t.Errorf("filtro caído en la revisión %d: debe fallar", n)
		}
	}
}

func TestNewDefaults(t *testing.T) {
	s := New(Config{Enabled: true}, Deps{})
	if s.cfg.Timeout != defaultTimeout || s.cfg.Location == nil || s.d.Now == nil || s.d.Moderation == nil {
		t.Errorf("valores por defecto = %+v", s.cfg)
	}
}
