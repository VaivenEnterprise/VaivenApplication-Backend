package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/mongodb"
	recomongo "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/adapter/mongo"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	recousecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/usecase"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

// recoTimeout limita los subcomandos de recomendaciones (spec 015, T008).
const recoTimeout = 5 * time.Minute

// recoDeps son las dependencias de los subcomandos de recomendaciones; close
// cierra la conexión.
type recoDeps struct {
	recompute interface {
		Recompute(ctx context.Context) (recousecase.RecomputeResult, error)
	}
	demo interface {
		InsertDemo(ctx context.Context, events []domain.Event) (int, error)
		PurgeDemo(ctx context.Context) (int64, error)
	}
	items interface {
		ActiveFeatures(ctx context.Context) ([]domain.ItemFeatures, error)
	}
	views    eventSource
	requests eventSource
	now      func() time.Time
	close    func()
}

type eventSource interface {
	Since(ctx context.Context, since time.Time, limit int) ([]domain.Event, error)
}

// recoFactory arma las dependencias contra la base del entorno.
type recoFactory func(ctx context.Context, cfg config.Config) (recoDeps, error)

// noCards: el recálculo no pide tarjetas; así el comando no necesita
// Cloudinary.
type noCards struct{}

func (noCards) Cards(context.Context, string, []string) ([]searchusecase.CardView, error) {
	return []searchusecase.CardView{}, nil
}

func mongoRecoDeps(ctx context.Context, cfg config.Config) (recoDeps, error) {
	mc, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoConnectTimeout)
	if err != nil {
		return recoDeps{}, fmt.Errorf("conectar a MongoDB: %w", err)
	}
	db := mc.Database(cfg.MongoDB)
	interactions := recomongo.NewInteractionRepo(db)
	requests := recomongo.NewRequestSource(db)
	items := recomongo.NewItemSource(db)
	svc, err := recousecase.NewService(recousecase.Deps{
		Interactions: interactions, Requests: requests, Items: items,
		Model: recomongo.NewModelRepo(db), Meta: recomongo.NewMetaRepo(db), Cards: noCards{},
		Log: slog.New(slog.NewTextHandler(os.Stderr, nil)), Enabled: cfg.Reco.Enabled,
	})
	if err != nil {
		_ = mc.Disconnect(context.Background())
		return recoDeps{}, err
	}
	return recoDeps{
		recompute: svc, demo: interactions, items: items, views: interactions, requests: requests,
		now: time.Now, close: func() { _ = mc.Disconnect(context.Background()) },
	}, nil
}
