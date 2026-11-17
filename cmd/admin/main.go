// Command admin reúne utilidades de administración del backend (plan P-S3):
// subcomandos con os.Args, sin bibliotecas.
//
//	go run ./cmd/admin check-services
//	printf '%s\n%s\n' "$CORREO" "$CONTRASENA" | go run ./cmd/admin create-admin
//	go run ./cmd/admin indexes
//	SEED_CREDENTIALS_FILE=/ruta/fuera/del/repo go run ./cmd/admin seed
//	go run ./cmd/admin smoke --base-url https://host:8443
//	go run ./cmd/admin eval-recommendations
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo/catalog"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
)

const usage = `uso: admin <subcomando>

subcomandos:
  check-services   verifica MongoDB, Redis y Cloudinary (OK/FALLA por servicio)
  create-admin     crea una cuenta admin; lee correo y contraseña de la entrada
                   estándar (una línea cada uno), nunca de argumentos
  indexes          compara los índices declarados con los de MongoDB, solo
                   lectura (rol read); sale con 0 si coinciden, 1 si hay
                   discrepancias y 2 ante un error
  seed [--scale N] crea los datos de demostración de forma idempotente; las
                   contraseñas salen del archivo de SEED_CREDENTIALS_FILE;
                   nunca con APP_ENV=production; --scale solo en development
                   y test
  upload-test      sube una foto con firma real del entorno, la verifica y la
                   borra (sin escribir en MongoDB; se permite en producción)
  smoke --base-url URL [--connect-to host:puerto]
                   prueba de humo que escribe: cuenta desechable, foto,
                   objeto y baja, limpiando todo; se niega en producción
  eval-recommendations [--source synthetic|db] [--k N]
                   evalúa el recomendador (Hit@K, Recall@K y MRR) contra la
                   popularidad; por defecto con datos sintéticos de semilla
                   fija, sin base; --source db nunca en producción
  recompute-recommendations
                   recalcula el modelo de recomendaciones ahora
  seed-interactions [--purge] [--users N]
                   crea (o borra con --purge) interacciones sintéticas de
                   demostración; nunca con APP_ENV=production
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, config.Load, serviceChecks, os.Stdin, mongoAdminDeps, mongoIndexes))
}

// run separa la lógica de os.Exit para poder probarla. Devuelve el código de salida.
//
// Los subcomandos cierran sus conexiones con defer dentro de run, así que
// terminan antes de que main llame a os.Exit.
func run(args []string, stdout, stderr io.Writer, load func() (config.Config, error), checksFor func(config.Config) []check, stdin io.Reader, admins adminFactory, indexes indexesFactory) int {
	// Solo seed, smoke, eval-recommendations y seed-interactions aceptan
	// opciones; los demás, ningún argumento extra.
	withOptions := map[string]bool{"seed": true, "smoke": true, "eval-recommendations": true, "seed-interactions": true}
	if len(args) == 0 || (len(args) > 1 && !withOptions[args[0]]) {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "seed":
		cfg, err := load()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		return runSeed(args[1:], stdout, stderr, cfg, os.Getenv, mongoSeedDeps)
	case "smoke":
		cfg, err := load()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		return runSmoke(args[1:], stdout, stderr, cfg, newSmokeDeps)
	case "eval-recommendations":
		return runRecoEval(args[1:], stdout, stderr, load, mongoRecoDeps)
	case "recompute-recommendations":
		return runRecoRecompute(stdout, stderr, load, mongoRecoDeps)
	case "seed-interactions":
		return runRecoSeed(args[1:], stdout, stderr, load, mongoRecoDeps)
	case "upload-test":
		cfg, err := load()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		return runUploadTest(stdout, cfg, newUploadTestDeps)
	case "check-services":
		cfg, err := load()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()
		if !runChecks(ctx, stdout, checksFor(cfg)) {
			return 1
		}
		return 0
	case "create-admin":
		cfg, err := load()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		ctx, cancel := context.WithTimeout(context.Background(), createAdminTimeout)
		defer cancel()
		deps, err := admins(ctx, cfg)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		defer deps.close()
		return createAdmin(ctx, stdin, stdout, stderr, deps, time.Now())
	case "indexes":
		cfg, err := load()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			return indexesError
		}
		return runIndexes(stdout, stderr, cfg, indexes, catalog.All())
	default:
		_, _ = fmt.Fprintf(stderr, "subcomando desconocido: %q\n\n%s", args[0], usage)
		return 2
	}
}
