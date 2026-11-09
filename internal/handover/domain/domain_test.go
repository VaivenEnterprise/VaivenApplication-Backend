package domain

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

var (
	t0  = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC) // 12:00 en México
	loc = mustLoc()
)

func mustLoc() *time.Location {
	l, err := bookings.Location()
	if err != nil {
		panic(err)
	}
	return l
}

func photo(id string) images.Image {
	return images.Image{PublicID: id, Delivery: images.DeliveryAuthenticated}
}

func booking(status bookings.Status) bookings.Booking {
	return bookings.Booking{
		ID: "b1", OwnerID: "owner", RequesterID: "req", Status: status,
		Start: bookings.MustDate("2026-11-20"), End: bookings.MustDate("2026-11-22"),
	}
}

// withQR devuelve una etapa con evidencia versión 1 y un QR vigente para token.
func withQR(token string, at time.Time) *bookings.Handover {
	h := Record(nil, "owner", "Sin daños", []images.Image{photo("p1")}, at)
	qr := NewQR(&h, token, at)
	h.QR = &qr
	return &h
}

type fakeLate struct{ adj *bookings.Adjustment }

func (fakeLate) LateAt(b bookings.Booking) time.Time {
	return b.End.AddDays(1).StartOf(loc).Add(2 * time.Hour)
}
func (f fakeLate) Adjustment(bookings.Booking, time.Time) *bookings.Adjustment { return f.adj }

func TestKindRoles(t *testing.T) {
	cases := []struct {
		k          Kind
		rec, conf  bookings.Role
		req, after bookings.Status
	}{
		{KindDelivery, bookings.RoleOwner, bookings.RoleRequester, bookings.StatusAccepted, bookings.StatusInProgress},
		{KindReturn, bookings.RoleRequester, bookings.RoleOwner, bookings.StatusInProgress, bookings.StatusCompleted},
	}
	for _, c := range cases {
		if !c.k.Valid() || c.k.RecorderRole() != c.rec || c.k.ConfirmerRole() != c.conf ||
			c.k.RequiredStatus() != c.req || c.k.ResultStatus() != c.after {
			t.Errorf("%s: tabla de roles incorrecta", c.k)
		}
	}
	if Kind("otra").Valid() {
		t.Error("una etapa desconocida no debe ser válida")
	}
}

func TestOf(t *testing.T) {
	d, r := &bookings.Handover{EvidenceVersion: 1}, &bookings.Handover{EvidenceVersion: 2}
	b := bookings.Booking{Delivery: d, Return: r}
	if Of(b, KindDelivery) != d || Of(b, KindReturn) != r {
		t.Error("Of devuelve la etapa equivocada")
	}
}

func TestValidateNotes(t *testing.T) {
	for _, s := range []string{"", "  ab ", strings.Repeat("x", 501)} {
		if _, err := ValidateNotes(s); !errors.Is(err, ErrConditionNotes) {
			t.Errorf("ValidateNotes(%.10q) = %v, quiero ErrConditionNotes", s, err)
		}
	}
	got, err := ValidateNotes("  Rayón leve  ")
	if err != nil || got != "Rayón leve" {
		t.Errorf("ValidateNotes recortado = %q, %v", got, err)
	}
	if _, err := ValidateNotes(strings.Repeat("é", 500)); err != nil {
		t.Errorf("500 caracteres (no bytes) deben valer: %v", err)
	}
}

func TestValidatePhotoCount(t *testing.T) {
	for n, ok := range map[int]bool{0: false, 1: true, 5: true, 6: false} {
		if err := ValidatePhotoCount(n); (err == nil) != ok {
			t.Errorf("ValidatePhotoCount(%d) = %v", n, err)
		}
	}
}

func TestCanRecordAndIssueQR(t *testing.T) {
	acc := booking(bookings.StatusAccepted)
	inProg := booking(bookings.StatusInProgress)
	confirmed := booking(bookings.StatusAccepted)
	done := t0
	confirmed.Delivery = &bookings.Handover{ConfirmedAt: &done}
	withEv := booking(bookings.StatusAccepted)
	h := Record(nil, "owner", "ok", []images.Image{photo("a")}, t0)
	withEv.Delivery = &h

	cases := []struct {
		name string
		b    bookings.Booking
		k    Kind
		user string
		want error
	}{
		{"entrega: propietario", acc, KindDelivery, "owner", nil},
		{"entrega: el solicitante no registra", acc, KindDelivery, "req", ErrForbidden},
		{"entrega: estado equivocado", inProg, KindDelivery, "owner", ErrWrongStatus},
		{"entrega: ya confirmada", confirmed, KindDelivery, "owner", ErrAlreadyConfirmed},
		{"devolución: solicitante", inProg, KindReturn, "req", nil},
		{"devolución: el propietario no registra", inProg, KindReturn, "owner", ErrForbidden},
		{"devolución: reserva aceptada aún", acc, KindReturn, "req", ErrWrongStatus},
	}
	for _, c := range cases {
		if err := CanRecord(c.b, c.k, c.user); !errors.Is(err, c.want) && err != c.want {
			t.Errorf("%s: CanRecord = %v, quiero %v", c.name, err, c.want)
		}
	}
	if err := CanIssueQR(acc, KindDelivery, "owner"); !errors.Is(err, ErrNoEvidence) {
		t.Errorf("sin evidencia: %v", err)
	}
	if err := CanIssueQR(withEv, KindDelivery, "owner"); err != nil {
		t.Errorf("con evidencia: %v", err)
	}
	if err := CanIssueQR(withEv, KindDelivery, "req"); !errors.Is(err, ErrForbidden) {
		t.Errorf("rol equivocado: %v", err)
	}
}

