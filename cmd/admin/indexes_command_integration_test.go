//go:build integration

package main

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	drv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo/catalog"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// commandOn ejecuta la comparación del comando contra db con el catálogo.
func commandOn(t *testing.T, db *drv.Database) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := checkIndexes(context.Background(), &out, &errb, "", catalog.All(), dbIndexSource{db: db})
	if errb.Len() > 0 {
		t.Logf("stderr: %s", errb.String())
	}
	return code, out.String()
}

func mustEnsure(t *testing.T, db *drv.Database) {
	t.Helper()
	if err := adaptermongo.EnsureAll(context.Background(), ensurers(db)...); err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}
}

func mustDrop(t *testing.T, db *drv.Database, col, name string) {
	t.Helper()
	if err := db.Collection(col).Indexes().DropOne(context.Background(), name); err != nil {
		t.Fatalf("dropIndex %s.%s: %v", col, name, err)
	}
}

func mustCreate(t *testing.T, db *drv.Database, col string, keys bson.D, name string) {
	t.Helper()
	if _, err := db.Collection(col).Indexes().CreateOne(context.Background(),
		drv.IndexModel{Keys: keys, Options: options.Index().SetName(name)}); err != nil {
		t.Fatalf("createIndex %s.%s: %v", col, name, err)
	}
}

