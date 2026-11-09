package usecase

import (
	"context"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
)

// Accept acepta una solicitud pending (propietario) y reserva el periodo de
// forma atómica (RF-07). Las pendientes que se cruzan pasan a rejected y se
// les avisa.
func (s *Service) Accept(ctx context.Context, viewer Viewer, id string, meetingPoint *string) (BookingView, error) {
	b, role, err := s.load(ctx, viewer, id, false)
	if err != nil {
		return BookingView{}, err
	}
	mp, err := domain.NormalizeText("meetingPoint.description", meetingPoint, domain.MeetingPointMin, domain.MeetingPointMax)
	if err != nil {
		return BookingView{}, err
	}
	now := s.now()
	if err := b.Transition(domain.StatusAccepted, role, now); err != nil {
		return BookingView{}, err
	}
	if mp != nil {
		b.MeetingPoint = mp
	}
	acc, rejected, err := s.d.Repo.Accept(ctx, b, now)
	if err != nil {
		return BookingView{}, err
	}
	s.notify(ctx, acc.RequesterID, "booking.accepted", "Solicitud aceptada", "Aceptaron tu solicitud de "+acc.Item.Title+".", acc)
	for _, r := range rejected {
		s.notify(ctx, r.RequesterID, "booking.rejected", "Solicitud rechazada", r.Item.Title+" ya no está disponible en esas fechas.", r)
	}
	return s.view(ctx, acc, role)
}

// Reject rechaza una solicitud pending (propietario, RF-08).
func (s *Service) Reject(ctx context.Context, viewer Viewer, id string, reason *string) (BookingView, error) {
	b, role, err := s.load(ctx, viewer, id, false)
	if err != nil {
		return BookingView{}, err
	}
	r, err := domain.NormalizeText("reason", reason, 1, domain.ReasonMax)
	if err != nil {
		return BookingView{}, err
	}
	if err := b.Transition(domain.StatusRejected, role, s.now()); err != nil {
		return BookingView{}, err
	}
	b.RejectReason = r
	saved, err := s.d.Repo.Update(ctx, b)
	if err != nil {
		return BookingView{}, err
	}
	s.notify(ctx, saved.RequesterID, "booking.rejected", "Solicitud rechazada", "Rechazaron tu solicitud de "+saved.Item.Title+".", saved)
	return s.view(ctx, saved, role)
}

// Cancel cancela (RF-08): una pending solo quien la envió; una accepted
// cualquiera de las partes, con motivo obligatorio. Libera el periodo (CA-08).
func (s *Service) Cancel(ctx context.Context, viewer Viewer, id string, reason *string) (BookingView, error) {
	b, role, err := s.load(ctx, viewer, id, false)
	if err != nil {
		return BookingView{}, err
	}
	now := s.now()
	r, err := domain.NormalizeText("reason", reason, domain.ReasonMin, domain.ReasonMax)
	if err != nil {
		return BookingView{}, err
	}
	if b.EffectiveStatus(now) == domain.StatusAccepted && r == nil {
		return BookingView{}, domain.ErrReasonRequired
	}
	if err := b.Transition(domain.StatusCancelled, role, now); err != nil {
		return BookingView{}, err
	}
	b.CancelReason, b.CancelledBy = r, &role
	saved, err := s.d.Repo.Update(ctx, b)
	if err != nil {
		return BookingView{}, err
	}
	other := saved.OwnerID
	if role == domain.RoleOwner {
		other = saved.RequesterID
	}
	s.notify(ctx, other, "booking.cancelled", "Reserva cancelada", "Se canceló la reserva de "+saved.Item.Title+".", saved)
	return s.view(ctx, saved, role)
}

// SetMeetingPoint fija o cambia el punto de encuentro de una reserva
// accepted o in_progress (propietario, RF-09).
func (s *Service) SetMeetingPoint(ctx context.Context, viewer Viewer, id, description string) (BookingView, error) {
	b, role, err := s.load(ctx, viewer, id, false)
	if err != nil {
		return BookingView{}, err
	}
	if role != domain.RoleOwner {
		return BookingView{}, domain.ErrForbidden
	}
	if b.Status != domain.StatusAccepted && b.Status != domain.StatusInProgress {
		return BookingView{}, domain.ErrInvalidTransition
	}
	mp, err := domain.NormalizeText("description", &description, domain.MeetingPointMin, domain.MeetingPointMax)
	if err != nil {
		return BookingView{}, err
	}
	if mp == nil {
		return BookingView{}, domain.ErrMeetingPointRequired
	}
	b.MeetingPoint = mp
	b.UpdatedAt = s.now()
	saved, err := s.d.Repo.Update(ctx, b)
	if err != nil {
		return BookingView{}, err
	}
	s.notify(ctx, saved.RequesterID, "booking.meeting_point", "Punto de encuentro", "Se actualizó el punto de encuentro de "+saved.Item.Title+".", saved)
	return s.view(ctx, saved, role)
}
