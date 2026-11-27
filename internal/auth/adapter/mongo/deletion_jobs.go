package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
)

// DeletionJobsCollection es la bandeja de salida (outbox) de la baja (spec 021,
// plan §4): los pasos que ocurren DESPUÉS del commit (borrar fotos, avisar a
// contrapartes). El job se inserta en la misma transacción que la baja, así no
// hay fotos huérfanas aunque el proceso caiga justo después del commit. Al
// terminar un job su documento se borra.
const DeletionJobsCollection = "deletion_jobs"

// DefaultJobLease es el arrendamiento de un job reclamado: pasado ese tiempo
// sin Done ni Fail, otro worker (otra réplica o el mismo tras reiniciar) puede
// reclamarlo.
const DefaultJobLease = 5 * time.Minute

// maxReasonLen acota el motivo guardado (sin datos personales).
const maxReasonLen = 200

// jobDoc es el documento BSON de deletion_jobs. Ningún campo se guarda como
// null: los opcionales se omiten. nextAttemptAt se quita cuando el job falla
// del todo, así el índice parcial solo contiene jobs por hacer.
type jobDoc struct {
	ID            bson.ObjectID `bson:"_id,omitempty"`
	UserID        string        `bson:"userId"`
	Kind          string        `bson:"kind"`
	Payload       jobPayloadDoc `bson:"payload"`
	Attempts      int           `bson:"attempts"`
	NextAttemptAt *time.Time    `bson:"nextAttemptAt,omitempty"`
	LeaseUntil    *time.Time    `bson:"leaseUntil,omitempty"`
	FailedAt      *time.Time    `bson:"failedAt,omitempty"`
	LastError     string        `bson:"lastError,omitempty"`
	// Retention solo existe en los jobs programados por la retención de 12
	// meses (D-4): no cuentan como pasos pendientes de la baja.
	Retention bool      `bson:"retention,omitempty"`
	CreatedAt time.Time `bson:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"`
}

// jobPayloadDoc reúne los datos de un job de imagen (publicId, delivery) o de
// aviso (userId destino, type, title, body, data). Solo se guardan los que
// tienen valor.
type jobPayloadDoc struct {
	PublicID string            `bson:"publicId,omitempty"`
	Delivery string            `bson:"delivery,omitempty"`
	UserID   string            `bson:"userId,omitempty"`
	Type     string            `bson:"type,omitempty"`
	Title    string            `bson:"title,omitempty"`
	Body     string            `bson:"body,omitempty"`
	Data     map[string]string `bson:"data,omitempty"`
}

func (d jobDoc) toJob() usecase.Job {
	j := usecase.Job{ID: d.ID.Hex(), UserID: d.UserID, Kind: domain.JobKind(d.Kind), Attempts: d.Attempts}
	if d.NextAttemptAt != nil {
		j.NextAttemptAt = d.NextAttemptAt.UTC()
	}
	switch j.Kind {
	case domain.JobImage:
		j.Image = &usecase.ImageRef{PublicID: d.Payload.PublicID, Delivery: d.Payload.Delivery}
	case domain.JobNotice:
		j.Notice = &usecase.Notice{
			UserID: d.Payload.UserID, Type: d.Payload.Type, Title: d.Payload.Title, Body: d.Payload.Body,
			Data: d.Payload.Data,
		}
	}
	return j
}

// DeletionJobs implementa usecase.Jobs sobre MongoDB.
type DeletionJobs struct {
	col   *drv.Collection
	lease time.Duration
}

// NewDeletionJobs crea el repositorio con el arrendamiento por omisión.
func NewDeletionJobs(db *drv.Database) *DeletionJobs {
	return &DeletionJobs{col: db.Collection(DeletionJobsCollection), lease: DefaultJobLease}
}

// DeletionJobIndexes declara los índices de DeletionJobsCollection (fuente
// única para EnsureIndexes y el catálogo de `admin indexes`): el de reclamo,
// parcial sobre los jobs por hacer (con nextAttemptAt), y el de la cuenta para
// contar los jobs pendientes de una baja.
func DeletionJobIndexes() []adaptermongo.IndexSpec {
	return adaptermongo.SpecsFor(DeletionJobsCollection, []drv.IndexModel{
		{
			Keys: bson.D{{Key: "nextAttemptAt", Value: 1}},
			Options: options.Index().SetName("nextAttemptAt_partial").
				SetPartialFilterExpression(bson.D{{Key: "nextAttemptAt", Value: bson.D{{Key: "$exists", Value: true}}}}),
		},
		{Keys: bson.D{{Key: "userId", Value: 1}}, Options: options.Index().SetName("userId")},
	}...)
}

// Indexes cumple adaptermongo.IndexDeclarer.
func (r *DeletionJobs) Indexes() []adaptermongo.IndexSpec { return DeletionJobIndexes() }

// EnsureIndexes crea los índices de la bandeja. Es idempotente.
func (r *DeletionJobs) EnsureIndexes(ctx context.Context) error {
	if _, err := r.col.Indexes().CreateMany(ctx, adaptermongo.Models(r.Indexes())); err != nil {
		return fmt.Errorf("deletion_jobs: crear índices: %w", err)
	}
	return nil
}

