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
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
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

func TestIntegrationUserRepoProfileAndAdmin(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepo(newDB(t))
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	ana, err := repo.Create(ctx, newUser("ana@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	beto, err := repo.Create(ctx, newUser("beto@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)

	name, phone := "Ana López", "+525512345678"
	u, err := repo.UpdateProfile(ctx, ana.ID, usecase.ProfileChanges{DisplayName: &name, Phone: &phone}, now)
	if err != nil || u.DisplayName != name || u.Phone == nil || *u.Phone != phone || u.Version != 2 || !u.UpdatedAt.Equal(now) {
		t.Fatalf("UpdateProfile = %+v, %v", u, err)
	}
	u, err = repo.UpdateProfile(ctx, ana.ID, usecase.ProfileChanges{ClearPhone: true}, now)
	if err != nil || u.Phone != nil || u.DisplayName != name || u.Version != 3 {
		t.Fatalf("borrar teléfono = %+v, %v", u, err)
	}
	for _, id := range []string{"no-es-id", "0123456789abcdef01234567"} {
		if _, err := repo.UpdateProfile(ctx, id, usecase.ProfileChanges{DisplayName: &name}, now); !domain.Is(err, domain.ErrUserNotFound) {
			t.Errorf("UpdateProfile(%s) = %v", id, err)
		}
	}

	// Calificaciones y suspensión (users.Admin).
	for _, s := range []int{5, 4} {
		if err := repo.AddRating(ctx, ana.ID, s); err != nil {
			t.Fatalf("AddRating: %v", err)
		}
	}
	if err := repo.AddRating(ctx, ana.ID, 6); err == nil {
		t.Error("AddRating(6) debe fallar")
	}
	if err := repo.AddRating(ctx, "0123456789abcdef01234567", 5); !domain.Is(err, domain.ErrUserNotFound) {
		t.Errorf("AddRating inexistente = %v", err)
	}
	until := now.Add(24 * time.Hour)
	if err := repo.Suspend(ctx, ana.ID, until); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if err := repo.Suspend(ctx, "no-es-id", until); !domain.Is(err, domain.ErrUserNotFound) {
		t.Errorf("Suspend id inválido = %v", err)
	}
	got, _ := repo.ByID(ctx, ana.ID)
	if got.RatingSum != 9 || got.RatingCount != 2 || !got.IsSuspended(now) {
		t.Errorf("tras calificar y suspender = %+v", got)
	}

	// Baja: anonimiza y libera el correo.
	anon := domain.AnonymizedEmail(beto.ID)
	if err := repo.Deactivate(ctx, beto.ID, anon, now); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if err := repo.Deactivate(ctx, "0123456789abcdef01234567", "x@vaiven.invalid", now); !domain.Is(err, domain.ErrUserNotFound) {
		t.Errorf("Deactivate inexistente = %v", err)
	}
	if err := repo.Deactivate(ctx, "no-es-id", "x@vaiven.invalid", now); !domain.Is(err, domain.ErrUserNotFound) {
		t.Errorf("Deactivate id inválido = %v", err)
	}
	b, _ := repo.ByID(ctx, beto.ID)
	if b.Email != anon || b.DisplayName != domain.DeletedDisplayName || b.Phone != nil || b.IsActive() || b.DeactivatedAt == nil {
		t.Errorf("tras la baja = %+v", b)
	}
	if _, err := repo.UpdateProfile(ctx, beto.ID, usecase.ProfileChanges{DisplayName: &name}, now); !domain.Is(err, domain.ErrUserNotFound) {
		t.Errorf("editar una cuenta de baja = %v", err)
	}
	if _, err := repo.Create(ctx, newUser("beto@example.com")); err != nil {
		t.Errorf("el correo original debe quedar libre: %v", err)
	}

	// Directorio por lote (users.Directory).
	dir, err := repo.PublicUsers(ctx, []string{ana.ID, beto.ID, "no-es-id", "0123456789abcdef01234567"})
	if err != nil {
		t.Fatalf("PublicUsers: %v", err)
	}
	if len(dir) != 2 || dir[ana.ID].DisplayName != name || dir[ana.ID].RatingAverage == nil ||
		*dir[ana.ID].RatingAverage != 4.5 || dir[beto.ID].DisplayName != domain.DeletedDisplayName {
		t.Errorf("PublicUsers = %+v", dir)
	}
	if empty, err := repo.PublicUsers(ctx, []string{"x"}); err != nil || len(empty) != 0 {
		t.Errorf("PublicUsers sin ids válidos = %v, %v", empty, err)
	}
}
