package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/cloudinary"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
)

// check es la verificación de un servicio. skip explica por qué se omite.
type check struct {
	name string
	skip string
	run  func(ctx context.Context) error
}

// runChecks ejecuta cada verificación e imprime OK, FALLA u OMITIDO. Devuelve
// true si ninguna falló. Los errores de los clientes no incluyen secretos.
func runChecks(ctx context.Context, w io.Writer, checks []check) bool {
	ok := true
	for _, c := range checks {
		switch {
		case c.skip != "":
			_, _ = fmt.Fprintf(w, "OMITIDO  %-10s %s\n", c.name, c.skip)
		default:
			if err := c.run(ctx); err != nil {
				ok = false
				_, _ = fmt.Fprintf(w, "FALLA    %-10s %v\n", c.name, err)
			} else {
				_, _ = fmt.Fprintf(w, "OK       %s\n", c.name)
			}
		}
	}
	return ok
}

// serviceChecks arma las verificaciones reales a partir de la configuración.
func serviceChecks(cfg config.Config) []check {
	checks := []check{
		{name: "MongoDB", run: func(ctx context.Context) error {
			c, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoConnectTimeout)
			if err != nil {
				return err
			}
			defer func() { _ = c.Disconnect(context.Background()) }()
			return nil
		}},
	}

	redisCheck := check{name: "Redis"}
	if !cfg.Redis.Enabled() {
		redisCheck.skip = "REDIS_URL no está definida (solo es obligatoria en staging y production)"
	} else {
		redisCheck.run = func(ctx context.Context) error {
			c, err := redis.NewClient(cfg.Redis.URL.Reveal(), cfg.AppEnv)
			if err != nil {
				return err
			}
			defer func() { _ = c.Close() }()
			return c.Ping(ctx)
		}
	}
	checks = append(checks, redisCheck)

	cloudCheck := check{name: "Cloudinary"}
	if cfg.Cloudinary.CloudName == "" {
		cloudCheck.skip = "sin credenciales (APP_ENV=test)"
	} else {
		cloudCheck.run = func(ctx context.Context) error {
			return cloudinary.Ping(ctx, cfg.Cloudinary)
		}
	}
	return append(checks, cloudCheck)
}

// checkTimeout limita la duración total de check-services.
const checkTimeout = 30 * time.Second
