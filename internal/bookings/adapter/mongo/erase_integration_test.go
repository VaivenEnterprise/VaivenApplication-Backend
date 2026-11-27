//go:build integration

package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
)

func rawBooking(t *testing.T, e env, id string) bson.Raw {
	t.Helper()
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		t.Fatal(err)
	}
	var raw bson.Raw
	if err := e.db.Collection(BookingsCollection).FindOne(context.Background(), bson.D{{Key: "_id", Value: oid}}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func hasField(raw bson.Raw, path ...string) bool {
	_, err := raw.LookupErr(path...)
	return err == nil
}

// Spec 021, T015 (CA-09, CA-10): las pendientes de la persona se cierran con el
// motivo «cuenta eliminada», cada contraparte recibe un aviso, y los ids y el
// itemSnapshot ajenos no cambian.
func TestIntegrationEraserClosesPendingAndDetachesPhotos(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	er := NewEraser(e.db)

	myItem := e.item("ana", "active")
	otherItem := e.item("beto", "active")
	// Ana es dueña: dos pendientes de otros sobre su objeto.
	asOwner1 := e.request(myItem, "carla", "2026-11-10", "2026-11-12")
	asOwner2 := e.request(myItem, "dario", "2026-11-20", "2026-11-21")
	// Ana es solicitante de un objeto de Beto.
	asRequester := e.request(otherItem, "ana", "2026-12-01", "2026-12-02")
	// Ajenas: Beto-Carla, no tocan a Ana.
	unrelated := e.request(otherItem, "carla", "2026-12-10", "2026-12-11")
	// Una reserva pasada de Ana como dueña (completed) conserva todo menos la foto.
	past := e.request(myItem, "carla", "2026-11-25", "2026-11-26")
	if _, err := e.db.Collection(BookingsCollection).UpdateOne(ctx, bson.D{{Key: "_id", Value: mustOID(t, past.ID)}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "completed"}}}, {Key: "$unset", Value: bson.D{{Key: "expiresAt", Value: ""}}}}); err != nil {
		t.Fatal(err)
	}
	// Otra pendiente de Ema, con vencimiento más lejano.
	late := e.request(myItem, "ema", "2026-11-28", "2026-11-29")
	unrelatedBefore := rawBooking(t, e, unrelated.ID)

	// Con el reloj muy adelantado todas las pendientes ya vencieron (aunque la
	// tarea periódica aún no las marque): no se cierran, las cierra el
	// vencimiento. Las fotos de los objetos de Ana sí se desprenden.
	res, err := er.Erase(ctx, "ana", now0.Add(72*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notices) != 0 || res.Counts["bookings.closed"] != 0 || res.Counts["bookings.photosUnset"] != 4 {
		t.Errorf("con todas vencidas = %+v", res)
	}
	if got := rawBooking(t, e, late.ID); statusOf(got) != "pending" {
		t.Errorf("la vencida no se toca, estado %s", statusOf(got))
	}

	// Ahora sí, con el reloj de cuando eran vigentes.
	at := now0.Add(time.Hour)
	res, err = er.Erase(ctx, "ana", at)
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts["bookings.closed"] != 4 || len(res.Notices) != 4 || res.Counts["bookings.photosUnset"] != 0 {
		t.Fatalf("resultado = %+v (4 pendientes cerradas; las fotos ya se quitaron)", res)
	}

	for _, id := range []string{asOwner1.ID, asOwner2.ID, late.ID} {
		raw := rawBooking(t, e, id)
		if statusOf(raw) != "rejected" || raw.Lookup("rejectReason").StringValue() != "cuenta eliminada" || hasField(raw, "cancelReason") {
			t.Errorf("%s (dueña) = %v", id, raw)
		}
		if hasField(raw, "itemSnapshot", "firstPhoto") || raw.Lookup("itemSnapshot", "title").StringValue() != "Taladro" {
			t.Errorf("%s: firstPhoto debe quitarse y el título conservarse", id)
		}
		if raw.Lookup("version").Int64() != 3 { // alta + foto + rechazo
			t.Errorf("%s: version = %d", id, raw.Lookup("version").Int64())
		}
		if hasField(raw, "expiresAt") {
			t.Errorf("%s: una solicitud cerrada no conserva expiresAt", id)
		}
	}
	raw := rawBooking(t, e, asRequester.ID)
	if statusOf(raw) != "cancelled" || raw.Lookup("cancelReason").StringValue() != "cuenta eliminada" ||
		raw.Lookup("cancelledBy").StringValue() != "requester" || hasField(raw, "rejectReason") {
		t.Errorf("solicitante = %v", raw)
	}
	if !hasField(raw, "itemSnapshot", "firstPhoto") {
		t.Error("la reserva donde Ana es solicitante conserva la foto del objeto de Beto")
	}
	// La pasada: sin foto del objeto borrado, estado y snapshot intactos.
	rp := rawBooking(t, e, past.ID)
	if statusOf(rp) != "completed" || hasField(rp, "itemSnapshot", "firstPhoto") || rp.Lookup("itemSnapshot", "title").StringValue() != "Taladro" ||
		rp.Lookup("requesterId").StringValue() != "carla" || rp.Lookup("ownerId").StringValue() != "ana" {
		t.Errorf("reserva pasada = %v", rp)
	}
	// Ajenas intactas, byte a byte.
	if got := rawBooking(t, e, unrelated.ID); string(got) != string(unrelatedBefore) {
		t.Error("una reserva ajena cambió")
	}
	if n := nullPathsRaw(t, rawBooking(t, e, asOwner1.ID)); len(n) != 0 {
		t.Errorf("null en %v", n)
	}

	// Un aviso por contraparte, con los tipos existentes y solo ids en Data.
	to := map[string]string{}
	for _, n := range res.Notices {
		to[n.UserID] = n.Type
		if n.Data["bookingId"] == "" || n.Data["itemId"] == "" || len(n.Data) != 2 {
			t.Errorf("Data = %v", n.Data)
		}
	}
	if to["carla"] != "booking.rejected" || to["dario"] != "booking.rejected" || to["ema"] != "booking.rejected" || to["beto"] != "booking.cancelled" {
		t.Errorf("destinatarios = %v", to)
	}

	// Idempotente.
	again, err := er.Erase(ctx, "ana", at.Add(time.Minute))
	if err != nil || len(again.Notices) != 0 || again.Counts["bookings.closed"] != 0 || again.Counts["bookings.photosUnset"] != 0 {
		t.Errorf("segunda ejecución = %+v, %v", again, err)
	}
	if er.Name() != "bookings" {
		t.Errorf("Name = %q", er.Name())
	}
}

