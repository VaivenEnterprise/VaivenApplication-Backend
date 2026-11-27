package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// verifyNullsTimeout es el plazo total de `admin verify-no-nulls`.
const verifyNullsTimeout = 30 * time.Minute

// Códigos de salida de `admin verify-no-nulls` (spec 021, RF-13; misma
// convención que `indexes`).
const (
	verifyNullsClean   = 0 // ningún valor null en ninguna colección
	verifyNullsFound   = 1 // hay al menos un null
	verifyNullsFailure = 2 // configuración, conexión, plazo o lectura
)

// nullSource lee las colecciones para buscar null. Solo lectura.
type nullSource interface {
	// Collections lista las colecciones (no vistas ni system.*).
	Collections(ctx context.Context) ([]string, error)
	// Scan entrega cada documento de la colección como bson.Raw, por lotes.
	// El documento solo es válido durante la llamada a visit.
	Scan(ctx context.Context, collection string, visit func(doc bson.Raw) error) error
}

// nullsFactory abre la fuente; closeFn cierra la conexión.
type nullsFactory func(ctx context.Context, cfg config.Config) (src nullSource, closeFn func(), err error)

// mongoNulls es la nullsFactory real: solo envía ping, listCollections y find
// (basta un usuario con rol read).
func mongoNulls(ctx context.Context, cfg config.Config) (nullSource, func(), error) {
	mc, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoConnectTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("conectar a MongoDB: %w", err)
	}
	return dbNullSource{db: mc.Database(cfg.MongoDB)}, func() { _ = mc.Disconnect(context.Background()) }, nil
}

type dbNullSource struct{ db *drv.Database }

func (s dbNullSource) Collections(ctx context.Context) ([]string, error) {
	names, err := s.db.ListCollectionNames(ctx, bson.D{{Key: "type", Value: "collection"}})
	if err != nil {
		return nil, fmt.Errorf("listCollections: %w", err)
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if !strings.HasPrefix(n, "system.") {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s dbNullSource) Scan(ctx context.Context, collection string, visit func(doc bson.Raw) error) error {
	cur, err := s.db.Collection(collection).Find(ctx, bson.D{})
	if err != nil {
		return fmt.Errorf("find %s: %w", collection, err)
	}
	defer func() { _ = cur.Close(ctx) }()
	for cur.Next(ctx) {
		if err := visit(cur.Current); err != nil {
			return err
		}
	}
	if err := cur.Err(); err != nil {
		return fmt.Errorf("leer %s: %w", collection, err)
	}
	return nil
}

// nullPaths recorre el documento en cualquier nivel (subdocumentos y arreglos)
// y devuelve cuántos valores BSON null hay por ruta normalizada: los índices de
// arreglo se escriben "[]" (p. ej. "payments[].paidAt"). Nunca devuelve valores
// ni ids.
func nullPaths(doc bson.Raw) (map[string]int64, error) {
	out := map[string]int64{}
	if err := walkNulls(doc, "", out); err != nil {
		return nil, err
	}
	return out, nil
}

func walkNulls(doc bson.Raw, prefix string, out map[string]int64) error {
	elems, err := doc.Elements()
	if err != nil {
		return fmt.Errorf("documento BSON inválido: %w", err)
	}
	for _, el := range elems {
		path := el.Key()
		if prefix != "" {
			path = prefix + "." + path
		}
		if err := walkValue(el.Value(), path, out); err != nil {
			return err
		}
	}
	return nil
}

func walkValue(v bson.RawValue, path string, out map[string]int64) error {
	switch v.Type {
	case bson.TypeNull:
		out[path]++
	case bson.TypeEmbeddedDocument:
		return walkNulls(v.Document(), path, out)
	case bson.TypeArray:
		vals, err := v.Array().Values()
		if err != nil {
			return fmt.Errorf("arreglo BSON inválido: %w", err)
		}
		for _, item := range vals {
			if err := walkValue(item, path+"[]", out); err != nil {
				return err
			}
		}
	}
	return nil
}

// runVerifyNulls es el subcomando completo: abre la fuente, verifica y cierra
// la conexión ANTES de devolver el código.
func runVerifyNulls(stdout, stderr io.Writer, cfg config.Config, open nullsFactory) int {
	ctx, cancel := context.WithTimeout(context.Background(), verifyNullsTimeout)
	defer cancel()
	src, closeFn, err := open(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), cfg.MongoURI))
		return verifyNullsFailure
	}
	defer closeFn()
	return verifyNulls(ctx, stdout, stderr, cfg.MongoURI, src)
}

// verifyNulls cuenta los null por colección y ruta, y escribe una línea por
// ruta y un resumen. No modifica nada ni imprime valores ni ids.
func verifyNulls(ctx context.Context, stdout, stderr io.Writer, uri string, src nullSource) int {
	cols, err := src.Collections(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), uri))
		return verifyNullsFailure
	}
	var totalNulls, dirtyCols int64
	for _, col := range cols {
		counts := map[string]int64{}
		var docs, docsWithNull int64
		err := src.Scan(ctx, col, func(doc bson.Raw) error {
			docs++
			paths, err := nullPaths(doc)
			if err != nil {
				return err
			}
			if len(paths) > 0 {
				docsWithNull++
			}
			for p, n := range paths {
				counts[p] += n
			}
			return nil
		})
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %s: %s\n", col, redact(err.Error(), uri))
			return verifyNullsFailure
		}
		if len(counts) == 0 {
			_, _ = fmt.Fprintf(stdout, "OK     %s: %d documentos, 0 con null\n", col, docs)
			continue
		}
		dirtyCols++
		_, _ = fmt.Fprintf(stdout, "NULL   %s: %d documentos, %d con null\n", col, docs, docsWithNull)
		paths := make([]string, 0, len(counts))
		for p := range counts {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			totalNulls += counts[p]
			_, _ = fmt.Fprintf(stdout, "         %s.%s: %d\n", col, p, counts[p])
		}
	}
	_, _ = fmt.Fprintf(stdout, "\nresumen: %d colecciones revisadas, %d con null, %d valores null en total\n", len(cols), dirtyCols, totalNulls)
	if totalNulls > 0 {
		return verifyNullsFound
	}
	return verifyNullsClean
}
