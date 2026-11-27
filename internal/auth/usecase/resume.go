package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
)

// Límites de una pasada de reanudación.
const (
	jobBatch       = 50  // jobs reclamados por vuelta
	maxJobsPerPass = 500 // tope de jobs por pasada (el resto, en la siguiente)
	pendingBatch   = 100 // cuentas con deletionPending por pasada
)

// errStepNotConfigured: el job necesita un puerto que no se configuró.
var errStepNotConfigured = errors.New("auth: paso de baja sin configurar")

// Resumer completa los pasos posteriores al commit de las bajas (spec 021,
// RF-03, RF-04, RF-05; N-1): revoca el acceso en Redis, ejecuta los jobs de la
// bandeja de salida (borrar fotos, avisar a contrapartes) y quita la marca
// deletionPending cuando no queda nada. Todo es idempotente: repetir o reanudar
// da el mismo resultado (CA-12), así que lo pueden correr a la vez varias
// réplicas, el worker periódico y el comando de administración. No guarda
// estado propio (Art. XIV).
//
// Un fallo se registra como evento SIN datos personales (solo «user:<id>») y se
// reintenta después; nunca revierte la baja ni la deja a medias en MongoDB.
type Resumer struct {
	d ResumerDeps
}

// ResumerDeps son las dependencias del Resumer. Solo Users es obligatoria; sin
// Pending o sin Jobs no hay marca ni bandeja que reanudar.
type ResumerDeps struct {
	Users interface {
		ByID(ctx context.Context, id string) (domain.User, error)
	}
	Pending     PendingDeletions
	Jobs        Jobs
	Revocations Revocations
	AccessTTL   time.Duration
	Images      ImageDestroyer
	Notices     NoticeSender
	Events      SecurityEvents
	Now         func() time.Time // nil = time.Now
}

