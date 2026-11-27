package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// noTx ejecuta fn sin transacción: es el valor por omisión de Deps.Tx para las
// pruebas y los comandos que no tienen replica set.
type noTx struct{}

func (noTx) Do(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

// Deactivate es el caso de uso de DELETE /me; delega en DeleteAccount.
func (s *Service) Deactivate(ctx context.Context, userID, password string) error {
	return s.DeleteAccount(ctx, userID, password)
}

// DeleteAccount da de baja la cuenta (RF-08, POL-17; spec 021). Orden:
//
//  1. Fuera de la transacción: contraseña (argon2id, lo caro) y una primera
//     comprobación de reservas activas para fallar rápido (403 / 409).
//  2. Una sola transacción: cerrojo de la cuenta, segunda comprobación de
//     reservas activas, erasers de cada módulo en el orden de Deps.Erasers,
//     borrado de sesiones, bandeja de salida y, al final, la cuenta. Un error de
//     cualquier paso (409 incluido) aborta todo y nada cambia.
//  3. Tras el commit: revocar el acceso en Redis y completar los pasos
//     pendientes. Un fallo aquí NO revierte la baja: la cuenta queda marcada
//     (deletionPending) y la reanuda el worker o el comando de administración.
//
// Una cuenta que ya estaba dada de baja devuelve éxito sin repetir efectos
// (RF-05, CA-12), salvo reanudar sus pasos pendientes.
func (s *Service) DeleteAccount(ctx context.Context, userID, password string) error {
	u, err := s.d.Users.ByID(ctx, userID)
	if err != nil {
		if domain.Is(err, domain.ErrUserNotFound) {
			return domain.ErrInvalidSession
		}
		return fmt.Errorf("auth: baja: %w", err)
	}
	if !u.IsActive() {
		// Reintento sobre una cuenta ya eliminada: idempotente.
		s.resumer.ResumeAccount(ctx, u.ID)
		return nil
	}
	ok, err := s.checkPassword(ctx, &u, password)
	if err != nil {
		return fmt.Errorf("auth: baja: %w", err)
	}
	if !ok {
		return domain.ErrWrongPassword
	}
	active, err := s.d.Bookings.HasActiveBookings(ctx, u.ID)
	if err != nil {
		return fmt.Errorf("auth: baja: reservas activas: %w", err)
	}
	if active {
		return domain.ErrActiveBookings
	}

	now := s.now()
	deleted := false
	err = s.d.Tx.Do(ctx, func(tx context.Context) error {
		deleted = false // el driver puede repetir la función
		return s.eraseInTx(tx, u.ID, now, &deleted)
	})
	if err != nil {
		if domain.Is(err, domain.ErrActiveBookings) {
			return err
		}
		return fmt.Errorf("auth: baja: %w", err)
	}
	if !deleted {
		// Otra baja ganó la carrera: ya no hay nada que hacer aquí.
		s.resumer.ResumeAccount(ctx, u.ID)
		return nil
	}
	s.event(ctx, "account.deleted", "success", "info", "user:"+u.ID)
	s.resumer.ResumeAccount(ctx, u.ID)
	return nil
}

// eraseInTx es el cuerpo de la transacción de la baja. Pone *deleted en true si
// esta ejecución dio de baja la cuenta.
func (s *Service) eraseInTx(tx context.Context, userID string, now time.Time, deleted *bool) error {
	locked, err := s.d.Users.LockActive(tx, userID)
	if err != nil {
		return fmt.Errorf("cerrojo de la cuenta: %w", err)
	}
	if !locked {
		return nil // la cuenta ya no está activa (RF-05)
	}
	// Segunda comprobación, ya dentro de la transacción: si una reserva se
	// aceptó entre la primera y esta, la baja se rechaza sin cambios (CA-04).
	active, err := s.d.Bookings.HasActiveBookings(tx, userID)
	if err != nil {
		return fmt.Errorf("reservas activas: %w", err)
	}
	if active {
		return domain.ErrActiveBookings
	}
	var jobs []Job
	for _, e := range s.d.Erasers {
		res, err := e.Erase(tx, userID, now)
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		for i := range res.Images {
			img := res.Images[i]
			next := now
			if !img.NotBefore.IsZero() {
				next = img.NotBefore
			}
			jobs = append(jobs, Job{UserID: userID, Kind: domain.JobImage, Image: &img, NextAttemptAt: next, Retention: next.After(now)})
		}
		for i := range res.Notices {
			n := res.Notices[i]
			jobs = append(jobs, Job{UserID: userID, Kind: domain.JobNotice, Notice: &n, NextAttemptAt: now})
		}
	}
	if err := s.d.Refresh.DeleteAllForUser(tx, userID); err != nil {
		return fmt.Errorf("borrar sesiones: %w", err)
	}
	if s.d.Jobs != nil && len(jobs) > 0 {
		if err := s.d.Jobs.Enqueue(tx, jobs, now); err != nil {
			return fmt.Errorf("bandeja de salida: %w", err)
		}
	}
	if err := s.d.Users.Deactivate(tx, userID, domain.AnonymizedEmail(userID), now); err != nil {
		return fmt.Errorf("cuenta: %w", err)
	}
	*deleted = true
	return nil
}
