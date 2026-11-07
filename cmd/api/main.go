// Command api inicia el servidor HTTP de VAIVÉN. Solo cablea dependencias; la
// lógica de negocio vive en internal/ (spec 002, plan §4.6; Art. III).
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/handler"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/logger"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
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

	// Sin repositorios en esta spec: la lista de ensurers está vacía.
	if err := adaptermongo.EnsureAll(ctx); err != nil {
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

	mods := modules(moduleDeps{
		Log:        log,
		Notifier:   notify.Noop{},
		Moderation: moderation.AllowAll{},
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

// resolveVersion prefiere la versión inyectada por -ldflags; si el binario no
// la trae (sigue en "dev"), usa APP_VERSION de la configuración.
func resolveVersion(injected, fromEnv string) string {
	if injected != devVersion {
		return injected
	}
	return fromEnv
}
