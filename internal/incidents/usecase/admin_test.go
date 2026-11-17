package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/domain"
)

func TestAdminListDefaultsToOpenOldestFirstWithAllNotes(t *testing.T) { // RF-08
	e := newEnv(t, booking(bookings.StatusAccepted))
	ctx := context.Background()
	for n := 0; n < 3; n++ {
		e.repo.rows = append(e.repo.rows, domain.Incident{
			ID: e.repo.NewID(), BookingID: fmt.Sprintf("b%d", n), ReporterID: requesterID, ReportedID: ownerID,
			Status: domain.StatusOpen, CreatedAt: completedAt.Add(time.Duration(n) * time.Minute),
			Notes: []domain.Note{{Text: "interna", VisibleToParties: false}, {Text: "pública", VisibleToParties: true}},
		})
	}
	e.repo.rows[1].Status = domain.StatusClosed

	p, err := e.svc.AdminList(ctx, "", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if e.repo.lastStatus != domain.StatusOpen || e.repo.listed != "all" {
		t.Errorf("sin status se listan las abiertas: %q %q", e.repo.listed, e.repo.lastStatus)
	}
	if len(p.Items) != 1 || p.Next == nil || p.Items[0].Incident.BookingID != "b0" {
		t.Fatalf("p = %+v", p)
	}
	if !p.Items[0].AdminView || len(p.Items[0].Notes) != 2 {
		t.Errorf("el admin ve todas las notas: %+v", p.Items[0])
	}
	p2, err := e.svc.AdminList(ctx, domain.StatusOpen, p.Next, 5)
	if err != nil || len(p2.Items) != 1 || p2.Items[0].Incident.BookingID != "b2" || p2.Next != nil {
		t.Errorf("p2 = %+v (%v)", p2, err)
	}
	closed, err := e.svc.AdminList(ctx, domain.StatusClosed, nil, 5)
	if err != nil || len(closed.Items) != 1 || closed.Items[0].Incident.BookingID != "b1" {
		t.Errorf("cerradas = %+v (%v)", closed, err)
	}
}

func TestAddNoteVisibleNotifiesBothParties(t *testing.T) {
	e := newEnv(t, booking(bookings.StatusAccepted))
	inc := seedIncident(t, e)
	e.not.sent = nil
	v, err := e.svc.AddNote(context.Background(), adminID, inc.ID, "  Por favor envía una foto.  ", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Notes) != 1 || v.Notes[0].Text != "Por favor envía una foto." || !v.Notes[0].VisibleToParties ||
		v.Notes[0].ByAdminID != adminID || !v.AdminView || v.Incident.Version != 2 {
		t.Errorf("v = %+v", v)
	}
	if got := e.not.to(); got != EventNote+":"+requesterID+","+EventNote+":"+ownerID {
		t.Errorf("avisos = %q, quiero a ambas partes", got)
	}
	for _, s := range e.not.sent {
		if strings.Contains(fmt.Sprint(s.e), "Por favor envía") {
			t.Error("el aviso no lleva el texto de la nota")
		}
	}
}

func TestAddNoteInternalDoesNotNotify(t *testing.T) {
	e := newEnv(t, booking(bookings.StatusAccepted))
	inc := seedIncident(t, e)
	e.not.sent = nil
	v, err := e.svc.AddNote(context.Background(), adminID, inc.ID, "revisar historial del usuario", false)
	if err != nil || len(v.Notes) != 1 || len(e.not.sent) != 0 {
		t.Errorf("v=%+v err=%v avisos=%q", v, err, e.not.to())
	}
}

func TestAddNoteRules(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, booking(bookings.StatusAccepted))
	inc := seedIncident(t, e)

	if _, err := e.svc.AddNote(ctx, adminID, inc.ID, "ab", true); err == nil {
		t.Error("texto corto debe ser inválido")
	}
	if _, err := e.svc.AddNote(ctx, adminID, "6731a0000000000000000099", "nota válida", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	for n := 1; n <= domain.MaxNotes; n++ {
		if _, err := e.svc.AddNote(ctx, adminID, inc.ID, fmt.Sprintf("nota número %d", n), false); err != nil {
			t.Fatalf("nota %d: %v", n, err)
		}
	}
	if _, err := e.svc.AddNote(ctx, adminID, inc.ID, "una más", false); !errors.Is(err, domain.ErrTooManyNotes) {
		t.Errorf("la 11.ª: %v", err)
	}

	e2 := newEnv(t, booking(bookings.StatusAccepted))
	inc2 := seedIncident(t, e2)
	if _, err := e2.svc.Close(ctx, adminID, inc2.ID, "Resuelto con ambas partes.", domain.OutcomeNone); err != nil {
		t.Fatal(err)
	}
	if _, err := e2.svc.AddNote(ctx, adminID, inc2.ID, "nota tardía", true); !errors.Is(err, domain.ErrNotOpen) {
		t.Errorf("nota en incidencia cerrada: %v", err)
	}
}

func TestAddNoteRaceExplained(t *testing.T) {
	ctx := context.Background()
	// Otra operación la cerró entre la lectura y la escritura atómica: el
	// repositorio no aplica y el caso de uso explica el motivo real.
	e := newEnv(t, booking(bookings.StatusAccepted))
	inc := seedIncident(t, e)
	e.repo.noteErr = ErrNoMatch
	e.repo.rows[0].Status = domain.StatusClosed
	// Para que la regla previa pase, devolvemos abierta en la primera lectura.
	flip := &flipRepo{fakeRepo: e.repo, id: inc.ID}
	e.svc.d.Repo = flip
	if _, err := e.svc.AddNote(ctx, adminID, inc.ID, "nota válida", true); !errors.Is(err, domain.ErrNotOpen) {
		t.Errorf("cerrada por otra operación: %v", err)
	}

	// Si al releer la regla ya pasa, se informa que la incidencia cambió.
	e = newEnv(t, booking(bookings.StatusAccepted))
	inc = seedIncident(t, e)
	e.repo.hideOnNext = true
	if _, err := e.svc.AddNote(ctx, adminID, inc.ID, "nota válida", true); !errors.Is(err, domain.ErrChanged) {
		t.Errorf("cambio simultáneo: %v", err)
	}

	boom := errors.New("mongo caído")
	e = newEnv(t, booking(bookings.StatusAccepted))
	inc = seedIncident(t, e)
	e.repo.noteErr = boom
	if _, err := e.svc.AddNote(ctx, adminID, inc.ID, "nota válida", true); !errors.Is(err, boom) {
		t.Errorf("error del repositorio: %v", err)
	}
}

// flipRepo devuelve la incidencia abierta en la primera lectura y cerrada en
// las siguientes, para probar la explicación de un ErrNoMatch.
type flipRepo struct {
	*fakeRepo
	id    string
	reads int
}

func (f *flipRepo) ByID(ctx context.Context, id string) (domain.Incident, error) {
	f.reads++
	i, err := f.fakeRepo.ByID(ctx, id)
	if f.reads == 1 {
		i.Status = domain.StatusOpen
	}
	return i, err
}

func (f *flipRepo) AddNote(context.Context, string, domain.Note) (domain.Incident, error) {
	return domain.Incident{}, ErrNoMatch
}

func (f *flipRepo) Close(context.Context, string, domain.FinalResponse) (domain.Incident, error) {
	return domain.Incident{}, ErrNoMatch
}

func TestCloseNotifiesBothPartiesAndRecordsOutcome(t *testing.T) { // RF-08, CA-07
	e := newEnv(t, booking(bookings.StatusAccepted))
	inc := seedIncident(t, e)
	e.not.sent = nil
	*e.clock = e.clock.Add(time.Hour)
	v, err := e.svc.Close(context.Background(), adminID, inc.ID, "  Se acordó un ajuste entre las partes.  ", domain.OutcomeMonetaryAdjustment)
	if err != nil {
		t.Fatal(err)
	}
	i := v.Incident
	if i.Status != domain.StatusClosed || i.Final == nil || i.Final.Text != "Se acordó un ajuste entre las partes." ||
		i.Final.Outcome != domain.OutcomeMonetaryAdjustment || i.Final.ByAdminID != adminID ||
		i.ClosedAt == nil || !i.ClosedAt.Equal(*e.clock) || !v.AdminView {
		t.Errorf("incidencia cerrada = %+v", i)
	}
	if got := e.not.to(); got != EventClosed+":"+requesterID+","+EventClosed+":"+ownerID {
		t.Errorf("avisos = %q", got)
	}
}

func TestCloseRules(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, booking(bookings.StatusAccepted))
	inc := seedIncident(t, e)

	for _, bad := range []struct {
		text    string
		outcome domain.Outcome
	}{{"abc", domain.OutcomeNone}, {"respuesta válida", "x"}, {"", ""}} {
		if _, err := e.svc.Close(ctx, adminID, inc.ID, bad.text, bad.outcome); err == nil {
			t.Errorf("Close(%q,%q) debe ser inválido", bad.text, bad.outcome)
		}
	}
	if e.repo.rows[0].Status != domain.StatusOpen {
		t.Fatal("una validación fallida no cierra")
	}
	if _, err := e.svc.Close(ctx, adminID, "6731a0000000000000000099", "Respuesta final.", domain.OutcomeNone); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	if _, err := e.svc.Close(ctx, adminID, inc.ID, "Respuesta final.", domain.OutcomeRestriction); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Close(ctx, adminID, inc.ID, "Otra respuesta.", domain.OutcomeNone); !errors.Is(err, domain.ErrNotOpen) {
		t.Errorf("cerrar dos veces: %v", err)
	}
}