func TestRecordBumpsVersionAndDropsQR(t *testing.T) {
	first := withQR("tok", t0)
	if first.EvidenceVersion != 1 || first.QR == nil {
		t.Fatalf("primera evidencia: %+v", first)
	}
	second := Record(first, "owner", "Nuevo estado", []images.Image{photo("p2")}, t0.Add(time.Minute))
	if second.EvidenceVersion != 2 {
		t.Errorf("evidenceVersion = %d, quiero 2", second.EvidenceVersion)
	}
	if second.QR != nil {
		t.Error("reemplazar la evidencia debe invalidar el QR (RF-01)")
	}
	if second.RecordedByID == nil || *second.RecordedByID != "owner" || second.RecordedAt == nil {
		t.Error("falta quién y cuándo registró")
	}
}

func TestNewTokenAndHash(t *testing.T) {
	tok, err := NewToken(bytes.NewReader(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 43 { // 32 bytes en base64url sin relleno
		t.Errorf("longitud del token = %d, quiero 43", len(tok))
	}
	if _, err := NewToken(bytes.NewReader([]byte{1, 2})); err == nil {
		t.Error("una fuente corta debe fallar")
	}
	a, _ := NewToken(nil)
	b, _ := NewToken(nil)
	if a == b || a == "" {
		t.Error("dos tokens aleatorios no deben coincidir")
	}
	h := HashToken(tok)
	if len(h) != 64 || strings.Contains(h, tok) {
		t.Errorf("hash = %q", h)
	}
	if HashToken(tok) != h || HashToken(tok+"x") == h {
		t.Error("el hash debe ser determinista y depender del token")
	}
}

func TestPayloadRoundTrip(t *testing.T) {
	p := Payload("b1", KindReturn, "a-b_c")
	if p != "vaiven://qr?b=b1&s=return&t=a-b_c" {
		t.Errorf("payload = %q", p)
	}
	id, k, tok, ok := ParsePayload(p)
	if !ok || id != "b1" || k != KindReturn || tok != "a-b_c" {
		t.Errorf("ParsePayload = %q %q %q %v", id, k, tok, ok)
	}
	for _, bad := range []string{"", "https://x", "vaiven://qr?b=1&s=delivery", "vaiven://qr?b=1&s=otra&t=x", "vaiven://qr?%zz"} {
		if _, _, _, ok := ParsePayload(bad); ok {
			t.Errorf("ParsePayload(%q) debe fallar", bad)
		}
	}
}

func TestStateOfAndVerifyQR(t *testing.T) {
	h := withQR("tok", t0)
	if QRTTL != 15*time.Minute {
		t.Fatalf("QRTTL = %v", QRTTL)
	}
	if StateOf(nil, t0) != QRNone || StateOf(&bookings.Handover{}, t0) != QRNone {
		t.Error("sin QR el estado es none")
	}
	if StateOf(h, t0.Add(14*time.Minute+59*time.Second)) != QRActive {
		t.Error("a los 14:59 sigue activo")
	}
	if StateOf(h, t0.Add(QRTTL)) != QRExpired {
		t.Error("a los 15:00 ya venció (límite exclusivo)")
	}
	if err := VerifyQR(h, "tok", t0.Add(time.Minute)); err != nil {
		t.Errorf("QR vigente: %v", err)
	}
	if err := VerifyQR(h, "otro", t0); !errors.Is(err, ErrQRInvalid) {
		t.Errorf("token ajeno: %v", err)
	}
	if err := VerifyQR(h, "tok", t0.Add(16*time.Minute)); !errors.Is(err, ErrQRExpired) {
		t.Errorf("CA-02, QR de hace 16 min: %v", err)
	}
	if err := VerifyQR(nil, "tok", t0); !errors.Is(err, ErrQRInvalid) {
		t.Errorf("sin etapa: %v", err)
	}
	used := t0.Add(time.Minute)
	h.QR.UsedAt = &used
	if err := VerifyQR(h, "tok", t0.Add(2*time.Minute)); !errors.Is(err, ErrQRUsed) || StateOf(h, t0) != QRUsed {
		t.Errorf("CA-03, QR usado: %v", err)
	}
	h.QR.UsedAt = nil
	h.EvidenceVersion = 2 // CA-04: cambió la evidencia sin limpiar el QR
	if err := VerifyQR(h, "tok", t0); !errors.Is(err, ErrQRInvalid) || StateOf(h, t0) != QRNone {
		t.Errorf("CA-04, evidencia cambiada: %v", err)
	}
}

func TestDeliveryWindow(t *testing.T) {
	from, until := DeliveryWindow(booking(bookings.StatusAccepted), loc)
	if !from.Equal(time.Date(2026, 11, 19, 6, 0, 0, 0, time.UTC)) { // medianoche de México = 06:00 UTC en noviembre
		t.Errorf("from = %v", from)
	}
	if !until.Equal(time.Date(2026, 11, 23, 6, 0, 0, 0, time.UTC)) {
		t.Errorf("until = %v", until)
	}
}

func TestPlanConfirmDelivery(t *testing.T) {
	b := booking(bookings.StatusAccepted)
	b.Delivery = withQR("tok", t0)
	at := t0.Add(2 * time.Minute)

	c, err := PlanConfirm(b, KindDelivery, "req", "tok", at, loc, fakeLate{})
	if err != nil {
		t.Fatalf("confirmación válida: %v", err)
	}
	if c.NewStatus != bookings.StatusInProgress || c.ConfirmedBy != "req" || !c.At.Equal(at) ||
		c.History.Status != bookings.StatusInProgress || c.History.By != bookings.RoleSystem {
		t.Errorf("Confirmation = %+v", c)
	}
	if c.LateAt == nil || !c.LateAt.Equal(time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("LateAt = %v, quiero fin del día + 2 h (08:00 UTC del 23)", c.LateAt)
	}
	if c.Adjustment != nil {
		t.Error("la entrega no lleva ajuste")
	}

	// Sin Lateness no hay LateAt ni ajuste, pero confirma.
	if c, err := PlanConfirm(b, KindDelivery, "req", "tok", at, loc, nil); err != nil || c.LateAt != nil {
		t.Errorf("sin Lateness: %+v %v", c, err)
	}
}

func TestPlanConfirmErrors(t *testing.T) {
	good := booking(bookings.StatusAccepted)
	good.Delivery = withQR("tok", t0)
	at := t0.Add(time.Minute)

	conf := good
	conf.Delivery = withQR("tok", t0)
	done := at
	conf.Delivery.ConfirmedAt = &done
	wrongStatus := good
	wrongStatus.Status = bookings.StatusInProgress

	cases := []struct {
		name  string
		b     bookings.Booking
		user  string
		token string
		at    time.Time
		want  error
	}{
		{"token vacío", good, "req", "  ", at, ErrTokenRequired},
		{"el propietario no confirma la entrega", good, "owner", "tok", at, ErrForbidden},
		{"ya confirmada", conf, "req", "tok", at, ErrAlreadyConfirmed},
		{"reserva en otro estado", wrongStatus, "req", "tok", at, ErrWrongStatus},
		{"token distinto", good, "req", "x", at, ErrQRInvalid},
		{"QR vencido", good, "req", "tok", t0.Add(16 * time.Minute), ErrQRExpired},
		{"demasiado pronto", good, "req", "tok", time.Date(2026, 11, 19, 5, 59, 0, 0, time.UTC), ErrOutsideWindow},
		{"después del último día", good, "req", "tok", time.Date(2026, 11, 23, 6, 0, 0, 0, time.UTC), ErrOutsideWindow},
	}
	for _, c := range cases {
		b := c.b
		if c.name == "demasiado pronto" || c.name == "después del último día" {
			b.Delivery = withQR("tok", c.at.Add(-time.Minute)) // QR vigente en ese instante
		}
		if _, err := PlanConfirm(b, KindDelivery, c.user, c.token, c.at, loc, nil); err != c.want {
			t.Errorf("%s: err = %v, quiero %v", c.name, err, c.want)
		}
	}
}

func TestPlanConfirmReturn(t *testing.T) {
	b := booking(bookings.StatusInProgress)
	h := Record(nil, "req", "Devuelto", []images.Image{photo("r1")}, t0)
	qr := NewQR(&h, "rtok", t0)
	h.QR = &qr
	b.Return = &h
	adj := &bookings.Adjustment{Type: bookings.AdjustmentLateReturn, LateDays: 2, AmountCents: 6000, Status: bookings.AdjustmentRecorded}
	// La devolución no tiene ventana: se confirma incluso mucho después del fin.
	at := t0.Add(5 * time.Minute)

	c, err := PlanConfirm(b, KindReturn, "owner", "rtok", at, loc, fakeLate{adj: adj})
	if err != nil {
		t.Fatal(err)
	}
	if c.NewStatus != bookings.StatusCompleted || c.Adjustment != adj || c.LateAt != nil {
		t.Errorf("Confirmation = %+v", c)
	}
	if _, err := PlanConfirm(b, KindReturn, "req", "rtok", at, loc, nil); err != ErrForbidden {
		t.Errorf("el solicitante no confirma la devolución: %v", err)
	}
}

func TestPlanConfirmNeedsTransition(t *testing.T) {
	// Defensa: si la máquina de estados de 007 no permitiera el paso, no confirma.
	b := booking(bookings.StatusAccepted)
	b.Delivery = withQR("tok", t0)
	if !bookings.CanTransition(bookings.StatusAccepted, bookings.StatusInProgress, bookings.RoleSystem) {
		t.Fatal("la máquina de 007 debe permitir accepted → in_progress")
	}
}
