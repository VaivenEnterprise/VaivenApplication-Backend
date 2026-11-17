package search

import (
	"context"
	"errors"
	"reflect"
	"testing"

	searchdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/domain"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

type fakeSearch struct {
	cards  []searchusecase.CardView
	err    error
	viewer string
	ids    []string
}

func (f *fakeSearch) CardsByIDs(_ context.Context, viewerID string, ids []string) ([]searchusecase.CardView, error) {
	f.viewer, f.ids = viewerID, ids
	return f.cards, f.err
}

func TestCards(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Error("sin búsqueda debe fallar")
	}
	want := []searchusecase.CardView{{Card: searchdomain.Card{ID: "b"}}, {Card: searchdomain.Card{ID: "a"}}}
	f := &fakeSearch{cards: want}
	c, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Cards(context.Background(), "u1", []string{"b", "a"})
	if err != nil || !reflect.DeepEqual(got, want) || f.viewer != "u1" || !reflect.DeepEqual(f.ids, []string{"b", "a"}) {
		t.Errorf("Cards = %v %v (viewer %s, ids %v)", got, err, f.viewer, f.ids)
	}
	f.cards = nil
	if got, err := c.Cards(context.Background(), "u1", nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("sin resultados debe ser [] y no nil: %v %v", got, err)
	}
	f.err = errors.New("caída")
	if _, err := c.Cards(context.Background(), "u1", []string{"a"}); err == nil {
		t.Error("debe propagar el error")
	}
}