func TestCloseRaceExplainedAndInfraErrors(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, booking(bookings.StatusAccepted))
	inc := seedIncident(t, e)
	e.repo.rows[0].Status = domain.StatusClosed
	e.svc.d.Repo = &flipRepo{fakeRepo: e.repo, id: inc.ID}
	if _, err := e.svc.Close(ctx, adminID, inc.ID, "Respuesta final.", domain.OutcomeNone); !errors.Is(err, domain.ErrNotOpen) {
		t.Errorf("cerrada por otra operación: %v", err)
	}

	e = newEnv(t, booking(bookings.StatusAccepted))
	inc = seedIncident(t, e)
	e.repo.hideOnNext = true
	if _, err := e.svc.Close(ctx, adminID, inc.ID, "Respuesta final.", domain.OutcomeNone); !errors.Is(err, domain.ErrChanged) {
		t.Errorf("cambio simultáneo: %v", err)
	}

	boom := errors.New("mongo caído")
	e = newEnv(t, booking(bookings.StatusAccepted))
	inc = seedIncident(t, e)
	e.repo.closeErr = boom
	if _, err := e.svc.Close(ctx, adminID, inc.ID, "Respuesta final.", domain.OutcomeNone); !errors.Is(err, boom) {
		t.Errorf("error del repositorio: %v", err)
	}
}

func TestCloseDoesNotChangeTheBooking(t *testing.T) { // CA-04
	b := booking(bookings.StatusInProgress)
	e := newEnv(t, b)
	inc := seedIncident(t, e)
	if _, err := e.svc.Close(context.Background(), adminID, inc.ID, "Respuesta final.", domain.OutcomeRestriction); err != nil {
		t.Fatal(err)
	}
	if b.Status != bookings.StatusInProgress {
		t.Error("cerrar una incidencia no cambia la reserva")
	}
}
