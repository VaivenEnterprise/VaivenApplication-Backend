package usecase

import (
	"cmp"
	"context"
	"slices"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain/model"
	searchusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/search/usecase"
)

// poolFactor: se piden tarjetas para limit·poolFactor candidatos (hasta
// searchusecase.MaxByIDs), porque algunos no son visibles y la diversidad
// descarta otros.
const poolFactor = 5

// Recommend devuelve hasta limit objetos para userID (RF-04 y RF-05):
//
//   - Con al menos domain.MinInteractions objetos en su historial, el híbrido
//     sobre sus vecinos; después, el arranque en frío para completar.
//   - Con menos, solo el arranque en frío: populares del municipio, de las
//     categorías vistas y generales.
//
// Nunca recomienda objetos del historial ni los que la persona tiene en una
// reserva activa; las tarjetas salen de la búsqueda, que ya excluye objetos
// propios, no activos y de cuentas dadas de baja o suspendidas (CA-03). Máximo
// domain.MaxPerCategory por categoría. Si el modelo falta o venció, lanza el
// recálculo aparte y responde con el que hay (RF-06, CA-08). Nunca devuelve nil.
func (s *Service) Recommend(ctx context.Context, userID, rawLimit, rawMunicipality string) ([]Recommendation, error) {
	if !s.d.Enabled {
		return nil, domain.ErrDisabled
	}
	limit, err := domain.ParseLimit(rawLimit)
	if err != nil {
		return nil, err
	}
	municipality, err := domain.CleanMunicipality(rawMunicipality)
	if err != nil {
		return nil, err
	}
	buildID, hasModel, err := s.currentModel(ctx)
	if err != nil {
		return nil, err
	}
	history, err := s.history(ctx, userID)
	if err != nil {
		return nil, err
	}
	historyIDs := distinctItems(history)
	briefs, err := s.d.Items.Briefs(ctx, historyIDs)
	if err != nil {
		return nil, err
	}
	active, err := s.d.Requests.ActiveItemIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	exclude := map[string]bool{}
	for _, id := range active {
		exclude[id] = true
	}

	var cands []model.Candidate
	if hasModel && len(historyIDs) >= domain.MinInteractions {
		neighbors, err := s.d.Model.Neighbors(ctx, buildID, historyIDs)
		if err != nil {
			return nil, err
		}
		cands = model.Rank(history, neighbors, exclude)
	}
	for _, id := range historyIDs {
		exclude[id] = true
	}
	cats := viewedCategories(historyIDs, briefs)
	popular, err := s.popular(ctx, buildID, hasModel, municipality, cats)
	if err != nil {
		return nil, err
	}
	cands = appendUnique(cands, model.ColdStart(popular, municipality, cats, exclude))
	return s.toRecommendations(ctx, userID, cands, briefs, limit)
}

// currentModel devuelve el modelo vigente y, si falta o venció, lanza el
// recálculo sin esperarlo.
func (s *Service) currentModel(ctx context.Context) (string, bool, error) {
	buildID, builtAt, ok, err := s.d.Meta.Current(ctx)
	if err != nil {
		return "", false, err
	}
	if !ok || s.d.Now().Sub(builtAt) > domain.ModelMaxAge {
		s.d.Go(s.recomputeInBackground)
	}
	return buildID, ok, nil
}

// history son las interacciones más recientes de userID: sus vistas y sus
// solicitudes posteriores al último borrado de historial (P-09).
func (s *Service) history(ctx context.Context, userID string) ([]domain.Event, error) {
	resetAt, _, err := s.d.Meta.ResetAt(ctx, userID)
	if err != nil {
		return nil, err
	}
	views, err := s.d.Interactions.Recent(ctx, userID, domain.HistorySize)
	if err != nil {
		return nil, err
	}
	requests, err := s.d.Requests.Recent(ctx, userID, resetAt, domain.HistorySize)
	if err != nil {
		return nil, err
	}
	h := slices.Concat(views, requests)
	slices.SortFunc(h, func(a, b domain.Event) int {
		if c := b.At.Compare(a.At); c != 0 {
			return c
		}
		return cmp.Compare(a.ItemID, b.ItemID)
	})
	if len(h) > domain.HistorySize {
		h = h[:domain.HistorySize]
	}
	return h, nil
}

// popular lee las listas de populares del modelo. Sin modelo todavía (solo
// antes del primer recálculo), las calcula al momento con los objetos activos
// para que el arranque en frío no quede vacío (CA-02).
func (s *Service) popular(ctx context.Context, buildID string, hasModel bool, municipality string, cats []string) (map[string][]string, error) {
	if !hasModel {
		feats, err := s.d.Items.ActiveFeatures(ctx)
		if err != nil {
			return nil, err
		}
		return model.Popular(nil, feats, domain.PopularSize), nil
	}
	keys := []string{domain.PopularAll}
	if municipality != "" {
		keys = append(keys, domain.PopularMunicipality(municipality))
	}
	for _, c := range cats {
		keys = append(keys, domain.PopularCategory(c))
	}
	return s.d.Model.Popular(ctx, buildID, keys)
}

// toRecommendations pide las tarjetas de los primeros candidatos (la búsqueda
// descarta los no visibles), aplica la diversidad y arma el motivo.
func (s *Service) toRecommendations(ctx context.Context, userID string, cands []model.Candidate, briefs map[string]domain.ItemBrief, limit int) ([]Recommendation, error) {
	poolSize := min(len(cands), limit*poolFactor, searchusecase.MaxByIDs)
	ids := make([]string, poolSize)
	for i := range ids {
		ids[i] = cands[i].ItemID
	}
	out := []Recommendation{}
	if len(ids) == 0 {
		return out, nil
	}
	cards, err := s.d.Cards.Cards(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]searchusecase.CardView, len(cards))
	category := make(map[string]string, len(cards))
	for _, c := range cards {
		byID[c.Card.ID] = c
		category[c.Card.ID] = c.Card.Category
	}
	for _, c := range model.Diversify(cands[:poolSize], category, domain.MaxPerCategory, limit) {
		reason := c.Reason
		if reason.Code == domain.ReasonSimilarTo {
			reason = domain.SimilarTo(reason.ItemID, briefs[reason.ItemID].Title)
		}
		out = append(out, Recommendation{Card: byID[c.ItemID], Reason: reason})
	}
	return out, nil
}

// distinctItems devuelve los objetos del historial sin repetir, del más
// reciente al más viejo.
func distinctItems(history []domain.Event) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range history {
		if !seen[e.ItemID] {
			seen[e.ItemID] = true
			out = append(out, e.ItemID)
		}
	}
	return out
}

// viewedCategories son las categorías del historial, sin repetir, de la más
// reciente a la más vieja.
func viewedCategories(historyIDs []string, briefs map[string]domain.ItemBrief) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range historyIDs {
		if c := briefs[id].Category; c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func appendUnique(a, b []model.Candidate) []model.Candidate {
	seen := make(map[string]bool, len(a))
	for _, c := range a {
		seen[c.ItemID] = true
	}
	for _, c := range b {
		if !seen[c.ItemID] {
			seen[c.ItemID] = true
			a = append(a, c)
		}
	}
	return a
}
