package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	drv "go.mongodb.org/mongo-driver/v2/mongo"

	adaptermongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/mongo/catalog"
	authmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/mongo"
	authusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	bookingsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/adapter/mongo"
	bookingsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	bookingsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	chatmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/adapter/mongo"
	chatusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/chat/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
	itemsmongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/adapter/mongo"
	itemsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/notify"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
)

// mongoSeedDeps es la seedFactory real: MongoDB, los casos de uso de auth,
// objetos, chat y reservas, e imágenes de Cloudinary (o locales con --scale).
// Los avisos van a notify.Noop: los datos de demostración no generan avisos
// ni push.
func mongoSeedDeps(ctx context.Context, cfg config.Config, local bool) (seedDeps, error) {
	mc, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoConnectTimeout)
	if err != nil {
		return seedDeps{}, fmt.Errorf("conectar a MongoDB: %w", err)
	}
	closeFn := func() { _ = mc.Disconnect(context.Background()) }
	d, err := buildSeedDeps(ctx, cfg, mc.Database(cfg.MongoDB), local)
	if err != nil {
		closeFn()
		return seedDeps{}, err
	}
	d.close = closeFn
	return d, nil
}

// buildSeedDeps arma los casos de uso sobre db (separado para las pruebas de
// integración, que inyectan su propia base e imágenes).
func buildSeedDeps(ctx context.Context, cfg config.Config, db *drv.Database, local bool) (seedDeps, error) {
	var imgs imgport.Service
	var photos photoStore
	if local {
		imgs, photos = localImages{baseFolder: cfg.Cloudinary.BaseFolder}, localPhotos{baseFolder: cfg.Cloudinary.BaseFolder}
	} else {
		svc, err := images.New(images.Config{
			CloudName: cfg.Cloudinary.CloudName, APIKey: cfg.Cloudinary.APIKey,
			APISecret: cfg.Cloudinary.APISecret.Reveal(), BaseFolder: cfg.Cloudinary.BaseFolder,
			MaxBytes: cfg.Images.MaxBytes,
		})
		if err != nil {
			return seedDeps{}, fmt.Errorf("configurar Cloudinary: %w", err)
		}
		imgs = svc
		photos = newCloudinaryPhotos(svc, cfg.Cloudinary.CloudName, cfg.Cloudinary.APIKey, cfg.Cloudinary.APISecret.Reveal())
	}
	return seedDepsOn(ctx, cfg, db, imgs, photos)
}

// seedDepsOn arma los casos de uso con imágenes y fotos dadas.
func seedDepsOn(ctx context.Context, cfg config.Config, db *drv.Database, imgs imgport.Service, photos photoStore) (seedDeps, error) {
	if err := prepareSchema(ctx, db); err != nil {
		return seedDeps{}, err
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	userRepo := authmongo.NewUserRepo(db)
	hasher, err := security.NewHasher(security.DefaultMemoryBudgetKiB, security.DefaultMaxWait)
	if err != nil {
		return seedDeps{}, err
	}
	jwt, err := security.NewJWT(cfg.Auth.AccessSecret.Reveal(), cfg.Auth.AccessTTL)
	if err != nil {
		return seedDeps{}, fmt.Errorf("configurar JWT: %w", err)
	}
	authSvc, err := authusecase.NewService(authusecase.Deps{
		Users: userRepo, Refresh: authmongo.NewRefreshRepo(db), Hasher: hasher, Access: jwt,
		NewRefresh: security.NewRefreshToken, HashRefresh: security.HashToken, RefreshTTL: cfg.Auth.RefreshTTL,
		Bookings: bookingsmongo.NewUserActivity(db),
	})
	if err != nil {
		return seedDeps{}, err
	}
	mod := moderation.NewFilter()
	itemsSvc, err := itemsusecase.NewService(itemsusecase.Deps{
		Items: itemsmongo.NewItemRepo(db), Images: imgs, Moderation: mod, Users: userRepo,
		Bookings: bookingsmongo.NewItemActivity(db),
	})
	if err != nil {
		return seedDeps{}, err
	}
	chatSvc, err := chatusecase.NewService(chatusecase.Deps{
		Conversations: chatmongo.NewConversationRepo(db), Messages: chatmongo.NewMessageRepo(db),
		Sender: chatmongo.NewSender(db), Bookings: chatmongo.NewBookingReader(db), Accounts: chatmongo.NewAccountReader(db),
		Users: userRepo, Images: imgs, Moderation: mod, Notifier: notify.Noop{}, Log: log,
	})
	if err != nil {
		return seedDeps{}, err
	}
	bookingsSvc, err := bookingsusecase.NewService(bookingsusecase.Deps{
		Repo: bookingsmongo.NewRepo(db), Images: imgs, Users: userRepo, Chat: chatSvc, Notifier: notify.Noop{}, Log: log,
	})
	if err != nil {
		return seedDeps{}, err
	}
	loc, err := bookingsdomain.Location()
	if err != nil {
		return seedDeps{}, err
	}
	return seedDeps{
		auth: authSvc, users: userRepo, admins: adminDeps{store: userRepo, hasher: hasher},
		items: itemsSvc, bookings: bookingsSvc, photos: photos,
		today: func() bookingsdomain.Date { return bookingsdomain.Today(time.Now(), loc) },
		now:   time.Now,
		close: func() {},
	}, nil
}

// prepareSchema aplica la validación $jsonSchema y los índices del catálogo,
// como al arrancar la API (idempotente).
func prepareSchema(ctx context.Context, db *drv.Database) error {
	if _, err := authmongo.EnsureSchemas(ctx, db); err != nil {
		return fmt.Errorf("validación de users: %w", err)
	}
	if _, err := itemsmongo.EnsureSchema(ctx, db); err != nil {
		return fmt.Errorf("validación de items: %w", err)
	}
	byCol := map[string][]adaptermongo.IndexSpec{}
	var order []string
	for _, s := range catalog.All() {
		if _, ok := byCol[s.Collection]; !ok {
			order = append(order, s.Collection)
		}
		byCol[s.Collection] = append(byCol[s.Collection], s)
	}
	var errs []error
	for _, col := range order {
		if _, err := db.Collection(col).Indexes().CreateMany(ctx, adaptermongo.Models(byCol[col])); err != nil {
			errs = append(errs, fmt.Errorf("índices de %s: %w", col, err))
		}
	}
	return errors.Join(errs...)
}
