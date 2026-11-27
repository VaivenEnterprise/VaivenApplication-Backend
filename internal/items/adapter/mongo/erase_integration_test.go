//go:build integration

package mongo

import (
	"context"
	"errors"
	"slices"
	"sort"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/erasure"
)

func photo(id string, d images.Delivery) images.Image {
	return images.Image{PublicID: id, Width: 10, Height: 10, Format: "jpg", Bytes: 100, Delivery: d}
}

func publicIDs(refs []erasure.ImageRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Delivery+":"+r.PublicID)
	}
	sort.Strings(out)
	return out
}

// Spec 021, T025 (D-2, CA-09): se borran los objetos de la persona en cualquier
// estado y se devuelven sus fotos (con el tipo de entrega de Cloudinary); los de
// otras personas no se tocan; repetir no hace nada.
func TestIntegrationEraserDeletesItemsAndReturnsPhotos(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	repo := NewItemRepo(db)
	er := NewEraser(db)
	now := time.Now().UTC().Truncate(time.Millisecond)

	mine := func(status domain.Status, photos ...images.Image) domain.Item {
		it := newItem("u1", now)
		it.Status = status
		it.Photos = photos
		return it
	}
	for _, it := range []domain.Item{
		mine(domain.StatusActive, photo("vaiven/t/a1", images.DeliveryPublic), photo("vaiven/t/a2", images.DeliveryAuthenticated)),
		mine(domain.StatusPaused, photo("vaiven/t/p1", images.DeliveryPublic)),
		mine(domain.StatusRemoved, photo("vaiven/t/r1", images.DeliveryPublic)),
	} {
		if _, err := repo.Create(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	other, err := repo.Create(ctx, func() domain.Item {
		it := newItem("u2", now)
		it.Photos = []images.Image{photo("vaiven/t/o1", images.DeliveryPublic)}
		return it
	}())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := db.Collection(ItemsCollection).FindOne(ctx, bson.D{{Key: "ownerId", Value: "u2"}}).Raw()

	res, err := er.Erase(ctx, "u1", now)
	if err != nil {
		t.Fatalf("Erase: %v", err)
	}
	want := []string{"authenticated:vaiven/t/a2", "upload:vaiven/t/a1", "upload:vaiven/t/p1", "upload:vaiven/t/r1"}
	if got := publicIDs(res.Images); !slices.Equal(got, want) {
		t.Errorf("fotos = %v; quiero %v", got, want)
	}
	if res.Counts["items.deleted"] != 3 || len(res.Notices) != 0 {
		t.Errorf("resultado = %+v", res)
	}
	for _, r := range res.Images {
		if !r.NotBefore.IsZero() {
			t.Errorf("las fotos de objetos se borran ya, no a futuro: %+v", r)
		}
	}

	// Ya no existen para nadie: ni por id ni en el listado del dueño.
	list, err := repo.ListByOwner(ctx, "u1", nil, nil, 50)
	if err != nil || len(list) != 0 {
		t.Errorf("ListByOwner = %v, %v; quiero vacío", len(list), err)
	}
	n, _ := db.Collection(ItemsCollection).CountDocuments(ctx, bson.D{{Key: "ownerId", Value: "u1"}})
	if n != 0 {
		t.Errorf("quedan %d objetos de la persona", n)
	}
	// El objeto ajeno queda intacto (byte a byte).
	after, _ := db.Collection(ItemsCollection).FindOne(ctx, bson.D{{Key: "ownerId", Value: "u2"}}).Raw()
	if string(before) != string(after) {
		t.Error("el objeto de otra persona cambió")
	}
	if _, err := repo.ByID(ctx, other.ID); err != nil {
		t.Errorf("ByID ajeno: %v", err)
	}

	// Idempotente: la segunda pasada no encuentra nada ni devuelve fotos.
	res2, err := er.Erase(ctx, "u1", now)
	if err != nil || len(res2.Images) != 0 || res2.Counts["items.deleted"] != 0 {
		t.Errorf("segunda pasada = %+v, %v", res2, err)
	}
}

// Un error posterior en la transacción deshace el borrado (CA-05).
func TestIntegrationEraserRollsBackWithTransaction(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	repo := NewItemRepo(db)
	now := time.Now().UTC().Truncate(time.Millisecond)
	created, err := repo.Create(ctx, newItem("u1", now))
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("falla a media transacción")
	err = adaptermongo.NewTxRunner(db).Do(ctx, func(tx context.Context) error {
		if _, err := NewEraser(db).Erase(tx, "u1", now); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, err := repo.ByID(ctx, created.ID); err != nil {
		t.Errorf("el objeto debía seguir existiendo: %v", err)
	}
}
