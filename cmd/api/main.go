// Command api inicia el servidor HTTP de VAIVÉN. Solo cablea dependencias; la
// lógica de negocio vive en internal/ (spec 002, plan §4.6; Art. III).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/handler"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	authhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/http"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	authusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images"
	imageshttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/logger"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/usecase/ratelimit"
)

const (
	serviceName = "vaiven-backend"
	devVersion  = "dev"

	// Límite global por IP (Art. XIV.2). Provisional: las specs funcionales
	// fijarán límites por política (POL-11, intentos de inicio de sesión).
	rateLimitPerWindow = 120
	rateLimitWindow    = time.Minute
)

// version la inyecta el build con -ldflags "-X main.version=..." (make build).
// Es la única variable global: el enlazador no puede asignar una constante.
var version = devVersion

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run carga la configuración, conecta, sirve y, al recibir SIGINT o SIGTERM,
// cierra el servidor y MongoDB en orden. Devuelve nil si el cierre fue limpio.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("no se pudo iniciar: %w", err)
	}

	log, err := logger.New(cfg.LogLevel.String(), cfg.AppEnv)
	if err != nil {
		return fmt.Errorf("no se pudo iniciar el logger: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mc, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoConnectTimeout)
	if err != nil {
		return fmt.Errorf("no se pudo conectar a MongoDB: %w", err)
	}
	// Corre después del Shutdown del servidor (o ante un error temprano).
	defer func() {
		dctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := mc.Disconnect(dctx); err != nil {
			log.Error("error al cerrar MongoDB", "error", err)
		}
	}()

	db := mc.Database(cfg.MongoDB)
	// Validación $jsonSchema (constitución §6.6) antes de los índices: crear
	// la colección con el validador solo requiere readWrite; aplicarlo a una
	// existente (collMod) requiere dbAdmin y, sin permiso, se avisa y se sigue.
	schemas, err := authmongo.EnsureSchemas(ctx, db)
	if err != nil {
		return fmt.Errorf("no se pudo aplicar la validación de MongoDB: %w", err)
	}
	log.Info("validación $jsonSchema", "creadas", schemas.Created, "actualizadas", schemas.Updated, "sin_cambios", schemas.Unchanged)
	for _, s := range schemas.Skipped {
		log.Warn("validación $jsonSchema no aplicada; aplícala con un usuario dbAdmin (docs/servicios.md)", "detalle", s)
	}
	userRepo := authmongo.NewUserRepo(db)
	refreshRepo := authmongo.NewRefreshRepo(db)
	if err := adaptermongo.EnsureAll(ctx, userRepo, refreshRepo); err != nil {
		return fmt.Errorf("no se pudieron crear los índices: %w", err)
	}

	// Redis es obligatorio en staging y production (lo exige config); en local
	// es opcional. Sin Redis el límite es local y /readyz informa "degradado".
	var redisPinger handler.Pinger
	var limiter ratelimit.Limiter = redis.NewMemoryLimiter()
	if cfg.Redis.Enabled() {
		rc, err := redis.NewClient(cfg.Redis.URL.Reveal(), cfg.AppEnv)
		if err != nil {
			return fmt.Errorf("no se pudo configurar Redis: %w", err)
		}
		defer func() { _ = rc.Close() }()
		if err := rc.Ping(ctx); err != nil {
			// Redis nunca tumba la API (Art. XIV.2): se arranca degradado.
			log.Warn("Redis no responde al arrancar; se sigue en modo degradado", "error", err)
		}
		redisPinger = rc
		limiter = redis.NewLimiter(rc)
	}

	authModule, err := newAuthModule(cfg, log, userRepo, refreshRepo, limiter)
	if err != nil {
		return err
	}

	imagesModule, err := newImagesModule(cfg, log, authModule, limiter)
	if err != nil {
		return err
	}

	mods := modules(moduleDeps{
		Log:        log,
		Notifier:   notify.Noop{},
		Moderation: moderation.AllowAll{},
		Auth:       authModule,
		Images:     imagesModule,
		Users:      userRepo,
	})
	for _, m := range mods {
		log.Info("módulo registrado", "module", m.Name())
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: apihttp.NewRouter(apihttp.Deps{
			Log:             log,
			AllowedOrigins:  cfg.CORSAllowedOrigins,
			Pinger:          mc,
			Redis:           redisPinger,
			Limiter:         limiter,
			LimiterFallback: redis.NewMemoryLimiter(),
			RateLimit:       middleware.RateLimitConfig{Limit: rateLimitPerWindow, Window: rateLimitWindow},
			Modules:         mods,
			Meta: handler.MetaInfo{
				Name:        serviceName,
				Version:     resolveVersion(version, cfg.AppVersion),
				Environment: cfg.AppEnv,
			},
		}),
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
	}

	// HTTPS forzoso en staging y production (Art. XIV.4): config exige el
	// certificado allí, así que solo se sirve HTTP en desarrollo y pruebas.
	serve := srv.ListenAndServe
	scheme := "http"
	if cfg.TLS.Enabled() {
		if err := apihttp.ConfigureTLS(srv, cfg.TLS.CertFile, cfg.TLS.KeyFile); err != nil {
			return fmt.Errorf("no se pudo configurar HTTPS: %w", err)
		}
		serve = func() error { return srv.ListenAndServeTLS("", "") }
		scheme = "https"
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- serve() }()
	log.Info("servidor iniciado", "addr", cfg.HTTPAddr, "scheme", scheme, "env", cfg.AppEnv, "version", resolveVersion(version, cfg.AppVersion))

	select {
	case err := <-serveErr:
		return fmt.Errorf("el servidor se detuvo: %w", err)
	case <-ctx.Done():
	}

	log.Info("cerrando el servidor")
	sctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("cierre del servidor: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("el servidor se detuvo: %w", err)
	}
	log.Info("servidor detenido")
	return nil
}

// newAuthModule arma el módulo de la spec 003: argon2id, JWT, casos de uso y
// rutas, con el límite de register, login y refresh sobre el limitador común.
func newAuthModule(cfg config.Config, log *slog.Logger, users *authmongo.UserRepo, refresh *authmongo.RefreshRepo, limiter ratelimit.Limiter) (*authhttp.Module, error) {
	jwt, err := security.NewJWT(cfg.Auth.AccessSecret.Reveal(), cfg.Auth.AccessTTL)
	if err != nil {
		return nil, fmt.Errorf("no se pudo configurar JWT: %w", err)
	}
	// argon2id con presupuesto de memoria y espera acotada; el señuelo del
	// login se calcula aquí, al arrancar (P-01, A-07).
	hasher, err := security.NewHasher(security.DefaultMemoryBudgetKiB, security.DefaultMaxWait)
	if err != nil {
		return nil, fmt.Errorf("no se pudo preparar argon2id: %w", err)
	}
	svc, err := authusecase.NewService(authusecase.Deps{
		Users:       users,
		Refresh:     refresh,
		Hasher:      hasher,
		Access:      jwt,
		NewRefresh:  security.NewRefreshToken,
		HashRefresh: security.HashToken,
		RefreshTTL:  cfg.Auth.RefreshTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el servicio de auth: %w", err)
	}
	return authhttp.NewModule(authhttp.ModuleDeps{
		Service:           svc,
		Tokens:            jwt,
		Log:               log,
		Limiter:           limiter,
		LimiterFallback:   redis.NewMemoryLimiter(),
		LimitPerMinute:    cfg.Auth.RateLimitPerMin,
		AccountLoginLimit: cfg.Auth.LoginAttemptsPerAccount,
	})
}

// newImagesModule arma el módulo de la spec 004: Cloudinary sin SDK, la
// fachada images.Service y POST /uploads/signature protegido por auth.
func newImagesModule(cfg config.Config, log *slog.Logger, authModule *authhttp.Module, limiter ratelimit.Limiter) (*imageshttp.Module, error) {
	svc, err := images.New(images.Config{
		CloudName:  cfg.Cloudinary.CloudName,
		APIKey:     cfg.Cloudinary.APIKey,
		APISecret:  cfg.Cloudinary.APISecret.Reveal(),
		BaseFolder: cfg.Cloudinary.BaseFolder,
		MaxBytes:   cfg.Images.MaxBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("no se pudo configurar Cloudinary: %w", err)
	}
	return imageshttp.NewModule(imageshttp.ModuleDeps{
		Service:           svc,
		Authenticate:      authModule.Authenticate(),
		Log:               log,
		Limiter:           limiter,
		LimiterFallback:   redis.NewMemoryLimiter(),
		SignaturesPerHour: cfg.Images.SignaturesPerHour,
	})
}

// resolveVersion prefiere la versión inyectada por -ldflags; si el binario no
// la trae (sigue en "dev"), usa APP_VERSION de la configuración.
func resolveVersion(injected, fromEnv string) string {
	if injected != devVersion {
		return injected
	}
	return fromEnv
}
