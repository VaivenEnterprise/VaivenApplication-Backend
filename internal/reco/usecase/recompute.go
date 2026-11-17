package usecase

import (
	"context"
	"fmt"
	"slices"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain/model"
)

// RecomputeResult resume un recálculo. No lleva identificadores de personas
// ni de objetos (RF-09): solo conteos.
type RecomputeResult struct {
	Built        bool // false si otro proceso tenía el candado
	BuildID      string
	Interactions int
	Items        int
	Purged       int // cuentas dadas de baja cuyos eventos se borraron
}

// Recompute recalcula el modelo completo (RF-02 a RF-06) si consigue el
// candado; si otro proceso lo tiene, devuelve Built = false sin error (CA-08).
// El modelo nuevo se escribe junto al anterior y se vuelve vigente en una
// sola escritura; si el candado venció mientras tanto, no se publica.
func (s *Service) Recompute(ctx context.Context) (RecomputeResult, error) {
	owner := s.d.NewID()
	got, err := s.d.Meta.TryLock(ctx, owner, s.d.Now())
	if err != nil || !got {
		return RecomputeResult{}, err
	}
	res, err := s.build(ctx, owner)
	if err != nil {
		// Soltar el candado no debe esperar a un contexto ya vencido.
		_ = s.d.Meta.Unlock(context.WithoutCancel(ctx), owner)
		return RecomputeResult{}, err
	}
	return res, nil
}

func (s *Service) build(ctx context.Context, owner string) (RecomputeResult, error) {
	now := s.d.Now()
	since := now.Add(-domain.Window)
	views, err := s.d.Interactions.Since(ctx, since, domain.MaxInteractionsRead)
	if err != nil {
		return RecomputeResult{}, err
	}
	requests, err := s.d.Requests.Since(ctx, since, domain.MaxInteractionsRead)
	if err != nil {
		return RecomputeResult{}, err
	}
	resets, err := s.d.Meta.Resets(ctx)
	if err != nil {
		return RecomputeResult{}, err
	}
	// Solicitudes anteriores a un borrado de historial no cuentan (P-09).
	kept := requests[:0:0]
	for _, r := range requests {
		if at, ok := resets[r.UserID]; !ok || r.At.After(at) {
			kept = append(kept, r)
		}
	}
	events := slices.Concat(views, kept)

	gone, err := s.d.Items.Deactivated(ctx, distinctUsers(events))
	if err != nil {
		return RecomputeResult{}, err
	}
	if err := s.d.Interactions.DeleteUsers(ctx, gone); err != nil {
		return RecomputeResult{}, err
	}
	if len(gone) > 0 {
		events = withoutUsers(events, gone)
	}

	items, err := s.d.Items.ActiveFeatures(ctx)
	if err != nil {
		return RecomputeResult{}, err
	}
	neighbors, popular := Build(events, items)

	buildID := s.d.NewID()
	if err := s.d.Model.Save(ctx, buildID, neighbors, popular, now); err != nil {
		return RecomputeResult{}, err
	}
	published, err := s.d.Meta.Publish(ctx, owner, buildID, now)
	if err != nil {
		return RecomputeResult{}, err
	}
	if !published {
		return RecomputeResult{}, fmt.Errorf("reco: el candado venció antes de publicar el modelo")
	}
	if err := s.d.Model.DeleteOtherBuilds(ctx, buildID); err != nil {
		// El modelo nuevo ya es el vigente; el anterior se borra en el próximo recálculo.
		s.d.Log.Warn("reco: no se pudo borrar el modelo anterior", "error", err)
	}
	return RecomputeResult{Built: true, BuildID: buildID, Interactions: len(events), Items: len(items), Purged: len(gone)}, nil
}

// Build calcula vecinos y populares a partir de interacciones y objetos
// activos (RF-02 a RF-04). Es puro: lo usan el recálculo y la evaluación.
func Build(events []domain.Event, items []domain.ItemFeatures) (map[string][]domain.Neighbor, map[string][]string) {
	feats := make(map[string]domain.ItemFeatures, len(items))
	for _, it := range items {
		feats[it.ID] = it
	}
	cf := model.CollaborativeNeighbors(model.BuildMatrix(events), domain.Neighbors)
	content := model.ContentNeighbors(items, domain.Neighbors)
	return model.MergeNeighbors(cf, content, feats, domain.Neighbors), model.Popular(events, items, domain.PopularSize)
}

// recomputeInBackground es el recálculo perezoso (RF-06): corre aparte de la
// petición, con su propio tiempo máximo, y solo registra el resultado.
func (s *Service) recomputeInBackground() {
	ctx, cancel := context.WithTimeout(context.Background(), domain.RecomputeTimeout)
	defer cancel()
	res, err := s.Recompute(ctx)
	switch {
	case err != nil:
		s.d.Log.Warn("reco: el recálculo falló", "error", err)
	case res.Built:
		s.d.Log.Info("reco: modelo recalculado", "interactions", res.Interactions, "items", res.Items, "purged", res.Purged)
	}
}

func distinctUsers(events []domain.Event) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range events {
		if !seen[e.UserID] {
			seen[e.UserID] = true
			out = append(out, e.UserID)
		}
	}
	return out
}

func withoutUsers(events []domain.Event, users []string) []domain.Event {
	drop := make(map[string]bool, len(users))
	for _, u := range users {
		drop[u] = true
	}
	out := events[:0:0]
	for _, e := range events {
		if !drop[e.UserID] {
			out = append(out, e)
		}
	}
	return out
}
