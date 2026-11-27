package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// migrateNullsTimeout es el plazo total de `admin migrate-remove-nulls`.
const migrateNullsTimeout = 2 * time.Hour

// migrateBatchSize es el tamaño de cada lote de escrituras (RNF-05).
const migrateBatchSize = 1000

// Códigos de salida de `admin migrate-remove-nulls` (spec 021, RF-12).
const (
	migrateOK          = 0 // terminó (o el dry-run se mostró) sin rutas no soportadas
	migrateUnsupported = 1 // hay rutas que la herramienta reporta y no toca
	migrateFailure     = 2 // uso, configuración, conexión, plazo o escritura
)

// nullOp son los cambios de un documento: rutas concretas a quitar con $unset y
// arreglos de los que sacar los null con $pull. Cada ruta lleva su guarda
// {ruta: {$type: "null"}} en el filtro, así un valor real que otro proceso
// escriba entre la lectura y la escritura nunca se borra.
type nullOp struct {
	ID    bson.RawValue
	Unset []string
	Pull  []string
}

// nullMigrator lee (como verify-no-nulls) y además aplica un lote de cambios.
type nullMigrator interface {
	nullSource
	// Apply ejecuta las operaciones de un lote y devuelve cuántos documentos
	// modificó.
	Apply(ctx context.Context, collection string, ops []nullOp) (int64, error)
}

// migrateFactory abre el migrador; closeFn cierra la conexión.
type migrateFactory func(ctx context.Context, cfg config.Config) (m nullMigrator, closeFn func(), err error)

type dbNullMigrator struct {
	dbNullSource
}

// mongoMigrate es la migrateFactory real. Necesita un usuario con rol readWrite.
func mongoMigrate(ctx context.Context, cfg config.Config) (nullMigrator, func(), error) {
	mc, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoConnectTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("conectar a MongoDB: %w", err)
	}
	return dbNullMigrator{dbNullSource{db: mc.Database(cfg.MongoDB)}}, func() { _ = mc.Disconnect(context.Background()) }, nil
}

func (m dbNullMigrator) Apply(ctx context.Context, collection string, ops []nullOp) (int64, error) {
	models := make([]drv.WriteModel, 0, len(ops))
	for _, op := range ops {
		filter := bson.D{{Key: "_id", Value: op.ID}}
		for _, p := range append(slices.Clone(op.Unset), op.Pull...) {
			filter = append(filter, bson.E{Key: p, Value: bson.D{{Key: "$type", Value: "null"}}})
		}
		update := bson.D{}
		if len(op.Unset) > 0 {
			u := make(bson.D, 0, len(op.Unset))
			for _, p := range op.Unset {
				u = append(u, bson.E{Key: p, Value: ""})
			}
			update = append(update, bson.E{Key: "$unset", Value: u})
		}
		if len(op.Pull) > 0 {
			pl := make(bson.D, 0, len(op.Pull))
			for _, p := range op.Pull {
				pl = append(pl, bson.E{Key: p, Value: nil})
			}
			update = append(update, bson.E{Key: "$pull", Value: pl})
		}
		models = append(models, drv.NewUpdateOneModel().SetFilter(filter).SetUpdate(update))
	}
	res, err := m.db.Collection(collection).BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return 0, fmt.Errorf("escribir %s: %w", collection, err)
	}
	return res.ModifiedCount, nil
}