// Pruebas 2, 4, 5, 6 y 7 (plan 012 §7.6) y CA-03, sobre MongoDB real. La
// prueba modifica índices solo en su propia base de Testcontainers.
func TestIntegrationIndexesCommand(t *testing.T) {
	client, _ := testMongo(t)
	db := client.Database("vaiven_cmd")
	mustEnsure(t, db)

	if code, out := commandOn(t, db); code != indexesOK || !strings.Contains(out, "34 correctos") {
		t.Fatalf("base recién creada: código %d, want 0\n%s", code, out)
	}

	// 2. Idempotencia: otro EnsureAll no cambia nada.
	mustEnsure(t, db)
	if code, out := commandOn(t, db); code != indexesOK {
		t.Fatalf("tras un segundo EnsureAll: código %d\n%s", code, out)
	}

	// 4. Faltante (CA-03).
	mustDrop(t, db, "sanctions", "user_createdAt")
	if code, out := commandOn(t, db); code != indexesDiscrepancy || !strings.Contains(out, "FALTANTE        sanctions.user_createdAt") {
		t.Fatalf("faltante: código %d\n%s", code, out)
	}
	mustEnsure(t, db)

	// 5. Sobrante.
	mustCreate(t, db, "reports", bson.D{{Key: "targetType", Value: 1}}, "extra_targetType")
	if code, out := commandOn(t, db); code != indexesDiscrepancy || !strings.Contains(out, "SOBRANTE        reports.extra_targetType") {
		t.Fatalf("sobrante: código %d\n%s", code, out)
	}
	mustDrop(t, db, "reports", "extra_targetType")

	// 6. Incompatible: mismo nombre, otra dirección de clave.
	mustDrop(t, db, "push_tokens", "user")
	mustCreate(t, db, "push_tokens", bson.D{{Key: "userId", Value: -1}}, "user")
	if code, out := commandOn(t, db); code != indexesDiscrepancy || !strings.Contains(out, "INCOMPATIBLE    push_tokens.user") {
		t.Fatalf("incompatible: código %d\n%s", code, out)
	}
	mustDrop(t, db, "push_tokens", "user")

	// 7. Nombre distinto: el equivalente con otro nombre no cuenta como correcto.
	mustCreate(t, db, "push_tokens", bson.D{{Key: "userId", Value: 1}}, "userId_1")
	code, out := commandOn(t, db)
	for _, want := range []string{"FALTANTE        push_tokens.user", "SOBRANTE        push_tokens.userId_1", "NOMBRE DISTINTO"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
	if code != indexesDiscrepancy {
		t.Fatalf("nombre distinto: código %d, want 1", code)
	}
	mustDrop(t, db, "push_tokens", "userId_1")
	mustEnsure(t, db)
	if code, out := commandOn(t, db); code != indexesOK {
		t.Fatalf("tras restaurar: código %d\n%s", code, out)
	}
}

// Pruebas 8 y 9: con un usuario de rol read, el comando funciona por la ruta
// real (mongoIndexes) y el driver solo envía comandos de lectura. El módulo de
// Testcontainers genera el keyFile que exige la autenticación con replica set.
func TestIntegrationIndexesReadOnlyUser(t *testing.T) {
	ctx := context.Background()
	const rootPwd, readerPwd, dbName = "root-pwd-de-prueba", "lector-pwd-de-prueba", "vaiven_ro"
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"),
		tcmongo.WithUsername("root"), tcmongo.WithPassword(rootPwd))
	if err != nil {
		t.Fatalf("no se pudo iniciar MongoDB con autenticación: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	rootURI, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	if strings.Contains(rootURI, "?") {
		rootURI += "&directConnection=true"
	} else {
		rootURI += "/?directConnection=true"
	}
	admin, err := mongodb.Connect(ctx, rootURI, 30*time.Second)
	if err != nil {
		t.Fatalf("conectar como root: %v", err)
	}
	t.Cleanup(func() { _ = admin.Disconnect(context.Background()) })
	db := admin.Database(dbName)
	mustEnsure(t, db)
	if err := db.RunCommand(ctx, bson.D{
		{Key: "createUser", Value: "lector"}, {Key: "pwd", Value: readerPwd},
		{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "read"}, {Key: "db", Value: dbName}}}},
	}).Err(); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	if !strings.Contains(rootURI, "root:"+rootPwd+"@") {
		t.Fatal("la URI del contenedor no tiene el formato esperado")
	}
	readerURI := strings.Replace(rootURI, "root:"+rootPwd+"@", "lector:"+readerPwd+"@", 1) + "&authSource=" + dbName

	var mu sync.Mutex
	var cmds []string
	mon := &event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
		mu.Lock()
		cmds = append(cmds, e.CommandName)
		mu.Unlock()
	}}
	reader, err := drv.Connect(options.Client().ApplyURI(readerURI).SetMonitor(mon))
	if err != nil {
		t.Fatalf("conectar como lector: %v", err)
	}
	t.Cleanup(func() { _ = reader.Disconnect(context.Background()) })

	// El rol read de verdad no puede escribir; si pudiera, la prueba no vale.
	if _, err := reader.Database(dbName).Collection("users").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}); err == nil {
		t.Fatal("el usuario de solo lectura pudo escribir")
	}

	// 9. Solo comandos de lectura durante la inspección.
	mu.Lock()
	cmds = nil
	mu.Unlock()
	if err := reader.Ping(ctx, nil); err != nil {
		t.Fatalf("ping: %v", err)
	}
	var out bytes.Buffer
	if code := checkIndexes(ctx, &out, &bytes.Buffer{}, readerURI, catalog.All(), dbIndexSource{db: reader.Database(dbName)}); code != indexesOK {
		t.Fatalf("lector: código %d\n%s", code, out.String())
	}
	allowed := map[string]bool{"ping": true, "listCollections": true, "listIndexes": true, "getMore": true, "killCursors": true}
	mu.Lock()
	seen := append([]string(nil), cmds...)
	mu.Unlock()
	sawList := false
	for _, n := range seen {
		if !allowed[n] {
			t.Errorf("comando no permitido durante la inspección: %s", n)
		}
		sawList = sawList || n == "listIndexes"
	}
	if !sawList {
		t.Errorf("no se observó listIndexes: %v", seen)
	}

	// 8. La ruta real del subcomando, con la configuración del lector.
	var out2, errb bytes.Buffer
	cfg := config.Config{MongoURI: readerURI, MongoDB: dbName, MongoConnectTimeout: 30 * time.Second}
	if code := runIndexes(&out2, &errb, cfg, mongoIndexes, catalog.All()); code != indexesOK {
		t.Fatalf("runIndexes como lector: código %d\n%s\n%s", code, out2.String(), errb.String())
	}
	if strings.Contains(out2.String()+errb.String(), readerPwd) {
		t.Fatal("la salida muestra la contraseña")
	}
}
