//go:build integration

package catalog

import (
	"context"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	incidentsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/incidents/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	reportsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/adapter/mongo"
	sanctionsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/sanctions/adapter/mongo"
)

// Spec 021, T026 (D-4, CA-11): el índice TTL {purgeAt:1} con expireAfterSeconds
// 0 borra los documentos de incidents, reports y sanctions cuando purgeAt ya
// pasó, y no toca a los que no tienen purgeAt ni a los de fecha futura.
func TestIntegrationPurgeAtTTLDeletesExpiredDocuments(t *testing.T) {
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatalf("no se pudo iniciar MongoDB con Docker o Podman: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	uri, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("vaiven_retention")

	// El monitor de TTL corre cada 60 s por omisión: se acelera (solo en la
	// prueba) para no esperar un minuto. Si el servidor no lo permite, se espera.
	timeout := 2 * time.Minute
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "setParameter", Value: 1}, {Key: "ttlMonitorSleepSecs", Value: 1}}).Err(); err == nil {
		timeout = 30 * time.Second
	}

	past, future := time.Now().Add(-time.Hour).UTC(), time.Now().Add(24*time.Hour).UTC()
	cols := []struct {
		name  string
		specs []adaptermongo.IndexSpec
	}{
		{incidentsmongo.IncidentsCollection, incidentsmongo.IncidentIndexes()},
		{reportsmongo.ReportsCollection, reportsmongo.ReportIndexes()},
		{sanctionsmongo.SanctionsCollection, sanctionsmongo.SanctionIndexes()},
	}
	for _, col := range cols {
		coll := db.Collection(col.name)
		if _, err := coll.Indexes().CreateMany(ctx, adaptermongo.Models(col.specs)); err != nil {
			t.Fatalf("índices de %s: %v", col.name, err)
		}
		// Los campos únicos de reports se distinguen por targetId.
		docs := []any{
			bson.D{{Key: "k", Value: "vencido"}, {Key: "reporterId", Value: "a"}, {Key: "targetType", Value: "item"}, {Key: "targetId", Value: "1"}, {Key: "purgeAt", Value: past}},
			bson.D{{Key: "k", Value: "vigente"}, {Key: "reporterId", Value: "a"}, {Key: "targetType", Value: "item"}, {Key: "targetId", Value: "2"}, {Key: "purgeAt", Value: future}},
			bson.D{{Key: "k", Value: "sin-fecha"}, {Key: "reporterId", Value: "a"}, {Key: "targetType", Value: "item"}, {Key: "targetId", Value: "3"}},
		}
		if _, err := coll.InsertMany(ctx, docs); err != nil {
			t.Fatalf("sembrar %s: %v", col.name, err)
		}
	}

	deadline := time.Now().Add(timeout)
	for _, col := range cols {
		coll := db.Collection(col.name)
		for {
			n, err := coll.CountDocuments(ctx, bson.D{{Key: "k", Value: "vencido"}})
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: el TTL no borró el documento vencido", col.name)
			}
			time.Sleep(500 * time.Millisecond)
		}
		for _, k := range []string{"vigente", "sin-fecha"} {
			if n, _ := coll.CountDocuments(ctx, bson.D{{Key: "k", Value: k}}); n != 1 {
				t.Errorf("%s: el documento %s no debía borrarse (quedan %d)", col.name, k, n)
			}
		}
	}
}