func statusOf(raw bson.Raw) string { return raw.Lookup("status").StringValue() }

func mustOID(t *testing.T, id string) bson.ObjectID {
	t.Helper()
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		t.Fatal(err)
	}
	return oid
}

func nullPathsRaw(t *testing.T, raw bson.Raw) []string {
	t.Helper()
	var out []string
	var walk func(v bson.RawValue, path string)
	walk = func(v bson.RawValue, path string) {
		switch v.Type {
		case bson.TypeNull:
			out = append(out, path)
		case bson.TypeEmbeddedDocument:
			els, _ := v.Document().Elements()
			for _, el := range els {
				walk(el.Value(), path+"."+el.Key())
			}
		case bson.TypeArray:
			vals, _ := v.Array().Values()
			for _, x := range vals {
				walk(x, path+"[]")
			}
		}
	}
	els, _ := raw.Elements()
	for _, el := range els {
		walk(el.Value(), el.Key())
	}
	return out
}

// CA-04: una reserva que se acepta entre la primera comprobación y la
// transacción hace que la baja se rechace sin cambios. La aceptación escribe el
// mismo documento que cierra la baja, así que la transacción choca
// (WriteConflict), el driver repite la función y la comprobación ya ve la
// reserva aceptada.
func TestIntegrationEraserInterleavedWithAcceptRollsBackEverything(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	er := NewEraser(e.db)
	activity := NewUserActivity(e.db)
	tx := adaptermongo.NewTxRunner(e.db)

	itemID := e.item("ana", "active")
	pending := e.request(itemID, "carla", "2026-11-10", "2026-11-12")
	other := e.request(itemID, "dario", "2026-11-20", "2026-11-21")

	errActive := errors.New("reservas activas") // el orquestador devuelve ErrActiveBookings
	attempts := 0
	err := tx.Do(ctx, func(txCtx context.Context) error {
		attempts++
		busy, err := activity.HasActiveBookings(txCtx, "ana")
		if err != nil {
			return err
		}
		if busy {
			return errActive
		}
		if attempts == 1 {
			// Ana acepta la solicitud de Carla justo ahora (fuera de la transacción).
			if _, _, err := e.repo.Accept(ctx, accepted(t, pending), now0); err != nil {
				t.Errorf("Accept concurrente: %v", err)
			}
		}
		_, err = er.Erase(txCtx, "ana", now0.Add(time.Hour))
		return err
	})
	if !errors.Is(err, errActive) {
		t.Fatalf("err = %v, want la comprobación de reservas activas", err)
	}
	if attempts != 2 {
		t.Errorf("intentos = %d, want 2 (el choque repite la función)", attempts)
	}
	got := rawBooking(t, e, pending.ID)
	if statusOf(got) != "accepted" || !hasField(got, "itemSnapshot", "firstPhoto") {
		t.Errorf("la aceptada debe quedar aceptada y con su foto: %v", got)
	}
	// La otra pendiente (rechazada por Accept por cruce o intacta) no la tocó la baja.
	if o := rawBooking(t, e, other.ID); statusOf(o) != "pending" || !hasField(o, "itemSnapshot", "firstPhoto") || o.Lookup("version").Int64() != 1 {
		t.Errorf("la baja revertida no debe dejar rastro: %v", o)
	}
}

// Un fallo del eraser a media transacción (CA-05) no deja nada.
func TestIntegrationEraserRollsBackWithTheTransaction(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	er := NewEraser(e.db)
	tx := adaptermongo.NewTxRunner(e.db)
	itemID := e.item("ana", "active")
	b := e.request(itemID, "carla", "2026-11-10", "2026-11-12")
	before := string(rawBooking(t, e, b.ID))
	boom := errors.New("falla simulada")
	err := tx.Do(ctx, func(txCtx context.Context) error {
		res, err := er.Erase(txCtx, "ana", now0.Add(time.Hour))
		if err != nil || res.Counts["bookings.closed"] != 1 {
			t.Errorf("Erase = %+v, %v", res, err)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if string(rawBooking(t, e, b.ID)) != before {
		t.Error("el rollback debe dejar la reserva idéntica")
	}
}
