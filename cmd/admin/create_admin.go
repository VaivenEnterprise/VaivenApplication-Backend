package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
)

// adminDisplayName es el nombre visible de las cuentas de administración.
const adminDisplayName = "Administración VAIVÉN"

// createAdminTimeout limita la duración total de create-admin.
const createAdminTimeout = 60 * time.Second

// adminStore guarda la cuenta. *authmongo.UserRepo lo cumple.
type adminStore interface {
	Create(ctx context.Context, u domain.User) (domain.User, error)
}

// passwordHasher calcula el hash. *security.Hasher lo cumple.
type passwordHasher interface {
	Hash(ctx context.Context, password string) (string, error)
}

// adminDeps son las dependencias de create-admin; close libera la conexión.
type adminDeps struct {
	store  adminStore
	hasher passwordHasher
	close  func()
}

// adminFactory conecta con MongoDB a partir de la configuración.
type adminFactory func(ctx context.Context, cfg config.Config) (adminDeps, error)

// mongoAdminDeps es la adminFactory real: MongoDB, índices y argon2id.
func mongoAdminDeps(ctx context.Context, cfg config.Config) (adminDeps, error) {
	mc, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoConnectTimeout)
	if err != nil {
		return adminDeps{}, fmt.Errorf("conectar a MongoDB: %w", err)
	}
	db := mc.Database(cfg.MongoDB)
	if _, err := authmongo.EnsureSchemas(ctx, db); err != nil {
		_ = mc.Disconnect(context.Background())
		return adminDeps{}, err
	}
	repo := authmongo.NewUserRepo(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		_ = mc.Disconnect(context.Background())
		return adminDeps{}, err
	}
	hasher, err := security.NewHasher(security.DefaultMemoryBudgetKiB, security.DefaultMaxWait)
	if err != nil {
		_ = mc.Disconnect(context.Background())
		return adminDeps{}, err
	}
	return adminDeps{
		store:  repo,
		hasher: hasher,
		close:  func() { _ = mc.Disconnect(context.Background()) },
	}, nil
}

// createAdmin lee el correo y la contraseña de la entrada estándar (una línea
// cada uno; la contraseña nunca va como argumento, RF-10) y crea la cuenta con
// rol admin. Si el correo ya tiene cuenta no la toca y sale con 1.
//
//	printf '%s\n%s\n' "$CORREO" "$CONTRASENA" | go run ./cmd/admin create-admin
func createAdmin(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, d adminDeps, now time.Time) int {
	sc := bufio.NewScanner(stdin)
	_, _ = fmt.Fprint(stderr, "Correo: ")
	email := readLine(sc)
	_, _ = fmt.Fprint(stderr, "Contraseña: ")
	password := strings.TrimRight(readLine(sc), "\r")
	_, _ = fmt.Fprintln(stderr)

	email = domain.NormalizeEmail(email)
	problems := append(domain.ValidateEmail(email), domain.ValidatePassword(password)...)
	if len(problems) > 0 {
		for _, p := range problems {
			_, _ = fmt.Fprintf(stderr, "error: %s %s\n", p.Field, p.Issue)
		}
		return 1
	}

	hash, err := d.hasher.Hash(ctx, password)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	now = now.UTC().Truncate(time.Millisecond)
	u, err := d.store.Create(ctx, domain.User{
		Email:           email,
		PasswordHash:    hash,
		DisplayName:     adminDisplayName,
		Role:            domain.RoleAdmin,
		Status:          domain.StatusActive,
		AcceptedTermsAt: now,
		CreatedAt:       now,
		UpdatedAt:       now,
		Version:         1,
	})
	if err != nil {
		if domain.Is(err, domain.ErrEmailTaken) {
			_, _ = fmt.Fprintln(stderr, "Ya existe una cuenta con ese correo; no se modificó.")
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Administrador creado (id %s).\n", u.ID)
	return 0
}

func readLine(sc *bufio.Scanner) string {
	if sc.Scan() {
		return sc.Text()
	}
	return ""
}