// planNulls recorre el documento y devuelve las operaciones concretas para
// quitar sus null y las rutas normalizadas no soportadas (más de un nivel de
// arreglo), que se reportan y no se tocan. Soportado: null en la raíz o en
// subdocumentos ($unset de la ruta), campo null dentro de un documento de un
// arreglo ($unset con el índice) y null suelto dentro de un arreglo de primer
// nivel ($pull).
func planNulls(doc bson.Raw) (unset, pull []string, unsupported map[string]int64, err error) {
	unsupported = map[string]int64{}
	pullSet := map[string]string{} // ruta concreta del arreglo -> ruta normalizada
	var walkDoc func(d bson.Raw, concrete, norm string, arrays int) error
	var walkVal func(v bson.RawValue, concrete, norm string, arrays int) error
	walkVal = func(v bson.RawValue, concrete, norm string, arrays int) error {
		switch v.Type {
		case bson.TypeNull:
			unset = append(unset, concrete)
		case bson.TypeEmbeddedDocument:
			return walkDoc(v.Document(), concrete, norm, arrays)
		case bson.TypeArray:
			vals, err := v.Array().Values()
			if err != nil {
				return fmt.Errorf("arreglo BSON inválido: %w", err)
			}
			for i, item := range vals {
				ic, in := concrete+"."+strconv.Itoa(i), norm+"[]"
				switch {
				case item.Type == bson.TypeNull && arrays == 0:
					pullSet[concrete] = norm // concrete no lleva índices: arreglo de primer nivel
				case item.Type == bson.TypeNull:
					unsupported[in]++
				default:
					if arrays >= 1 && containsNull(item) {
						countNulls(item, in, unsupported)
						continue
					}
					if err := walkVal(item, ic, in, arrays+1); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	walkDoc = func(d bson.Raw, concrete, norm string, arrays int) error {
		elems, err := d.Elements()
		if err != nil {
			return fmt.Errorf("documento BSON inválido: %w", err)
		}
		for _, el := range elems {
			c, n := el.Key(), el.Key()
			if concrete != "" {
				c, n = concrete+"."+c, norm+"."+n
			}
			if err := walkVal(el.Value(), c, n, arrays); err != nil {
				return err
			}
		}
		return nil
	}
	if err = walkDoc(doc, "", "", 0); err != nil {
		return nil, nil, nil, err
	}
	// Un $pull y un $unset sobre el mismo arreglo chocan en MongoDB: si el
	// arreglo mezcla null sueltos y documentos con campos null, se reporta como
	// no soportado y no se toca el $pull (los $unset sí se aplican).
	for p, norm := range pullSet {
		mixed := slices.ContainsFunc(unset, func(u string) bool { return strings.HasPrefix(u, p+".") })
		if mixed {
			unsupported[norm+"[]"]++
			continue
		}
		pull = append(pull, p)
	}
	sort.Strings(pull)
	return unset, pull, unsupported, nil
}

// containsNull indica si v (documento o arreglo) tiene algún null por debajo.
func containsNull(v bson.RawValue) bool {
	m := map[string]int64{}
	countNulls(v, "x", m)
	return len(m) > 0
}

// countNulls cuenta los null de v por ruta normalizada, partiendo de path.
func countNulls(v bson.RawValue, path string, out map[string]int64) {
	_ = walkValue(v, path, out)
}

// migrateOptions son las opciones de la línea de comandos.
type migrateOptions struct {
	apply, dryRun, yesProduction bool
	backupRef                    string
}

func parseMigrateArgs(args []string, stderr io.Writer) (migrateOptions, bool) {
	var o migrateOptions
	fs := flag.NewFlagSet("migrate-remove-nulls", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.apply, "apply", false, "")
	fs.BoolVar(&o.dryRun, "dry-run", false, "")
	fs.BoolVar(&o.yesProduction, "yes-production", false, "")
	fs.StringVar(&o.backupRef, "backup-ref", "", "")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		_, _ = fmt.Fprint(stderr, "uso: admin migrate-remove-nulls [--dry-run] [--apply --backup-ref <texto> [--yes-production]]\n")
		return o, false
	}
	return o, true
}

// runMigrateNulls es el subcomando completo (cierra la conexión antes de
// devolver el código).
func runMigrateNulls(args []string, stdout, stderr io.Writer, load func() (config.Config, error), open migrateFactory) int {
	o, ok := parseMigrateArgs(args, stderr)
	if !ok {
		return migrateFailure
	}
	if o.apply && o.dryRun {
		_, _ = fmt.Fprintln(stderr, "error: --apply y --dry-run se excluyen")
		return migrateFailure
	}
	ref := strings.TrimSpace(o.backupRef)
	if o.apply {
		if ref == "" {
			_, _ = fmt.Fprintln(stderr, "error: --apply exige --backup-ref <texto> con la referencia del respaldo previo (docs/respaldos.md)")
			return migrateFailure
		}
		if len(ref) > 200 {
			_, _ = fmt.Fprintln(stderr, "error: --backup-ref es demasiado largo (máximo 200 caracteres)")
			return migrateFailure
		}
	}
	cfg, err := load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return migrateFailure
	}
	if o.apply && cfg.IsProduction() && !o.yesProduction {
		_, _ = fmt.Fprintln(stderr, "error: con APP_ENV=production, --apply exige además --yes-production")
		return migrateFailure
	}
	ctx, cancel := context.WithTimeout(context.Background(), migrateNullsTimeout)
	defer cancel()
	m, closeFn, err := open(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), cfg.MongoURI))
		return migrateFailure
	}
	defer closeFn()
	return migrateNulls(ctx, stdout, stderr, cfg.MongoURI, m, o.apply, ref)
}

