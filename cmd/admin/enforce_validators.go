package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	drv "go.mongodb.org/mongo-driver/v2/mongo"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// enforceValidatorsTimeout es el plazo total de `admin enforce-validators`.
const enforceValidatorsTimeout = 30 * time.Minute

// Códigos de salida de `admin enforce-validators` (spec 021, T030).
const (
	enforceOK      = 0 // el plan se mostró (dry-run) o se aplicó sin pendientes
	enforceRefused = 1 // hay null (no se endurece) o falta permiso de collMod en alguna colección
	enforceFailure = 2 // uso, configuración, conexión, plazo o escritura
)

// validatorTool lee (como verify-no-nulls) el estado de los validadores y los
// aplica.
type validatorTool interface {
	nullSource
	// State dice si la colección ya tiene el validador estricto, el tolerante,
	// otro o ninguno.
	State(ctx context.Context, spec adaptermongo.ValidatorSpec) (adaptermongo.Strictness, error)
	// Apply aplica el validador estricto (strict) o el tolerante a las colecciones.
	Apply(ctx context.Context, specs []adaptermongo.ValidatorSpec, strict bool) (adaptermongo.ValidatorsResult, error)
}

// enforceFactory abre la herramienta; closeFn cierra la conexión.
type enforceFactory func(ctx context.Context, cfg config.Config) (t validatorTool, closeFn func(), err error)

// allValidatorSpecs son todas las colecciones con validador, en su forma
// tolerante: las de auth e items y las de campos opcionales del resto.
func allValidatorSpecs() []adaptermongo.ValidatorSpec {
	specs := authmongo.ValidatorSpecs()
	specs = append(specs, itemsmongo.ValidatorSpec())
	return append(specs, adaptermongo.OptionalFieldSpecs()...)
}

type dbValidatorTool struct {
	dbNullSource
	db *drv.Database
}

// mongoEnforce es la enforceFactory real. El dry-run solo lee (rol read); aplicar
// exige collMod (rol dbAdmin).
func mongoEnforce(ctx context.Context, cfg config.Config) (validatorTool, func(), error) {
	mc, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoConnectTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("conectar a MongoDB: %w", err)
	}
	db := mc.Database(cfg.MongoDB)
	return dbValidatorTool{dbNullSource: dbNullSource{db: db}, db: db}, func() { _ = mc.Disconnect(context.Background()) }, nil
}

func (t dbValidatorTool) State(ctx context.Context, spec adaptermongo.ValidatorSpec) (adaptermongo.Strictness, error) {
	return adaptermongo.ValidatorState(ctx, t.db, spec)
}

func (t dbValidatorTool) Apply(ctx context.Context, specs []adaptermongo.ValidatorSpec, strict bool) (adaptermongo.ValidatorsResult, error) {
	return adaptermongo.EnforceValidators(ctx, t.db, specs, strict)
}

// runEnforceValidators es `admin enforce-validators [--apply] [--tolerant]
// [--yes-production]` (spec 021, T030): el interruptor EXPLÍCITO que endurece los
// validadores (sin null, nivel moderate, acción error) en todas las colecciones.
//
// La API nunca endurece sola al desplegarse: su arranque aplica los esquemas
// tolerantes y respeta uno ya endurecido. Este comando es el único camino a
// estricto y se usa solo DESPUÉS de migrar y verificar (despliegue A → respaldo →
// migrate-remove-nulls → verify-no-nulls en 0 → este comando). Por omisión es
// un dry-run que muestra el estado de cada colección. Con --apply, vuelve a
// verificar que no haya null y se niega (salida 1, sin cambiar nada) si los hay.
// --tolerant (con --apply) es el camino de vuelta al modo tolerante (acción
// warn en el resto de colecciones), sin verificación. Con APP_ENV=production
// --apply exige además --yes-production. Idempotente. No imprime valores ni ids.
func runEnforceValidators(args []string, stdout, stderr io.Writer, load func() (config.Config, error), open enforceFactory) int {
	fs := flag.NewFlagSet("enforce-validators", flag.ContinueOnError)
	fs.SetOutput(stderr)
	apply := fs.Bool("apply", false, "aplicar el cambio (por omisión solo muestra el plan)")
	tolerant := fs.Bool("tolerant", false, "volver al modo tolerante en lugar de endurecer")
	yesProd := fs.Bool("yes-production", false, "confirma --apply con APP_ENV=production")
	if err := fs.Parse(args); err != nil {
		return enforceFailure
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintln(stderr, "enforce-validators: argumentos de más")
		return enforceFailure
	}
	cfg, err := load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return enforceFailure
	}
	if *apply && cfg.AppEnv == config.EnvProduction && !*yesProd {
		_, _ = fmt.Fprintln(stderr, "enforce-validators: con APP_ENV=production --apply exige también --yes-production")
		return enforceFailure
	}
	strict := !*tolerant

	ctx, cancel := context.WithTimeout(context.Background(), enforceValidatorsTimeout)
	defer cancel()
	tool, closeFn, err := open(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), cfg.MongoURI))
		return enforceFailure
	}
	defer closeFn()

	specs := allValidatorSpecs()
	pending := 0
	for _, sp := range specs {
		st, err := tool.State(ctx, sp)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), cfg.MongoURI))
			return enforceFailure
		}
		_, _ = fmt.Fprintf(stdout, "%-20s %s\n", sp.Collection, st)
		if (strict && st != adaptermongo.StrictnessStrict) || (!strict && st == adaptermongo.StrictnessStrict) {
			pending++
		}
	}

	if strict {
		// No se endurece sobre datos sin migrar: se verifica de nuevo aquí.
		var out bytes.Buffer
		switch verifyNulls(ctx, &out, stderr, cfg.MongoURI, tool) {
		case verifyNullsClean:
			_, _ = fmt.Fprintln(stdout, "\nverificación: cero null en todas las colecciones")
		case verifyNullsFound:
			_, _ = stdout.Write(out.Bytes())
			_, _ = fmt.Fprintln(stderr, "\nenforce-validators: hay null; no se endurece. Respalda, ejecuta migrate-remove-nulls y verify-no-nulls hasta obtener cero (docs/respaldos.md §6)")
			return enforceRefused
		default:
			return enforceFailure
		}
	}

	target := "estricto"
	if !strict {
		target = "tolerante"
	}
	if !*apply {
		_, _ = fmt.Fprintf(stdout, "dry-run: %d de %d colecciones cambiarían a %s; usa --apply para aplicarlo\n", pending, len(specs), target)
		return enforceOK
	}
	res, err := tool.Apply(ctx, specs, strict)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), cfg.MongoURI))
		return enforceFailure
	}
	_, _ = fmt.Fprintf(stdout, "\nvalidador %s: %d creadas, %d actualizadas, %d sin cambios, %d omitidas\n",
		target, len(res.Created), len(res.Updated), len(res.Unchanged), len(res.Skipped))
	if len(res.Skipped) > 0 {
		for _, c := range res.Skipped {
			_, _ = fmt.Fprintf(stdout, "  omitida (sin permiso para collMod, requiere dbAdmin): %s\n", c)
		}
		_, _ = fmt.Fprintln(stderr, "enforce-validators: faltó permiso en algunas colecciones; aplícalo con un usuario dbAdmin (docs/servicios.md §9)")
		return enforceRefused
	}
	return enforceOK
}