// NewResumer crea el Resumer.
func NewResumer(d ResumerDeps) (*Resumer, error) {
	if d.Users == nil {
		return nil, errors.New("auth: faltan dependencias de la reanudación de bajas")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Resumer{d: d}, nil
}

// PassReport resume una pasada: solo contadores, nunca ids ni datos.
type PassReport struct {
	JobsDone    int
	JobsRetried int
	JobsFailed  int // agotaron sus intentos en esta pasada
	Accounts    int // cuentas con deletionPending revisadas
	Cleared     int // cuentas a las que se les quitó la marca
}

func (r *Resumer) now() time.Time { return r.d.Now().UTC().Truncate(time.Millisecond) }

func (r *Resumer) event(ctx context.Context, name, result, severity, userID string) {
	if r.d.Events != nil {
		subject := "unknown"
		if userID != "" {
			subject = "user:" + userID
		}
		r.d.Events.Record(ctx, SecurityEvent{Name: name, Result: result, Severity: severity, Subject: subject})
	}
}

// ResumeAccount completa los pasos pendientes de una cuenta ya dada de baja:
// revoca todos sus tokens de acceso (Redis) y, si no le quedan jobs por hacer,
// quita la marca deletionPending. Devuelve true si la quitó. Nunca devuelve
// error: un fallo se registra y la cuenta sigue marcada para el siguiente intento.
func (r *Resumer) ResumeAccount(ctx context.Context, userID string) bool {
	u, err := r.d.Users.ByID(ctx, userID)
	if err != nil || u.IsActive() {
		r.event(ctx, "account.deletion_resume", "failure", "medium", userID)
		return false
	}
	if r.d.Revocations != nil {
		cutoff := r.now()
		if u.DeactivatedAt != nil {
			cutoff = *u.DeactivatedAt
		}
		if err := r.d.Revocations.RevokeUserBefore(ctx, userID, cutoff, r.d.AccessTTL); err != nil {
			r.event(ctx, "account.deletion_revoke", "failure", "medium", userID)
			return false
		}
	}
	if r.d.Pending == nil {
		return false
	}
	open := int64(0)
	if r.d.Jobs != nil {
		if open, err = r.d.Jobs.Open(ctx, userID); err != nil {
			r.event(ctx, "account.deletion_resume", "failure", "medium", userID)
			return false
		}
	}
	if open > 0 {
		return false
	}
	cleared, err := r.d.Pending.ClearIfDone(ctx, userID, r.now())
	if err != nil {
		r.event(ctx, "account.deletion_resume", "failure", "medium", userID)
		return false
	}
	return cleared
}

// RunJobs reclama y ejecuta los jobs vencidos de la bandeja (hasta
// maxJobsPerPass). Un job que sale bien se borra; uno que falla se reprograma
// con espera exponencial (domain.JobBackoff) y, tras domain.MaxJobAttempts
// intentos, queda como fallido para operación con el evento
// deletion.job_failed. Si ctx se cancela se detiene sin contar el fallo: el
// arrendamiento vence y otro lo retoma.
func (r *Resumer) RunJobs(ctx context.Context) (PassReport, error) {
	var rep PassReport
	if r.d.Jobs == nil {
		return rep, nil
	}
	for handled := 0; handled < maxJobsPerPass && ctx.Err() == nil; {
		jobs, err := r.d.Jobs.Claim(ctx, r.now(), jobBatch)
		if err != nil {
			r.event(ctx, "deletion.job_claim", "failure", "medium", "")
			return rep, err
		}
		if len(jobs) == 0 {
			break
		}
		for _, j := range jobs {
			handled++
			if ctx.Err() != nil {
				return rep, nil
			}
			r.runJob(ctx, j, &rep)
		}
	}
	return rep, nil
}

func (r *Resumer) runJob(ctx context.Context, j Job, rep *PassReport) {
	err := r.execute(ctx, j)
	if err == nil {
		if err := r.d.Jobs.Done(ctx, j.ID); err != nil {
			// El job se repetirá al vencer su arrendamiento: es idempotente.
			r.event(ctx, "deletion.job_done", "failure", "medium", j.UserID)
			return
		}
		rep.JobsDone++
		return
	}
	if ctx.Err() != nil {
		return // cancelado: no cuenta como intento fallido
	}
	now := r.now()
	f := JobFailure{At: now, Next: now.Add(domain.JobBackoff(j.Attempts)), Exhausted: domain.JobExhausted(j.Attempts), Reason: string(j.Kind) + ": falló el paso"}
	if err := r.d.Jobs.Fail(ctx, j.ID, f); err != nil {
		r.event(ctx, "deletion.job_fail_record", "failure", "medium", j.UserID)
	}
	if f.Exhausted {
		rep.JobsFailed++
		r.event(ctx, "deletion.job_failed", "failure", "high", j.UserID)
		return
	}
	rep.JobsRetried++
	r.event(ctx, "deletion.job_retry", "failure", "low", j.UserID)
}

func (r *Resumer) execute(ctx context.Context, j Job) error {
	switch j.Kind {
	case domain.JobImage:
		if r.d.Images == nil || j.Image == nil {
			return errStepNotConfigured
		}
		return r.d.Images.Destroy(ctx, *j.Image)
	case domain.JobNotice:
		if r.d.Notices == nil || j.Notice == nil {
			return errStepNotConfigured
		}
		return r.d.Notices.Send(ctx, *j.Notice)
	}
	return errStepNotConfigured
}

// ResumeDeletions es una pasada completa de reanudación (spec 021, N-1): primero los jobs
// vencidos y después las cuentas marcadas con deletionPending. Es lo que llaman
// el worker periódico y `admin resume-deletions`.
func (r *Resumer) ResumeDeletions(ctx context.Context) (PassReport, error) {
	rep, err := r.RunJobs(ctx)
	if err != nil {
		return rep, err
	}
	if r.d.Pending == nil {
		return rep, nil
	}
	ids, err := r.d.Pending.ListPending(ctx, pendingBatch)
	if err != nil {
		r.event(ctx, "account.deletion_resume", "failure", "medium", "")
		return rep, err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		rep.Accounts++
		if r.ResumeAccount(ctx, id) {
			rep.Cleared++
		}
	}
	return rep, nil
}
