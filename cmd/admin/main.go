// Command admin reúne utilidades de administración del backend (plan P-S3):
// subcomandos con os.Args, sin bibliotecas.
//
//	go run ./cmd/admin check-services
//	printf '%s\n%s\n' "$CORREO" "$CONTRASENA" | go run ./cmd/admin create-admin
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
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, config.Load, serviceChecks, os.Stdin, mongoAdminDeps, mongoIndexes))
}

// run separa la lógica de os.Exit para poder probarla. Devuelve el código de salida.
//
// Los subcomandos cierran sus conexiones con defer dentro de run, así que
// terminan antes de que main llame a os.Exit.
func run(args []string, stdout, stderr io.Writer, load func() (config.Config, error), checksFor func(config.Config) []check, stdin io.Reader, admins adminFactory, indexes indexesFactory) int {
	if len(args) != 1 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
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
