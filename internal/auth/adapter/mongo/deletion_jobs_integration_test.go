//go:build integration

package mongo

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
)

func imageJob(user, pid string, next time.Time) usecase.Job {
	return usecase.Job{UserID: user, Kind: domain.JobImage, Image: &usecase.ImageRef{PublicID: pid, Delivery: "upload"}, NextAttemptAt: next}
}

// retentionJob es un job de imagen programado por la retención de 12 meses.
func retentionJob(user, pid string, next time.Time) usecase.Job {
	j := imageJob(user, pid, next)
	j.Retention = true
	return j
}

func noticeJob(user, to string) usecase.Job {
	return usecase.Job{UserID: user, Kind: domain.JobNotice, Notice: &usecase.Notice{
		UserID: to, Type: "booking.cancelled", Title: "Reserva cancelada", Body: "Se canceló la reserva.",
		Data: map[string]string{"bookingId": "b1"},
	}}
}

// Spec 021, T014: la bandeja de salida (outbox) de la baja.
func TestIntegrationDeletionJobsLifecycle(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	repo := NewDeletionJobs(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)

	if err := repo.Enqueue(ctx, nil, now); err != nil {
		t.Fatalf("Enqueue vacío: %v", err)
	}
	if err := repo.Enqueue(ctx, []usecase.Job{
		imageJob("u1", "p1", time.Time{}),
		retentionJob("u1", "evidencia", now.Add(24*time.Hour)), // con fecha (retención): aún no vence
		noticeJob("u1", "u2"),
		imageJob("u2", "otra-cuenta", time.Time{}),
	}, now); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Ningún documento lleva null.
	cur, err := repo.col.Find(ctx, bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	var raws []bson.Raw
	if err := cur.All(ctx, &raws); err != nil || len(raws) != 4 {
		t.Fatalf("documentos = %d, %v", len(raws), err)
	}
	for _, raw := range raws {
		if n := nullPaths(t, raw, ""); len(n) != 0 {
			t.Errorf("job con null en %v", n)
		}
		for _, f := range []string{"leaseUntil", "failedAt", "lastError"} {
			if _, err := raw.LookupErr(f); err == nil {
				t.Errorf("%s no debe existir en un job nuevo", f)
			}
		}
	}

	// El job de retención espera su fecha y no cuenta como paso pendiente de la baja.
	if n, err := repo.Open(ctx, "u1"); err != nil || n != 2 {
		t.Fatalf("Open(u1) = %d, %v, want 2", n, err)
	}
	if n, _ := repo.Open(ctx, "nadie"); n != 0 {
		t.Errorf("Open(nadie) = %d", n)
	}

	// Claim: solo los vencidos; el de la evidencia espera a su fecha.
	got, err := repo.Claim(ctx, now, 10)
	if err != nil || len(got) != 3 {
		t.Fatalf("Claim = %d jobs, %v, want 3", len(got), err)
	}
	var notice *usecase.Job
	for i := range got {
		if got[i].Attempts != 1 {
			t.Errorf("Attempts = %d, want 1 tras reclamar", got[i].Attempts)
		}
		if got[i].Kind == domain.JobNotice {
			notice = &got[i]
		}
	}
	if notice == nil || notice.Notice == nil || notice.Notice.UserID != "u2" || notice.Notice.Data["bookingId"] != "b1" || notice.Notice.Title == "" {
		t.Fatalf("job de aviso = %+v", notice)
	}
	// Mientras dura el arrendamiento nadie más los reclama.
	if again, _ := repo.Claim(ctx, now.Add(time.Minute), 10); len(again) != 0 {
		t.Errorf("reclamo con arrendamiento vigente devolvió %d jobs", len(again))
	}
	// Pasado el arrendamiento (el proceso cayó) se vuelven a reclamar.
	later, _ := repo.Claim(ctx, now.Add(DefaultJobLease+time.Second), 10)
	if len(later) != 3 || later[0].Attempts != 2 {
		t.Fatalf("reclamo tras vencer el arrendamiento = %+v", later)
	}

	// Done borra; un job inexistente no es error.
	for _, j := range later {
		if err := repo.Done(ctx, j.ID); err != nil {
			t.Fatalf("Done: %v", err)
		}
	}
	if err := repo.Done(ctx, later[0].ID); err != nil {
		t.Errorf("Done repetido = %v", err)
	}
	if err := repo.Done(ctx, "no-es-id"); err == nil {
		t.Error("Done con id inválido debe fallar")
	}
	if n, _ := repo.Open(ctx, "u1"); n != 0 { // solo queda la evidencia con fecha (retención)
		t.Errorf("Open(u1) tras Done = %d, want 0", n)
	}
	// La evidencia se reclama cuando llega su fecha.
	due, _ := repo.Claim(ctx, now.Add(25*time.Hour), 10)
	if len(due) != 1 || due[0].Image == nil || due[0].Image.PublicID != "evidencia" {
		t.Errorf("evidencia vencida = %+v", due)
	}
}

func TestIntegrationDeletionJobsFail(t *testing.T) {
	ctx := context.Background()
	repo := NewDeletionJobs(newDB(t))
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := repo.Enqueue(ctx, []usecase.Job{imageJob("u1", "p1", time.Time{})}, now); err != nil {
		t.Fatal(err)
	}
	jobs, _ := repo.Claim(ctx, now, 5)
	if len(jobs) != 1 {
		t.Fatal("se esperaba un job")
	}
	id := jobs[0].ID

	// Fallo con espera exponencial: se libera el arrendamiento y se reprograma.
	next := now.Add(domain.JobBackoff(1))
	if err := repo.Fail(ctx, id, usecase.JobFailure{At: now, Next: next, Reason: "cloudinary 500"}); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if early, _ := repo.Claim(ctx, now.Add(30*time.Second), 5); len(early) != 0 {
		t.Error("antes de la espera no se reclama")
	}
	due, _ := repo.Claim(ctx, next, 5)
	if len(due) != 1 || due[0].Attempts != 2 {
		t.Fatalf("tras la espera = %+v", due)
	}

	// Agotado: queda con failedAt y sin nextAttemptAt, fuera de Claim y de Open.
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	if err := repo.Fail(ctx, id, usecase.JobFailure{At: next, Exhausted: true, Reason: string(long)}); err != nil {
		t.Fatalf("Fail agotado: %v", err)
	}
	raw := rawJob(t, repo, id)
	if _, err := raw.LookupErr("failedAt"); err != nil {
		t.Error("el job agotado debe tener failedAt")
	}
	for _, f := range []string{"nextAttemptAt", "leaseUntil"} {
		if _, err := raw.LookupErr(f); err == nil {
			t.Errorf("%s debe quitarse del job fallido", f)
		}
	}
	if s, _ := raw.Lookup("lastError").StringValueOK(); len(s) != maxReasonLen {
		t.Errorf("lastError = %d caracteres, want %d", len(s), maxReasonLen)
	}
	if n := nullPaths(t, raw, ""); len(n) != 0 {
		t.Errorf("job fallido con null en %v", n)
	}
	if far, _ := repo.Claim(ctx, next.Add(1000*time.Hour), 5); len(far) != 0 {
		t.Error("un job fallido no se reclama")
	}
	if n, _ := repo.Open(ctx, "u1"); n != 0 {
		t.Errorf("Open cuenta el fallido: %d", n)
	}
	// Un job inexistente o un id inválido.
	if err := repo.Fail(ctx, bson.NewObjectID().Hex(), usecase.JobFailure{At: now, Next: now}); err != nil {
		t.Errorf("Fail de un job inexistente = %v", err)
	}
	if err := repo.Fail(ctx, "no-es-id", usecase.JobFailure{At: now}); err == nil {
		t.Error("Fail con id inválido debe fallar")
	}
}

func rawJob(t *testing.T, repo *DeletionJobs, id string) bson.Raw {
	t.Helper()
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		t.Fatal(err)
	}
	var raw bson.Raw
	if err := repo.col.FindOne(context.Background(), bson.D{{Key: "_id", Value: oid}}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

// Dos reclamos simultáneos (dos réplicas) no devuelven el mismo job.
func TestIntegrationDeletionJobsConcurrentClaims(t *testing.T) {
	ctx := context.Background()
	repo := NewDeletionJobs(newDB(t))
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	const total = 40
	var batch []usecase.Job
	for i := 0; i < total; i++ {
		batch = append(batch, imageJob("u1", "p"+string(rune('a'+i%26))+string(rune('a'+i/26)), time.Time{}))
	}
	if err := repo.Enqueue(ctx, batch, now); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var all []string
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := repo.Claim(ctx, now, total)
			if err != nil {
				errs <- err
				return
			}
			mu.Lock()
			for _, j := range got {
				all = append(all, j.ID)
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Claim: %v", err)
	}
	slices.Sort(all)
	if len(all) != total || len(slices.Compact(slices.Clone(all))) != total {
		t.Errorf("reclamados %d, únicos %d; want %d sin repetidos", len(all), len(slices.Compact(slices.Clone(all))), total)
	}
}

func TestIntegrationDeletionJobsEnqueueValidation(t *testing.T) {
	ctx := context.Background()
	repo := NewDeletionJobs(newDB(t))
	now := time.Now().UTC()
	for name, j := range map[string]usecase.Job{
		"tipo inválido":   {UserID: "u1", Kind: "otro"},
		"imagen sin id":   {UserID: "u1", Kind: domain.JobImage, Image: &usecase.ImageRef{}},
		"imagen nil":      {UserID: "u1", Kind: domain.JobImage},
		"aviso sin nadie": {UserID: "u1", Kind: domain.JobNotice, Notice: &usecase.Notice{}},
		"aviso nil":       {UserID: "u1", Kind: domain.JobNotice},
	} {
		if err := repo.Enqueue(ctx, []usecase.Job{j}, now); err == nil {
			t.Errorf("%s: debe rechazarse", name)
		}
	}
}

// El job entra en la misma transacción que la baja: si la transacción se
// aborta, no queda ningún job (sin fotos huérfanas).
func TestIntegrationDeletionJobsEnqueueIsTransactional(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	repo := NewDeletionJobs(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	boom := errors.New("falla simulada")
	tx := adaptermongo.NewTxRunner(db).Do
	err := tx(ctx, func(c context.Context) error {
		if err := repo.Enqueue(c, []usecase.Job{imageJob("u1", "p1", time.Time{})}, now); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n, _ := repo.col.CountDocuments(ctx, bson.D{}); n != 0 {
		t.Errorf("el job debe revertirse con la transacción, hay %d", n)
	}
	if err := tx(ctx, func(c context.Context) error {
		return repo.Enqueue(c, []usecase.Job{imageJob("u1", "p1", time.Time{})}, now)
	}); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.col.CountDocuments(ctx, bson.D{}); n != 1 {
		t.Errorf("tras confirmar hay %d jobs, want 1", n)
	}
}
