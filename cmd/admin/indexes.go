package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// indexesTimeout es el plazo total de `admin indexes` (conexión y lectura).
const indexesTimeout = 30 * time.Second

// Códigos de salida de `admin indexes` (spec 012, RF-03).
const (
	indexesOK          = 0 // todo coincide
	indexesDiscrepancy = 1 // falta, sobra, es incompatible o no se puede verificar algún índice
	indexesError       = 2 // configuración, conexión, plazo o ejecución
)

// indexSource lee los índices reales por colección. Solo lectura.
type indexSource interface {
	ListIndexes(ctx context.Context) (map[string][]bson.D, error)
}

// indexesFactory abre la fuente de índices; close cierra la conexión.
type indexesFactory func(ctx context.Context, cfg config.Config) (src indexSource, closeFn func(), err error)

// mongoIndexes es la indexesFactory real. Solo envía ping (al conectar),
// listCollections y listIndexes: basta un usuario con rol read.
func mongoIndexes(ctx context.Context, cfg config.Config) (indexSource, func(), error) {
	mc, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoConnectTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("conectar a MongoDB: %w", err)
	}
	return dbIndexSource{db: mc.Database(cfg.MongoDB)}, func() { _ = mc.Disconnect(context.Background()) }, nil
}

// dbIndexSource lee con listCollections y listIndexes.
type dbIndexSource struct{ db *drv.Database }

// ListIndexes devuelve los índices de cada colección (no vistas ni system.*).
func (s dbIndexSource) ListIndexes(ctx context.Context) (map[string][]bson.D, error) {
	names, err := s.db.ListCollectionNames(ctx, bson.D{{Key: "type", Value: "collection"}})
	if err != nil {
		return nil, fmt.Errorf("listCollections: %w", err)
	}
	out := make(map[string][]bson.D, len(names))
	for _, n := range names {
		if strings.HasPrefix(n, "system.") {
			continue
		}
		cur, err := s.db.Collection(n).Indexes().List(ctx)
		if err != nil {
			return nil, fmt.Errorf("listIndexes %s: %w", n, err)
		}
		var docs []bson.D
		if err := cur.All(ctx, &docs); err != nil {
			return nil, fmt.Errorf("leer índices de %s: %w", n, err)
		}
		out[n] = docs
	}
	return out, nil
}

// runIndexes es el subcomando completo: abre la fuente, compara y cierra la
// conexión ANTES de devolver el código (main hace os.Exit después).
func runIndexes(stdout, stderr io.Writer, cfg config.Config, open indexesFactory, expected []adaptermongo.IndexSpec) int {
	ctx, cancel := context.WithTimeout(context.Background(), indexesTimeout)
	defer cancel()
	src, closeFn, err := open(ctx, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), cfg.MongoURI))
		return indexesError
	}
	defer closeFn()
	return checkIndexes(ctx, stdout, stderr, cfg.MongoURI, expected, src)
}

// checkIndexes compara lo declarado con lo real y escribe una línea por
// índice y un resumen. No crea, modifica ni borra nada.
func checkIndexes(ctx context.Context, stdout, stderr io.Writer, uri string, expected []adaptermongo.IndexSpec, src indexSource) int {
	actual, err := src.ListIndexes(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), uri))
		return indexesError
	}
	findings, err := adaptermongo.CompareIndexes(expected, actual)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: declaración de índices inválida: %v\n", err)
		return indexesError
	}
	counts := map[adaptermongo.IndexStatus]int{}
	renamed := 0
	for _, f := range findings {
		counts[f.Status]++
		line := fmt.Sprintf("%-15s %s.%s", f.Status, f.Collection, f.Name)
		if f.Detail != "" {
			line += " — " + f.Detail
		}
		if f.SameShapeAs != "" {
			renamed++
			line += fmt.Sprintf(" (NOMBRE DISTINTO: equivale a %q; sigue siendo una discrepancia)", f.SameShapeAs)
		}
		_, _ = fmt.Fprintln(stdout, line)
	}
	_, _ = fmt.Fprintf(stdout, "\nresumen: %d correctos, %d faltantes, %d sobrantes, %d incompatibles, %d no verificables",
		counts[adaptermongo.IndexOK], counts[adaptermongo.IndexMissing], counts[adaptermongo.IndexExtra],
		counts[adaptermongo.IndexIncompatible], counts[adaptermongo.IndexUnverifiable])
	if renamed > 0 {
		_, _ = fmt.Fprintf(stdout, " (%d con nota NOMBRE DISTINTO)", renamed)
	}
	_, _ = fmt.Fprintln(stdout)
	for _, f := range findings {
		if f.Discrepancy() {
			return indexesDiscrepancy
		}
	}
	return indexesOK
}

// userinfo es el usuario:contraseña de una URI de MongoDB.
var userinfo = regexp.MustCompile(`(mongodb(?:\+srv)?://)[^@/\s]+@`)

// redact quita la URI completa y cualquier credencial embebida de un mensaje
// de error antes de mostrarlo (Art. VI).
func redact(msg, uri string) string {
	if u := strings.TrimSpace(uri); u != "" {
		msg = strings.ReplaceAll(msg, u, "<MONGO_URI>")
	}
	return userinfo.ReplaceAllString(msg, "${1}<credenciales>@")
}
