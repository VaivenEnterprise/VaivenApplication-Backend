//go:build integration

package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	bookingsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	chatmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/mongo"
	chatdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/domain"
	chatusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/usecase"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
	itemsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	ratingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/adapter/mongo"
	ratingsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ratings/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// stubImages cumple images/port.Service sin red: solo las URL de lectura.
type stubImages struct{}

func (stubImages) Signature(context.Context, string, imagesdomain.Purpose) (imagesdomain.UploadSignature, error) {
	return imagesdomain.UploadSignature{}, errors.New("no se usa")
}

func (stubImages) Resolve(context.Context, string, imagesdomain.Purpose, string, []imagesdomain.Ref) ([]imagesdomain.Image, error) {
	return nil, errors.New("no se usa")
}

func (stubImages) URLs(imagesdomain.Image) (imagesdomain.URLs, error) {
	return imagesdomain.URLs{URL: "https://img.example/x.jpg", ThumbURL: "https://img.example/x-t.jpg"}, nil
}

// Spec 021, T027 (RF-07, RF-08, CA-07, CA-09, CA-13, R-03): tras la baja de Ana,
// ninguna lectura de sus contrapartes da 404 ni 500; el chat queda de solo
// lectura; la calificación sin comentario; el objeto ya no existe y el perfil
// responde «Cuenta eliminada».
func TestIntegrationReadsWithDeletedCounterpart(t *testing.T) {
	rc := newRedis(t)
	ctx := context.Background()
	e := newDelEnv(t, rc, nil, nil)
	s := e.seed()
	if st, code := e.deleteMe(s.ana, delPass); st != http.StatusNoContent {
		t.Fatalf("baja = %d %s", st, code)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	notifier, err := notify.NewService(notify.ServiceDeps{Store: e.notifs, Users: e.users, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	var dir users.Directory = e.users

	// Chat (RF-08, CA-07): el historial completo, solo lectura; enviar da 409.
	chatSvc, err := chatusecase.NewService(chatusecase.Deps{
		Conversations: chatmongo.NewConversationRepo(e.db), Messages: chatmongo.NewMessageRepo(e.db), Sender: chatmongo.NewSender(e.db),
		Bookings: chatmongo.NewBookingReader(e.db), Accounts: chatmongo.NewAccountReader(e.db),
		Users: dir, Images: stubImages{}, Moderation: moderation.NewFilter(), Notifier: notifier, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("chat en solo lectura", func(t *testing.T) {
		page, err := chatSvc.List(ctx, s.beto.id, nil, 20)
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("List = %d, %v", len(page.Items), err)
		}
		if v := page.Items[0]; v.Counterpart.DisplayName != "Cuenta eliminada" || v.CanWrite {
			t.Errorf("conversación = %+v", v)
		}
		msgs, err := chatSvc.Messages(ctx, s.beto.id, s.conv, chatusecase.MessageQuery{Limit: 50})
		if err != nil {
			t.Fatalf("Messages: %v", err)
		}
		var bodies []string
		for _, m := range msgs.Items {
			bodies = append(bodies, m.Body)
		}
		slices.Sort(bodies)
		if !slices.Equal(bodies, []string{"Mensaje eliminado", "Mensaje eliminado", "hola Ana"}) {
			t.Errorf("mensajes = %v", bodies)
		}
		if err := chatSvc.Read(ctx, s.beto.id, s.conv); err != nil {
			t.Errorf("marcar como leída: %v", err)
		}
		if _, _, err := chatSvc.Send(ctx, s.beto.id, s.conv, "¿sigues ahí?", nil); !errors.Is(err, chatdomain.ErrClosed) {
			t.Errorf("enviar a una cuenta eliminada = %v, want 409 «La conversación está cerrada.»", err)
		}
	})

	// Reservas (RF-07, CA-09): el historial ajeno se lee con «Cuenta eliminada» y
	// sin error, aunque el objeto ya no exista.
	bookingsSvc, err := bookingsusecase.NewService(bookingsusecase.Deps{
		Repo: e.bookings, Images: stubImages{}, Users: dir, Chat: chatSvc, Notifier: notifier, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("reservas con la otra cuenta y el objeto eliminados", func(t *testing.T) {
		v, err := bookingsSvc.Get(ctx, bookingsusecase.Viewer{ID: s.beto.id}, s.pendingOwned.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if v.Booking.Status != bookings.StatusRejected || v.Owner.DisplayName != "Cuenta eliminada" || v.Owner.RatingCount != 0 || v.Booking.Item.Title == "" {
			t.Errorf("reserva = %+v owner=%+v", v.Booking.Status, v.Owner)
		}
		page, err := bookingsSvc.List(ctx, bookingsusecase.Viewer{ID: s.beto.id}, bookingsusecase.ListQuery{Role: bookings.RoleRequester, Limit: 20})
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("List como solicitante = %d, %v", len(page.Items), err)
		}
		// Carla ve la solicitud que Ana le hizo, cancelada y con Ana eliminada.
		page, err = bookingsSvc.List(ctx, bookingsusecase.Viewer{ID: s.carla.id}, bookingsusecase.ListQuery{Role: bookings.RoleOwner, Limit: 20})
		if err != nil || len(page.Items) != 1 || page.Items[0].Requester.DisplayName != "Cuenta eliminada" || page.Items[0].Booking.Status != bookings.StatusCancelled {
			t.Fatalf("List como dueña = %+v, %v", page.Items, err)
		}
	})

	// Calificaciones (RF-07, CA-08): el comentario de Ana ya no existe; la
	// persona eliminada se lista sin 404.
	ratingsSvc, err := ratingsusecase.NewService(ratingsusecase.Deps{
		Repo: ratingsmongo.NewRepo(e.db, e.users), Bookings: e.bookings, Users: dir, Notifier: notifier, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("calificaciones", func(t *testing.T) {
		page, err := ratingsSvc.List(ctx, s.beto.id, "", 20)
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("List(beto) = %d, %v", len(page.Items), err)
		}
		if v := page.Items[0]; v.RaterName != "Cuenta eliminada" || v.Rating.Comment != nil || v.Rating.Score != 5 {
			t.Errorf("calificación de Ana = %+v", v)
		}
		// GET /users/{ana}/ratings: la persona eliminada sigue «existiendo».
		if page, err = ratingsSvc.List(ctx, s.ana.id, "", 20); err != nil || len(page.Items) != 1 {
			t.Errorf("List(ana eliminada) = %d, %v; quiero 200 con lo que recibió", len(page.Items), err)
		}
	})

	// Objetos (CA-09, R-03): ya no existen; el Snapshot de moderación responde
	// not_found, que la moderación tolera (ver reports/usecase).
	t.Run("objetos de la cuenta eliminada", func(t *testing.T) {
		n, err := e.db.Collection(itemsmongo.ItemsCollection).CountDocuments(ctx, bson.D{{Key: "ownerId", Value: s.ana.id}})
		if err != nil || n != 0 {
			t.Errorf("objetos de Ana = %d, %v", n, err)
		}
		if _, err := itemsmongo.NewItemRepo(e.db).ByID(ctx, "6731a0000000000000000abc"); !errors.Is(err, itemsdomain.ErrNotFound) {
			t.Errorf("ByID inexistente = %v, want ErrNotFound (que la moderación tolera)", err)
		}
	})

	// Perfil (CA-13, N-2): 200 «Cuenta eliminada» con el contrato intacto.
	t.Run("perfil", func(t *testing.T) {
		var pub map[string]any
		if st, code := e.api.call(http.MethodGet, "/api/v1/users/"+s.ana.id, "", s.beto.access, &pub); st != http.StatusOK ||
			pub["displayName"] != "Cuenta eliminada" || pub["ratingAverage"] != nil || pub["ratingCount"] != float64(0) {
			t.Errorf("GET /users/{ana} = %d %s %v", st, code, pub)
		}
	})
}