// Enqueue guarda los jobs. Se llama con el ctx de la transacción de la baja.
// Un job sin NextAttemptAt corre de inmediato.
func (r *DeletionJobs) Enqueue(ctx context.Context, jobs []usecase.Job, now time.Time) error {
	if len(jobs) == 0 {
		return nil
	}
	now = now.UTC()
	docs := make([]any, 0, len(jobs))
	for _, j := range jobs {
		if !j.Kind.Valid() {
			return fmt.Errorf("deletion_jobs: tipo de job inválido %q", j.Kind)
		}
		next := j.NextAttemptAt.UTC()
		if j.NextAttemptAt.IsZero() {
			next = now
		}
		d := jobDoc{
			ID: bson.NewObjectID(), UserID: j.UserID, Kind: string(j.Kind),
			NextAttemptAt: &next, Retention: j.Retention, CreatedAt: now, UpdatedAt: now,
		}
		switch j.Kind {
		case domain.JobImage:
			if j.Image == nil || j.Image.PublicID == "" {
				return errors.New("deletion_jobs: job de imagen sin publicId")
			}
			d.Payload = jobPayloadDoc{PublicID: j.Image.PublicID, Delivery: j.Image.Delivery}
		case domain.JobNotice:
			if j.Notice == nil || j.Notice.UserID == "" {
				return errors.New("deletion_jobs: job de aviso sin destinatario")
			}
			n := j.Notice
			d.Payload = jobPayloadDoc{UserID: n.UserID, Type: n.Type, Title: n.Title, Body: n.Body, Data: n.Data}
		}
		docs = append(docs, d)
	}
	if _, err := r.col.InsertMany(ctx, docs); err != nil {
		return fmt.Errorf("deletion_jobs: encolar: %w", err)
	}
	return nil
}

// Claim reserva hasta limit jobs vencidos. Cada reclamo es un findOneAndUpdate
// atómico que fija el arrendamiento y suma un intento, así dos reclamos
// simultáneos (dos réplicas) nunca reciben el mismo job; un job con el
// arrendamiento vencido (el proceso cayó) se vuelve a reclamar.
func (r *DeletionJobs) Claim(ctx context.Context, now time.Time, limit int) ([]usecase.Job, error) {
	now = now.UTC()
	filter := bson.D{
		{Key: "failedAt", Value: bson.D{{Key: "$exists", Value: false}}},
		{Key: "nextAttemptAt", Value: bson.D{{Key: "$lte", Value: now}}},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "leaseUntil", Value: bson.D{{Key: "$exists", Value: false}}}},
			bson.D{{Key: "leaseUntil", Value: bson.D{{Key: "$lte", Value: now}}}},
		}},
	}
	update := bson.D{
		{Key: "$set", Value: bson.D{{Key: "leaseUntil", Value: now.Add(r.lease)}, {Key: "updatedAt", Value: now}}},
		{Key: "$inc", Value: bson.D{{Key: "attempts", Value: 1}}},
	}
	opts := options.FindOneAndUpdate().SetSort(bson.D{{Key: "nextAttemptAt", Value: 1}}).SetReturnDocument(options.After)
	var out []usecase.Job
	for len(out) < limit {
		var d jobDoc
		err := r.col.FindOneAndUpdate(ctx, filter, update, opts).Decode(&d)
		if errors.Is(err, drv.ErrNoDocuments) {
			break
		}
		if err != nil {
			return out, fmt.Errorf("deletion_jobs: reclamar: %w", err)
		}
		out = append(out, d.toJob())
	}
	return out, nil
}

// Done borra el job terminado. Un job que ya no existe no es error.
func (r *DeletionJobs) Done(ctx context.Context, id string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("deletion_jobs: id inválido: %w", err)
	}
	if _, err := r.col.DeleteOne(ctx, bson.D{{Key: "_id", Value: oid}}); err != nil {
		return fmt.Errorf("deletion_jobs: terminar: %w", err)
	}
	return nil
}

// Fail libera el arrendamiento y programa el reintento; si Exhausted, marca el
// job como fallido (failedAt) y le quita nextAttemptAt: deja de reclamarse y
// sale del índice, y queda en la colección para operación. Un job que ya no
// existe no es error.
func (r *DeletionJobs) Fail(ctx context.Context, id string, f usecase.JobFailure) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("deletion_jobs: id inválido: %w", err)
	}
	reason := f.Reason
	if len(reason) > maxReasonLen {
		reason = reason[:maxReasonLen]
	}
	set := bson.D{{Key: "updatedAt", Value: f.At.UTC()}}
	unset := bson.D{{Key: "leaseUntil", Value: ""}}
	if reason != "" {
		set = append(set, bson.E{Key: "lastError", Value: reason})
	}
	if f.Exhausted {
		set = append(set, bson.E{Key: "failedAt", Value: f.At.UTC()})
		unset = append(unset, bson.E{Key: "nextAttemptAt", Value: ""})
	} else {
		set = append(set, bson.E{Key: "nextAttemptAt", Value: f.Next.UTC()})
	}
	_, err = r.col.UpdateOne(ctx, bson.D{{Key: "_id", Value: oid}},
		bson.D{{Key: "$set", Value: set}, {Key: "$unset", Value: unset}})
	if err != nil {
		return fmt.Errorf("deletion_jobs: registrar fallo: %w", err)
	}
	return nil
}

// Open cuenta los pasos por hacer de la cuenta: no cuenta los fallidos ni los
// jobs de retención (esperan su fecha de purga, no mantienen deletionPending).
func (r *DeletionJobs) Open(ctx context.Context, userID string) (int64, error) {
	n, err := r.col.CountDocuments(ctx, bson.D{
		{Key: "userId", Value: userID},
		{Key: "failedAt", Value: bson.D{{Key: "$exists", Value: false}}},
		{Key: "retention", Value: bson.D{{Key: "$exists", Value: false}}},
	})
	if err != nil {
		return 0, fmt.Errorf("deletion_jobs: contar pendientes: %w", err)
	}
	return n, nil
}

// Comprobación en compilación del puerto que implementa DeletionJobs.
var _ usecase.Jobs = (*DeletionJobs)(nil)
