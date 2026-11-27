package notices

import (
	"context"
	"errors"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
)

type recorder struct {
	user string
	ev   notify.Event
	err  error
}

func (r *recorder) Notify(_ context.Context, userID string, e notify.Event) error {
	r.user, r.ev = userID, e
	return r.err
}

func TestSenderForwardsTheNotice(t *testing.T) {
	rec := &recorder{}
	s, err := New(rec)
	if err != nil {
		t.Fatal(err)
	}
	n := usecase.Notice{UserID: "u1", Type: "booking.cancelled", Title: "T", Body: "B", Data: map[string]string{"bookingId": "b1"}}
	if err := s.Send(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if rec.user != "u1" || rec.ev.Type != "booking.cancelled" || rec.ev.Title != "T" || rec.ev.Body != "B" || rec.ev.Data["bookingId"] != "b1" {
		t.Errorf("evento = %+v para %q", rec.ev, rec.user)
	}
	boom := errors.New("falla")
	rec.err = boom
	if err := s.Send(context.Background(), n); !errors.Is(err, boom) {
		t.Errorf("el error se devuelve para reintentar: %v", err)
	}
}

func TestNewNeedsNotifier(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Error("sin notificador debe fallar")
	}
}
