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

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/handler"
	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/logger"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

const (
	serviceName = "vaiven-backend"
	devVersion  = "dev"
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

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: apihttp.NewRouter(apihttp.Deps{
			Log:            log,
			AllowedOrigins: cfg.CORSAllowedOrigins,
			Pinger:         mc,
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

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	log.Info("servidor iniciado", "addr", cfg.HTTPAddr, "env", cfg.AppEnv, "version", resolveVersion(version, cfg.AppVersion))

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
