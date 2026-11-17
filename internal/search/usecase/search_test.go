package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
)

type fakeRepo struct {
	page domain.Page
	err  error
	got  domain.Query
}

func (f *fakeRepo) Search(_ context.Context, q domain.Query) (domain.Page, error) {
	f.got = q
	return f.page, f.err
}

type fakeImages struct{ err error }

func (fakeImages) Signature(context.Context, string, images.Purpose) (images.UploadSignature, error) {
	return images.UploadSignature{}, nil
}

func (fakeImages) Resolve(context.Context, string, images.Purpose, string, []images.Ref) ([]images.Image, error) {
	return nil, nil
}

func (f fakeImages) URLs(img images.Image) (images.URLs, error) {
	return images.URLs{URL: "u/" + img.PublicID, ThumbURL: "t/" + img.PublicID}, f.err
}

// 2026-11-09 03:00 UTC = 2026-11-08 21:00 en Ciudad de México.
var lateNight = time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)

func newService(t *testing.T, repo *fakeRepo, imgs fakeImages) *Service {
	t.Helper()
	s, err := NewService(Deps{Repo: repo, Images: imgs, Now: func() time.Time { return lateNight }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewService(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	s, err := NewService(Deps{Repo: &fakeRepo{}, Images: fakeImages{}})
	if err != nil || s.d.Location.String() != TimeZone || s.d.Now == nil {
		t.Errorf("valores por defecto: %v", err)
	}
}

func TestSearch(t *testing.T) {
	repo := &fakeRepo{page: domain.Page{
		Cards: []domain.Card{{ID: "a", FirstPhoto: &images.Image{PublicID: "p1"}}, {ID: "b"}},
		Next:  &domain.Cursor{ID: "b"},
	}}
	s := newService(t, repo, fakeImages{})
	res, err := s.Search(context.Background(), "u1", domain.RawQuery{Q: "taladro", From: "2026-11-08", To: "2026-11-09"})
	if err != nil {
		t.Fatal(err)
	}
	// Hoy se calcula en Ciudad de México: 2026-11-08 sigue siendo válido aunque en UTC ya sea 9.
	if repo.got.ViewerID != "u1" || repo.got.Text != "taladro" || repo.got.Period.From != "2026-11-08" {
		t.Errorf("consulta = %+v", repo.got)
	}
	if len(res.Cards) != 2 || *res.Cards[0].ThumbURL != "t/p1" || res.Cards[1].ThumbURL != nil || res.Next.ID != "b" {
		t.Errorf("resultado = %+v", res)
	}
}

func TestSearchErrors(t *testing.T) {
	ctx := context.Background()
	s := newService(t, &fakeRepo{}, fakeImages{})
	_, err := s.Search(ctx, "u", domain.RawQuery{Lat: "19"}) // CA-06
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeValidation || de.Details[0].Field != "lng" {
		t.Errorf("CA-06 = %v", err)
	}
	if _, err := s.Search(ctx, "u", domain.RawQuery{From: "2026-11-07", To: "2026-11-08"}); err == nil {
		t.Error("from en el pasado (México) debe fallar")
	}
	s = newService(t, &fakeRepo{err: errors.New("mongo")}, fakeImages{})
	if _, err := s.Search(ctx, "u", domain.RawQuery{}); err == nil {
		t.Error("error del repositorio")
	}
	s = newService(t, &fakeRepo{page: domain.Page{Cards: []domain.Card{{FirstPhoto: &images.Image{}}}}}, fakeImages{err: errors.New("url")})
	if _, err := s.Search(ctx, "u", domain.RawQuery{}); err == nil {
		t.Error("error de miniatura")
	}
}
