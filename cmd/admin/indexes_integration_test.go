//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// baselinePath es el fixture de equivalencia de T004 (plan 012 §7.6): los 34
// índices que crean los EnsureIndexes en una base limpia, sin _id_.
var baselinePath = filepath.Join("testdata", "indexes_baseline.json")

// updateBaselineEnv regenera el fixture en lugar de compararlo. Solo se usa
// una vez, con el código ANTERIOR al refactor, en una base limpia.
const updateBaselineEnv = "VAIVEN_UPDATE_INDEX_BASELINE"

// testMongo arranca MongoDB 7 (replica set) con Testcontainers y devuelve un
// cliente conectado; todo se cierra al terminar la prueba.
func testMongo(t *testing.T) (*mongodb.Client, string) {
	t.Helper()
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatalf("no se pudo iniciar MongoDB con Docker o Podman: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	uri, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	// directConnection: el replica set se anuncia con la IP interna del
	// contenedor, que no siempre se alcanza desde el host (Podman en Windows).
	uri += "&directConnection=true"
	client, err := mongodb.Connect(ctx, uri, 30*time.Second)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	return client, uri
}

// effectiveIndexes lista los índices reales de todas las colecciones de db,
// sin _id_ y sin los metadatos v y ns, ordenados por colección y nombre. Cada
// índice se guarda como JSON extendido canónico, que conserva los tipos BSON.
func effectiveIndexes(t *testing.T, db *drv.Database) []baselineIndex {
	t.Helper()
	ctx := context.Background()
	cols, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		t.Fatalf("listCollections: %v", err)
	}
	var out []baselineIndex
	for _, col := range cols {
		cur, err := db.Collection(col).Indexes().List(ctx)
		if err != nil {
			t.Fatalf("listIndexes %s: %v", col, err)
		}
		var docs []bson.D
		if err := cur.All(ctx, &docs); err != nil {
			t.Fatalf("leer índices de %s: %v", col, err)
		}
		for _, d := range docs {
			var name string
			clean := bson.D{}
			for _, e := range d {
				switch e.Key {
				case "v", "ns":
					continue
				case "name":
					name, _ = e.Value.(string)
				}
				clean = append(clean, e)
			}
			if name == "_id_" {
				continue
			}
			raw, err := bson.MarshalExtJSON(clean, true, false)
			if err != nil {
				t.Fatalf("JSON de %s.%s: %v", col, name, err)
			}
			out = append(out, baselineIndex{Collection: col, Name: name, Spec: json.RawMessage(raw)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Collection != out[j].Collection {
			return out[i].Collection < out[j].Collection
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// baselineIndex es una entrada del fixture.
type baselineIndex struct {
	Collection string          `json:"collection"`
	Name       string          `json:"name"`
	Spec       json.RawMessage `json:"spec"` // JSON extendido canónico del documento de listIndexes
}

// TestIntegrationIndexBaseline es la prueba 1 del plan 012 §7.6: los
// EnsureIndexes de los 14 repositorios, en una base limpia, crean exactamente
// los 34 índices del fixture (claves, orden, opciones, TTL, filtro parcial y
// configuración de texto). Con VAIVEN_UPDATE_INDEX_BASELINE=1 regenera el
// fixture en lugar de compararlo.
func TestIntegrationIndexBaseline(t *testing.T) {
	client, _ := testMongo(t)
	db := client.Database("vaiven_baseline")
	ctx := context.Background()
	if err := adaptermongo.EnsureAll(ctx, ensurers(db)...); err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}
	got := effectiveIndexes(t, db)
	if len(got) != 34 {
		t.Errorf("índices creados = %d, want 34", len(got))
	}

	if os.Getenv(updateBaselineEnv) == "1" {
		writeBaseline(t, got)
		t.Logf("fixture regenerado en %s con %d índices", baselinePath, len(got))
		return
	}
	want := readBaseline(t)
	compareBaseline(t, want, got)
}

func writeBaseline(t *testing.T, idx []baselineIndex) {
	t.Helper()
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		t.Fatalf("serializar fixture: %v", err)
	}
	if err := os.WriteFile(baselinePath, append(b, '\n'), 0o644); err != nil {
		t.Fatalf("escribir fixture: %v", err)
	}
}

func readBaseline(t *testing.T) []baselineIndex {
	t.Helper()
	b, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("leer fixture %s: %v", baselinePath, err)
	}
	var idx []baselineIndex
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatalf("fixture inválido: %v", err)
	}
	return idx
}

// compareBaseline exige los mismos índices, con el mismo documento efectivo
// (se compara el JSON compactado: tipos BSON incluidos).
func compareBaseline(t *testing.T, want, got []baselineIndex) {
	t.Helper()
	key := func(i baselineIndex) string { return i.Collection + "." + i.Name }
	gotBy := map[string]baselineIndex{}
	for _, g := range got {
		gotBy[key(g)] = g
	}
	for _, w := range want {
		g, ok := gotBy[key(w)]
		if !ok {
			t.Errorf("falta %s", key(w))
			continue
		}
		delete(gotBy, key(w))
		if !sameJSON(t, w.Spec, g.Spec) {
			t.Errorf("%s cambió:\n want %s\n  got %s", key(w), w.Spec, g.Spec)
		}
	}
	for k := range gotBy {
		t.Errorf("sobra %s (no está en el fixture)", k)
	}
}

func sameJSON(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var ca, cb bytes.Buffer
	if err := json.Compact(&ca, a); err != nil {
		t.Fatalf("compactar: %v", err)
	}
	if err := json.Compact(&cb, b); err != nil {
		t.Fatalf("compactar: %v", err)
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}
