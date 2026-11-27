//go:build integration

package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// Spec 021, T012 (CA-05): TxRunner con un replica set real.
func TestIntegrationTxRunner(t *testing.T) {
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatalf("no se pudo iniciar MongoDB con Docker: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	uri, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	client, err := mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("vaiven_tx")
	a, b := db.Collection("tx_a"), db.Collection("tx_b")
	// Las colecciones deben existir antes de la transacción (MongoDB 7).
	for _, n := range []string{"tx_a", "tx_b"} {
		if err := db.CreateCollection(ctx, n); err != nil {
			t.Fatalf("CreateCollection: %v", err)
		}
	}
	runner := NewTxRunner(db)
	count := func(col *drv.Collection) int64 {
		n, err := col.CountDocuments(ctx, bson.D{})
		if err != nil {
			t.Fatalf("CountDocuments: %v", err)
		}
		return n
	}

	t.Run("confirma todas las escrituras", func(t *testing.T) {
		err := runner.Do(ctx, func(tx context.Context) error {
			if _, err := a.InsertOne(tx, bson.M{"k": "ok"}); err != nil {
				return err
			}
			_, err := b.InsertOne(tx, bson.M{"k": "ok"})
			return err
		})
		if err != nil || count(a) != 1 || count(b) != 1 {
			t.Fatalf("err=%v a=%d b=%d", err, count(a), count(b))
		}
	})

	t.Run("un error a media transacción no deja cambios", func(t *testing.T) {
		boom := errors.New("falla simulada")
		err := runner.Do(ctx, func(tx context.Context) error {
			if _, err := a.InsertOne(tx, bson.M{"k": "mitad"}); err != nil {
				return err
			}
			if _, err := a.DeleteMany(tx, bson.D{{Key: "k", Value: "ok"}}); err != nil {
				return err
			}
			return boom
		})
		if err != boom { // se devuelve sin envolver
			t.Fatalf("err = %v, want la misma falla", err)
		}
		if n, _ := a.CountDocuments(ctx, bson.D{{Key: "k", Value: "mitad"}}); n != 0 {
			t.Error("la escritura de la mitad no debe quedar")
		}
		if n, _ := a.CountDocuments(ctx, bson.D{{Key: "k", Value: "ok"}}); n != 1 {
			t.Error("el borrado de la mitad debe revertirse")
		}
	})

	t.Run("sin el ctx de la transacción las escrituras quedan fuera", func(t *testing.T) {
		// Documenta por qué los repositorios deben usar el ctx de fn.
		before := count(b)
		_ = runner.Do(ctx, func(context.Context) error {
			_, err := b.InsertOne(ctx, bson.M{"k": "fuera"})
			if err != nil {
				return err
			}
			return errors.New("abortar")
		})
		if count(b) != before+1 {
			t.Error("una escritura hecha con el ctx externo no es parte de la transacción")
		}
	})

	t.Run("repite la función ante un error transitorio", func(t *testing.T) {
		attempts := 0
		err := runner.Do(ctx, func(tx context.Context) error {
			attempts++
			if _, err := a.InsertOne(tx, bson.M{"k": "reintento"}); err != nil {
				return err
			}
			if attempts == 1 {
				return drv.CommandError{Code: 112, Name: "WriteConflict", Labels: []string{"TransientTransactionError"}}
			}
			return nil
		})
		if err != nil || attempts != 2 {
			t.Fatalf("err=%v attempts=%d", err, attempts)
		}
		if n, _ := a.CountDocuments(ctx, bson.D{{Key: "k", Value: "reintento"}}); n != 1 {
			t.Errorf("el reintento debe dejar un solo documento, hay %d", n)
		}
	})

	t.Run("propaga la cancelación del contexto", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if err := runner.Do(cctx, func(context.Context) error { return nil }); err == nil {
			t.Error("con el ctx cancelado Do debe fallar")
		}
	})
}
