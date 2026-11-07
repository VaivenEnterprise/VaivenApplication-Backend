//go:build integration

package mongo

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	drv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
)

// testClient lo comparten todas las pruebas del paquete: un solo contenedor
// de MongoDB (replica set) para no arrancar uno por prueba.
var testClient *mongodb.Client

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := tcmongo.Run(ctx, "mongo:7", tcmongo.WithReplicaSet("rs0"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "no se pudo iniciar MongoDB con Docker o Podman:", err)
		os.Exit(1)
	}
	uri, err := c.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ConnectionString:", err)
		_ = c.Terminate(ctx)
		os.Exit(1)
	}
	// directConnection: con Podman en Windows la IP interna del replica set no
	// es accesible desde el host (ver internal/adapter/mongo).
	testClient, err = mongodb.Connect(ctx, uri+"&directConnection=true", 30*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Connect:", err)
		_ = c.Terminate(ctx)
		os.Exit(1)
	}
	code := m.Run()
	_ = testClient.Disconnect(ctx)
	_ = c.Terminate(ctx)
	os.Exit(code)
}

// newDB devuelve una base vacía y exclusiva de la prueba.
func newDB(t *testing.T) *drv.Database {
	t.Helper()
	db := testClient.Database(fmt.Sprintf("auth_it_%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	return db
}

func newUser(email string) domain.User {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return domain.User{
		Email:           email,
		PasswordHash:    "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		DisplayName:     "Ana",
		Role:            domain.RoleUser,
		Status:          domain.StatusActive,
		AcceptedTermsAt: now,
		CreatedAt:       now,
		UpdatedAt:       now,
		Version:         1,
	}
}

func TestIntegrationUserRepo(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepo(newDB(t))

	// Dos veces: EnsureIndexes es idempotente.
	for i := 0; i < 2; i++ {
		if err := repo.EnsureIndexes(ctx); err != nil {
			t.Fatalf("EnsureIndexes (vez %d): %v", i+1, err)
		}
	}
	specs, err := repo.col.Indexes().ListSpecifications(ctx)
	if err != nil {
		t.Fatalf("ListSpecifications: %v", err)
	}
	unique := false
	for _, s := range specs {
		if s.Name == "email_unique" && s.Unique != nil && *s.Unique {
			unique = true
		}
	}
	if !unique {
		t.Fatal("falta el índice único email_unique")
	}

	created, err := repo.Create(ctx, newUser("ana@example.com"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(created.ID) != 24 {
		t.Fatalf("ID = %q, want ObjectID en hex", created.ID)
	}

	byEmail, err := repo.ByEmail(ctx, "ana@example.com")
	if err != nil {
		t.Fatalf("ByEmail: %v", err)
	}
	byID, err := repo.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if byEmail.ID != created.ID || byID.Email != "ana@example.com" || byID.DisplayName != "Ana" ||
		byID.Role != domain.RoleUser || byID.Status != domain.StatusActive || byID.Phone != nil ||
		!byID.CreatedAt.Equal(created.CreatedAt) || byID.CreatedAt.Location() != time.UTC {
		t.Errorf("leído = %+v, creado = %+v", byID, created)
	}

	if _, err := repo.Create(ctx, newUser("ana@example.com")); !domain.Is(err, domain.ErrEmailTaken) {
		t.Errorf("Create duplicado = %v, want ErrEmailTaken", err)
	}

	for name, find := range map[string]func() (domain.User, error){
		"correo inexistente": func() (domain.User, error) { return repo.ByEmail(ctx, "nadie@example.com") },
		"id inexistente":     func() (domain.User, error) { return repo.ByID(ctx, "0123456789abcdef01234567") },
		"id mal formado":     func() (domain.User, error) { return repo.ByID(ctx, "no-es-un-id") },
	} {
		if _, err := find(); !domain.Is(err, domain.ErrUserNotFound) {
			t.Errorf("%s: err = %v, want ErrUserNotFound", name, err)
		}
	}
}