// migrateNulls quita los null de todas las colecciones. Sin apply solo cuenta
// (dry-run). Nunca borra documentos, solo $unset / $pull de los null; es
// idempotente y reanudable (una segunda pasada no encuentra nada que cambiar).
// No imprime valores ni ids.
func migrateNulls(ctx context.Context, stdout, stderr io.Writer, uri string, m nullMigrator, apply bool, backupRef string) int {
	mode := "DRY-RUN (no se escribe nada)"
	if apply {
		mode = "APPLY, backup-ref=" + strconv.Quote(backupRef)
	}
	_, _ = fmt.Fprintf(stdout, "migrate-remove-nulls: %s\n", mode)
	cols, err := m.Collections(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), uri))
		return migrateFailure
	}
	var totalDocs, totalMods, totalUnsupported int64
	for _, col := range cols {
		var scanned, wouldChange, nulls, modified, unsupportedN int64
		unsupported := map[string]int64{}
		var batch []nullOp
		flush := func() error {
			if !apply || len(batch) == 0 {
				batch = batch[:0]
				return nil
			}
			n, err := m.Apply(ctx, col, batch)
			batch = batch[:0]
			modified += n
			return err
		}
		err := m.Scan(ctx, col, func(doc bson.Raw) error {
			scanned++
			unset, pull, unsup, err := planNulls(doc)
			if err != nil {
				return err
			}
			for p, n := range unsup {
				unsupported[p] += n
				unsupportedN += n
			}
			if len(unset)+len(pull) == 0 {
				return nil
			}
			wouldChange++
			nulls += int64(len(unset))
			id, err := doc.LookupErr("_id")
			if err != nil {
				return fmt.Errorf("documento sin _id: %w", err)
			}
			batch = append(batch, nullOp{ID: bson.RawValue{Type: id.Type, Value: slices.Clone(id.Value)}, Unset: unset, Pull: pull})
			if len(batch) >= migrateBatchSize {
				return flush()
			}
			return nil
		})
		if err == nil {
			err = flush()
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %s: %s\n", col, redact(err.Error(), uri))
			return migrateFailure
		}
		totalDocs += wouldChange
		totalMods += modified
		totalUnsupported += unsupportedN
		verb := "cambiaría"
		if apply {
			verb = "modificó"
		}
		changed := wouldChange
		if apply {
			changed = modified
		}
		_, _ = fmt.Fprintf(stdout, "%-20s revisados=%d con_null=%d %s=%d campos_null_a_quitar=%d\n", col, scanned, wouldChange, verb, changed, nulls)
		paths := make([]string, 0, len(unsupported))
		for p := range unsupported {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			_, _ = fmt.Fprintf(stdout, "  NO SOPORTADA (no se toca): %s.%s: %d\n", col, p, unsupported[p])
		}
	}
	if apply {
		_, _ = fmt.Fprintf(stdout, "\nresumen: %d documentos modificados en %d colecciones; backup-ref=%s\n", totalMods, len(cols), strconv.Quote(backupRef))
	} else {
		_, _ = fmt.Fprintf(stdout, "\nresumen: %d documentos cambiarían en %d colecciones (dry-run)\n", totalDocs, len(cols))
	}
	if totalUnsupported > 0 {
		_, _ = fmt.Fprintf(stdout, "hay %d null en rutas no soportadas; añade una regla antes de verificar\n", totalUnsupported)
		return migrateUnsupported
	}
	return migrateOK
}
