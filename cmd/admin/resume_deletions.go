package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/imagedestroyer"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/notices"
	authusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images"
	notificationsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notifications/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
)

// resumeDeletionsTimeout es el plazo total de `admin resume-deletions`.
const resumeDeletionsTimeout = 10 * time.Minute

// Códigos de salida de `admin resume-deletions` (spec 021, N-1).
const (
	resumeOK      = 0 // la pasada terminó sin jobs agotados
	resumeFailed  = 1 // hubo jobs que agotaron sus intentos en esta pasada
	resumeFailure = 2 // configuración, conexión, plazo o error de la pasada
)

// deletionResumer hace una pasada de reanudación de bajas (usecase.Resumer).
type deletionResumer interface {
	ResumeDeletions(ctx context.Context) (authusecase.PassReport, error)
}

// resumeFactory abre el Resumer; closeFn cierra las conexiones.
type resumeFactory func(ctx context.Context, cfg config.Config) (res deletionResumer, closeFn func(), warnings []string, err error)

// runResumeDeletions ejecuta UNA pasada del mismo caso de uso que el worker de
// la API: ejecuta los jobs vencidos de la bandeja de salida (avisos y, desde la
// fase 3, fotos), repite la revocación de acceso en Redis de las cuentas con
// deletionPending y les quita la marca cuando no queda nada. Es la reanudación
// manual de una baja interrumpida (N-1); es idempotente y se puede repetir.
// Imprime solo contadores: nunca ids, correos ni textos.
func runResumeDeletions(stdout, stderr io.Writer, load func() (config.Config, error), open resumeFactory) int {
	cfg, err := load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return resumeFailure
	}
	ctx, cancel := context.WithTimeout(context.Background(), resumeDeletionsTimeout)
	defer cancel()
	res, closeFn, warnings, err := open(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return resumeFailure
	}
	defer closeFn()
	for _, w := range warnings {
		_, _ = fmt.Fprintf(stderr, "aviso: %s\n", w)
	}
	rep, err := res.ResumeDeletions(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: la pasada falló: %v\n", err)
		return resumeFailure
	}
	_, _ = fmt.Fprintf(stdout, "jobs hechos: %d, reintentos programados: %d, jobs agotados: %d, cuentas revisadas: %d, marcas quitadas: %d\n",
		rep.JobsDone, rep.JobsRetried, rep.JobsFailed, rep.Accounts, rep.Cleared)
	if rep.JobsFailed > 0 {
		return resumeFailed
	}
	return resumeOK
}

// mongoResume es la resumeFactory real: MongoDB (cuentas y bandeja), Cloudinary
// (borrado de fotos, obligatorio), Redis si
// está configurado (revocación de acceso) y notify.Service sin push (el aviso
// queda guardado; el push solo lo envía el worker de la API).
func mongoResume(ctx context.Context, cfg config.Config) (deletionResumer, func(), []string, error) {
	mc, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoConnectTimeout)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("conectar a MongoDB: %w", err)
	}
	closers := []func(){func() { _ = mc.Disconnect(context.Background()) }}
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	db := mc.Database(cfg.MongoDB)
	users := authmongo.NewUserRepo(db)

	var warnings []string
	var revocations authusecase.Revocations
	if cfg.Redis.Enabled() {
		rc, err := redis.NewClient(cfg.Redis.URL.Reveal(), cfg.AppEnv)
		if err != nil {
			closeAll()
			return nil, nil, nil, fmt.Errorf("configurar Redis: %w", err)
		}
		closers = append(closers, func() { _ = rc.Close() })
		revocations = redis.NewRevocations(rc)
	} else {
		warnings = append(warnings, "Redis no está configurado: no se revoca el acceso de las cuentas")
	}

	notifier, err := notify.NewService(notify.ServiceDeps{
		Store: notificationsmongo.NewNotificationRepo(db), Users: users, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		closeAll()
		return nil, nil, nil, err
	}
	sender, err := notices.New(notifier)
	if err != nil {
		closeAll()
		return nil, nil, nil, err
	}
	// Cloudinary: sin él los jobs de fotos no se pueden ejecutar y quedarían
	// reintentando (nunca nil: un job «image» sin destructor fallaría siempre).
	destroyer, err := images.NewDestroyer(images.Config{
		CloudName: cfg.Cloudinary.CloudName, APIKey: cfg.Cloudinary.APIKey, APISecret: cfg.Cloudinary.APISecret.Reveal(),
	})
	if err != nil {
		closeAll()
		return nil, nil, nil, fmt.Errorf("configurar Cloudinary: %w", err)
	}
	imageJobs, err := imagedestroyer.New(destroyer)
	if err != nil {
		closeAll()
		return nil, nil, nil, err
	}
	resumer, err := authusecase.NewResumer(authusecase.ResumerDeps{
		Users: users, Pending: users, Jobs: authmongo.NewDeletionJobs(db), Revocations: revocations,
		AccessTTL: cfg.Auth.AccessTTL, Images: imageJobs, Notices: sender,
	})
	if err != nil {
		closeAll()
		return nil, nil, nil, err
	}
	return resumer, closeAll, warnings, nil
}
