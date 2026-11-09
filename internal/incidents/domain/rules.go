package domain

import (
	"time"

	bookings "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/policy"
)

// Errores de incidencias; el adaptador HTTP los traduce con el envoltorio del
// Art. IV.
var (
	// ErrNotFound: la incidencia no existe o quien consulta no es parte ni admin.
	ErrNotFound = shared.NewNotFound("No encontramos la incidencia.")
	// ErrBookingStatus: la reserva no está en un estado que admita reportes.
	ErrBookingStatus = shared.NewConflict("Solo puedes reportar una reserva aceptada, en curso o completada hace poco.")
	// ErrWindowClosed: pasaron más de 72 h desde que se completó (POL-08).
	ErrWindowClosed = shared.NewConflict("Ya pasó el plazo de 72 horas para reportar esta reserva.")
	// ErrTooEarly: «no se presentó» solo después de la fecha de inicio (RF-05).
	ErrTooEarly = shared.NewConflict("Solo puedes reportar que alguien no se presentó cuando ya llegó la fecha de inicio.")
	// ErrTooManyOpen: ya hay 3 incidencias abiertas en la reserva (CA-08).
	ErrTooManyOpen = shared.NewConflict("Esta reserva ya tiene 3 incidencias abiertas.")
	// ErrNotOpen: la incidencia ya está cerrada (RF-06, RF-08).
	ErrNotOpen = shared.NewConflict("La incidencia ya está cerrada.")
	// ErrTooManyNotes: la incidencia ya tiene 10 notas (RF-08).
	ErrTooManyNotes = shared.NewConflict("La incidencia ya tiene el máximo de 10 notas.")
	// ErrChanged: otra operación cambió la incidencia a la vez.
	ErrChanged = shared.NewConflict("La incidencia cambió mientras la procesabas. Vuelve a cargarla.")
)

// completedAt devuelve cuándo se completó la reserva: la última entrada
// `completed` del historial que registra la spec 009.
func completedAt(b bookings.Booking) (time.Time, bool) {
	for i := len(b.History) - 1; i >= 0; i-- {
		if b.History[i].Status == bookings.StatusCompleted {
			return b.History[i].At.UTC(), true
		}
	}
	return time.Time{}, false
}

// CanOpen aplica las reglas de apertura (RF-05) a userID sobre la reserva b,
// que ya tiene openCount incidencias abiertas: ser parte (si no, ErrNotFound),
// el estado de la reserva (aceptada, en curso o completada hace menos de 72 h),
// `no_show` solo desde la fecha de inicio y máximo 3 abiertas. loc es la zona
// de las fechas de calendario (America/Mexico_City).
func CanOpen(b bookings.Booking, userID string, reason Reason, openCount int, now time.Time, loc *time.Location) error {
	if _, ok := b.RoleOf(userID); !ok {
		return bookings.ErrNotFound
	}
	switch b.Status {
	case bookings.StatusAccepted, bookings.StatusInProgress:
	case bookings.StatusCompleted:
		at, ok := completedAt(b)
		if !ok || !now.Before(at.Add(time.Duration(policy.POL08IncidentWindowHours)*time.Hour)) {
			return ErrWindowClosed
		}
	default:
		return ErrBookingStatus
	}
	if reason == ReasonNoShow && now.Before(b.Start.StartOf(loc)) {
		return ErrTooEarly
	}
	if openCount >= policy.POL08MaxOpenIncidents {
		return ErrTooManyOpen
	}
	return nil
}

// ValidateInput valida el motivo, la descripción (10 a 1000 caracteres,
// recortada) y la cantidad de imágenes de evidencia (0 a 5), y devuelve la
// descripción normalizada.
func ValidateInput(reason Reason, description string, evidenceCount int) (string, error) {
	var d []shared.Detail
	if !reason.Valid() {
		d = append(d, shared.Detail{Field: "reason", Issue: "debe ser no_show, late_return, damaged, not_returned, payment_dispute u other"})
	}
	text, n := trimmedLen(description)
	if n < DescriptionMin || n > DescriptionMax {
		d = append(d, shared.Detail{Field: "description", Issue: lengthIssue(DescriptionMin, DescriptionMax)})
	}
	if evidenceCount < 0 || evidenceCount > MaxEvidence {
		d = append(d, shared.Detail{Field: "evidence", Issue: "puede tener de 0 a 5 imágenes"})
	}
	if len(d) > 0 {
		return "", shared.NewValidation(MessageInvalidData, d...)
	}
	return text, nil
}

// New arma la incidencia abierta de reporterID contra la otra parte de b.
// description ya viene validada por ValidateInput.
func New(id string, b bookings.Booking, reporterID string, reason Reason, description string, evidence []images.Image, now time.Time) (Incident, error) {
	role, ok := b.RoleOf(reporterID)
	if !ok {
		return Incident{}, bookings.ErrNotFound
	}
	reportedID := b.RequesterID
	if role == bookings.RoleRequester {
		reportedID = b.OwnerID
	}
	if evidence == nil {
		evidence = []images.Image{}
	}
	return Incident{
		ID: id, BookingID: b.ID, ReporterID: reporterID, ReportedID: reportedID,
		Reason: reason, Description: description, Evidence: evidence,
		Status: StatusOpen, Notes: []Note{},
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(), Version: 1,
	}, nil
}

// CanAddNote indica si la incidencia admite otra nota: debe estar abierta y
// tener menos de 10.
func CanAddNote(i Incident) error {
	if i.Status != StatusOpen {
		return ErrNotOpen
	}
	if len(i.Notes) >= MaxNotes {
		return ErrTooManyNotes
	}
	return nil
}

// NewNote valida el texto (3 a 1000 caracteres) y arma la nota del
// administrador.
func NewNote(text string, visibleToParties bool, adminID string, now time.Time) (Note, error) {
	t, n := trimmedLen(text)
	if n < NoteMin || n > NoteMax {
		return Note{}, invalid("text", lengthIssue(NoteMin, NoteMax))
	}
	return Note{Text: t, At: now.UTC(), ByAdminID: adminID, VisibleToParties: visibleToParties}, nil
}

// CanClose indica si la incidencia se puede cerrar: solo desde `open`.
func CanClose(i Incident) error {
	if i.Status != StatusOpen {
		return ErrNotOpen
	}
	return nil
}

// NewFinalResponse valida la respuesta final (5 a 1000 caracteres) y el
// resultado, y arma el cierre.
func NewFinalResponse(text string, outcome Outcome, adminID string, now time.Time) (FinalResponse, error) {
	var d []shared.Detail
	t, n := trimmedLen(text)
	if n < FinalMin || n > FinalMax {
		d = append(d, shared.Detail{Field: "finalResponse", Issue: lengthIssue(FinalMin, FinalMax)})
	}
	if !outcome.Valid() {
		d = append(d, shared.Detail{Field: "outcome", Issue: "debe ser none, monetary_adjustment o restriction"})
	}
	if len(d) > 0 {
		return FinalResponse{}, shared.NewValidation(MessageInvalidData, d...)
	}
	return FinalResponse{Text: t, Outcome: outcome, ByAdminID: adminID, At: now.UTC()}, nil
}

// Close devuelve la incidencia cerrada con la respuesta final (sin guardar).
func Close(i Incident, f FinalResponse) (Incident, error) {
	if err := CanClose(i); err != nil {
		return Incident{}, err
	}
	i.Status = StatusClosed
	i.Final = &f
	closed := f.At
	i.ClosedAt = &closed
	i.UpdatedAt = f.At
	return i, nil
}
