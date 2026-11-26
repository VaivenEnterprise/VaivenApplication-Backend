package usecase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reco/domain/synth"
)

var ctx = context.Background()

func ids(rs []Recommendation) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Card.Card.ID
	}
	return out
}

func codeOf(err error) shared.Code {
	var de *shared.Error
	if errors.As(err, &de) {
		return de.Code
	}
	return ""
}

func TestNewService(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("sin dependencias debe fallar")
	}
	s := newStore()
	svc, err := NewService(Deps{Interactions: s, Requests: requests{s}, Items: s, Model: s, Meta: meta{s}, Cards: cards{s},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil || svc.d.Now == nil || svc.d.Go == nil || len(svc.d.NewID()) != 32 || svc.Enabled() {
		t.Fatalf("valores por defecto: %v", err)
	}
	done := make(chan struct{})
	svc.d.Go(func() { close(done) })
	<-done
}

func TestDisabledEverywhere(t *testing.T) {
	h := newHarness(t)
	h.svc.d.Enabled = false
	_, recErr := h.svc.Recommend(ctx, "u", "", "")
	for _, err := range []error{h.svc.RecordView(ctx, "u", oid(1), "view"), recErr, h.svc.DeleteHistory(ctx, "u")} {
		if !errors.Is(err, domain.ErrDisabled) {
			t.Errorf("apagado debe dar ErrDisabled: %v", err)
		}
	}
}

func TestRecordView(t *testing.T) {
	h := newHarness(t)
	h.addItem(oid(1), "otro", "active", "hogar", "Tula")
	h.addItem(oid(2), "u", "active", "hogar", "Tula")
	h.addItem(oid(3), "otro", "paused", "hogar", "Tula")

	if err := h.svc.RecordView(ctx, "u", "x", "click"); codeOf(err) != shared.CodeValidation {
		t.Errorf("inválido: %v", err)
	}
	if err := h.svc.RecordView(ctx, "u", oid(9), "view"); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	for _, id := range []string{oid(2), oid(3)} { // propio y pausado: se ignoran sin error
		if err := h.svc.RecordView(ctx, "u", id, "view"); err != nil {
			t.Errorf("ignorado: %v", err)
		}
	}
	if len(h.views) != 0 {
		t.Fatalf("no debía guardar nada: %v", h.views)
	}
	for range 2 { // la segunda vista en el mismo bloque no cuenta (CA-09)
		if err := h.svc.RecordView(ctx, "u", oid(1), "view"); err != nil {
			t.Fatal(err)
		}
	}
	if len(h.views) != 1 || h.views[0].Weight != domain.WeightView {
		t.Errorf("vistas = %+v", h.views)
	}
	for _, op := range []string{"ItemState", "Record"} {
		h.fail[op] = true
		if err := h.svc.RecordView(ctx, "u", oid(1), "view"); !errors.Is(err, errBoom) {
			t.Errorf("%s: %v", op, err)
		}
		h.fail[op] = false
	}
}

// CA-01: quien ve herramientas eléctricas recibe sobre todo esa categoría, con
// objetos vistos por personas parecidas, y el motivo dice por qué.
func TestRecommendHybridCA01(t *testing.T) {
	h := newHarness(t)
	for i, cat := range []string{"tools", "tools", "tools", "tools", "tools", "hogar"} {
		h.addItem(oid(i+1), "dueño", "active", cat, "Tula")
	}
	h.views = []domain.Event{
		{UserID: "u", ItemID: oid(1), Type: domain.EventView, Weight: 1, At: now0.Add(-2 * time.Hour)},
		{UserID: "u", ItemID: oid(2), Type: domain.EventView, Weight: 1, At: now0.Add(-time.Hour)},
	}
	h.publish(map[string][]domain.Neighbor{
		oid(1): {{ItemID: oid(3), CF: 0.9, Content: 0.75}, {ItemID: oid(2), CF: 1}},
		oid(2): {{ItemID: oid(4), CF: 0.5, Content: 0.75}, {ItemID: oid(5), CF: 0.1, Content: 0.75}},
	}, map[string][]string{domain.PopularAll: {oid(6), oid(5)}})

	got, err := h.svc.Recommend(ctx, "u", "3", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{oid(3), oid(4), oid(5)}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("recomendados = %v, want %v", ids(got), want)
	}
	if r := got[0].Reason; r.Code != domain.ReasonSimilarTo || r.ItemID != oid(1) || r.ItemTitle != "Título "+oid(1) {
		t.Errorf("motivo = %+v", r)
	}
	if h.launched != 0 {
		t.Error("con el modelo vigente no debe recalcular")
	}
}

// CA-02: sin historial, populares del municipio y luego generales; nunca vacío.
func TestRecommendColdStartCA02(t *testing.T) {
	h := newHarness(t)
	for i := 1; i <= 4; i++ {
		h.addItem(oid(i), "dueño", "active", "hogar", "Tula")
	}
	h.addItem(oid(5), "dueño", "active", "deportes", "Tula")
	h.publish(nil, map[string][]string{"m:tula": {oid(1), oid(2)}, domain.PopularAll: {oid(5), oid(1), oid(3)}})

	got, err := h.svc.Recommend(ctx, "nueva", "", " TULA ")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{oid(1), oid(2), oid(5), oid(3)}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("arranque en frío = %v, want %v", ids(got), want)
	}
	if got[0].Reason.Code != domain.ReasonPopularNearby || got[2].Reason.Code != domain.ReasonPopular {
		t.Errorf("motivos = %+v", got)
	}
	// Con una sola interacción sigue en frío y usa la categoría vista.
	h.views = []domain.Event{{UserID: "u1", ItemID: oid(5), Weight: 1, At: now0}}
	h.popular["b0"]["c:deportes"] = []string{oid(4)}
	got, _ = h.svc.Recommend(ctx, "u1", "", "")
	if len(got) == 0 || got[0].Card.Card.ID != oid(4) || got[0].Reason.Code != domain.ReasonPopularInCategory {
		t.Errorf("categoría vista = %v", got)
	}
}

// CA-03: nunca propios, no visibles, del historial ni en reserva activa; máximo
// 3 por categoría.
func TestRecommendExclusionsCA03(t *testing.T) {
	h := newHarness(t)
	for i := 1; i <= 9; i++ {
		h.addItem(oid(i), "dueño", "active", "tools", "Tula")
	}
	h.addItem(oid(10), "u", "active", "tools", "Tula")     // propio
	h.addItem(oid(11), "dueño", "paused", "tools", "Tula") // pausado
	h.addItem(oid(12), "dueño", "active", "hogar", "Tula")
	h.hidden[oid(3)] = true          // p. ej. dueño suspendido
	h.active["u"] = []string{oid(4)} // reserva activa
	h.views = []domain.Event{{UserID: "u", ItemID: oid(1), Weight: 1, At: now0}}
	h.requests = []domain.Event{{UserID: "u", ItemID: oid(2), Type: domain.EventRequest, Weight: 3, At: now0}}
	all := []string{oid(10), oid(11), oid(1), oid(2), oid(3), oid(4), oid(5), oid(6), oid(7), oid(8), oid(12)}
	h.publish(map[string][]domain.Neighbor{oid(1): {{ItemID: oid(5), CF: 1}}}, map[string][]string{domain.PopularAll: all})

	got, err := h.svc.Recommend(ctx, "u", "10", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{oid(5), oid(6), oid(7), oid(12)}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("recomendados = %v, want %v", ids(got), want)
	}
}

// CA-05: tras borrar el historial, las vistas desaparecen y las solicitudes
// anteriores no cuentan: vuelve al arranque en frío.
func TestDeleteHistoryCA05(t *testing.T) {
	h := newHarness(t)
	for i := 1; i <= 4; i++ {
		h.addItem(oid(i), "dueño", "active", "tools", "Tula")
	}
	h.views = []domain.Event{{UserID: "u", ItemID: oid(1), Weight: 1, At: now0.Add(-time.Hour)}, {UserID: "otra", ItemID: oid(1), Weight: 1, At: now0}}
	h.requests = []domain.Event{{UserID: "u", ItemID: oid(2), Type: domain.EventRequest, Weight: 3, At: now0.Add(-time.Hour)}}
	h.publish(map[string][]domain.Neighbor{oid(1): {{ItemID: oid(3), CF: 1}}, oid(2): {{ItemID: oid(3), CF: 1}}},
		map[string][]string{domain.PopularAll: {oid(4), oid(3)}})
	if got, _ := h.svc.Recommend(ctx, "u", "1", ""); got[0].Reason.Code != domain.ReasonSimilarTo {
		t.Fatalf("antes del borrado = %+v", got)
	}
	if err := h.svc.DeleteHistory(ctx, "u"); err != nil {
		t.Fatal(err)
	}
	if len(h.views) != 1 || h.views[0].UserID != "otra" || !h.resets["u"].Equal(now0) {
		t.Fatalf("borrado: %v %v", h.views, h.resets)
	}
	got, err := h.svc.Recommend(ctx, "u", "1", "")
	if err != nil || got[0].Card.Card.ID != oid(4) || got[0].Reason.Code != domain.ReasonPopular {
		t.Errorf("después del borrado = %+v %v", got, err)
	}
	for _, op := range []string{"DeleteUser", "SetReset"} {
		h.fail[op] = true
		if err := h.svc.DeleteHistory(ctx, "u"); !errors.Is(err, errBoom) {
			t.Errorf("%s: %v", op, err)
		}
		h.fail[op] = false
	}
}

func TestRecommendWithoutModelAndStale(t *testing.T) {
	h := newHarness(t)
	h.addItem(oid(1), "dueño", "active", "hogar", "Tula")
	got, err := h.svc.Recommend(ctx, "u", "", "")
	if err != nil || len(got) != 1 || h.launched != 1 {
		t.Fatalf("sin modelo: %v %v, recálculos %d", got, err, h.launched)
	}
	h.publish(nil, map[string][]string{domain.PopularAll: {oid(1)}})
	h.now = now0.Add(domain.ModelMaxAge + time.Minute)
	if _, err := h.svc.Recommend(ctx, "u", "", ""); err != nil || h.launched != 2 {
		t.Errorf("modelo vencido: %v, recálculos %d", err, h.launched)
	}
	h.popular["b0"] = map[string][]string{}
	if got, err := h.svc.Recommend(ctx, "u", "", ""); err != nil || got == nil || len(got) != 0 {
		t.Errorf("sin candidatos debe ser [] y no nil: %v %v", got, err)
	}
}

func TestRecommendValidationAndErrors(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Recommend(ctx, "u", "0", ""); codeOf(err) != shared.CodeValidation {
		t.Errorf("limit: %v", err)
	}
	if _, err := h.svc.Recommend(ctx, "u", "", "x"); codeOf(err) != shared.CodeValidation {
		t.Errorf("municipio: %v", err)
	}
	h.addItem(oid(1), "dueño", "active", "hogar", "Tula")
	h.addItem(oid(2), "dueño", "active", "hogar", "Tula")
	h.addItem(oid(3), "dueño", "active", "hogar", "Tula")
	h.views = []domain.Event{{UserID: "u", ItemID: oid(1), Weight: 1, At: now0}, {UserID: "u", ItemID: oid(2), Weight: 1, At: now0}}
	h.publish(nil, map[string][]string{domain.PopularAll: {oid(1), oid(3)}})
	for _, op := range []string{"Current", "ResetAt", "InteractionsRecent", "RequestsRecent", "Briefs", "ActiveItemIDs", "Neighbors", "Popular", "Cards"} {
		h.fail[op] = true
		if _, err := h.svc.Recommend(ctx, "u", "", ""); !errors.Is(err, errBoom) {
			t.Errorf("%s: %v", op, err)
		}
		h.fail[op] = false
	}
	h.current = ""
	h.fail["ActiveFeatures"] = true
	if _, err := h.svc.Recommend(ctx, "u", "", ""); !errors.Is(err, errBoom) {
		t.Errorf("ActiveFeatures sin modelo: %v", err)
	}
}

func TestRecompute(t *testing.T) {
	h := newHarness(t)
	for i := 1; i <= 4; i++ {
		h.addItem(oid(i), "dueño", "active", "tools", "Tula")
	}
	h.gone[oid(99)] = true
	h.views = []domain.Event{
		{UserID: "a", ItemID: oid(1), Weight: 1, At: now0.Add(-time.Hour)}, {UserID: "a", ItemID: oid(2), Weight: 1, At: now0.Add(-time.Hour)},
		{UserID: oid(99), ItemID: oid(3), Weight: 1, At: now0.Add(-time.Hour)},
		{UserID: "viejo", ItemID: oid(4), Weight: 1, At: now0.Add(-domain.Window - time.Hour)},
	}
	h.requests = []domain.Event{
		{UserID: "b", ItemID: oid(1), Weight: 3, At: now0.Add(-2 * time.Hour)},
		{UserID: "b", ItemID: oid(2), Weight: 3, At: now0.Add(-30 * time.Minute)},
	}
	h.resets["b"] = now0.Add(-time.Hour) // la primera solicitud de b ya no cuenta

	res, err := h.svc.Recompute(ctx)
	if err != nil || !res.Built {
		t.Fatalf("Recompute = %+v %v", res, err)
	}
	if res.Interactions != 3 || res.Items != 4 || res.Purged != 1 || h.current != res.BuildID {
		t.Errorf("resultado = %+v (vigente %s)", res, h.current)
	}
	for _, v := range h.views {
		if v.UserID == oid(99) {
			t.Error("los eventos de la cuenta dada de baja debían borrarse")
		}
	}
	if n := h.neighbors[res.BuildID][oid(1)]; len(n) == 0 || n[0].ItemID != oid(2) {
		t.Errorf("vecinos de 1 = %+v", n)
	}
	if len(h.popular[res.BuildID][domain.PopularAll]) != 4 || h.lockOwner != "" {
		t.Error("populares o candado")
	}

	// Un segundo recálculo borra el modelo anterior.
	res2, _ := h.svc.Recompute(ctx)
	if _, old := h.neighbors[res.BuildID]; old || h.current != res2.BuildID {
		t.Error("debía quedar solo el modelo nuevo")
	}
}

func TestRecomputeLockAndFailures(t *testing.T) {
	h := newHarness(t)
	h.addItem(oid(1), "dueño", "active", "tools", "Tula")
	h.lockOwner, h.lockUntil = "otro", now0.Add(time.Minute)
	if res, err := h.svc.Recompute(ctx); err != nil || res.Built {
		t.Fatalf("con candado ajeno: %+v %v", res, err)
	}
	h.lockOwner, h.lockUntil = "", time.Time{}

	for _, op := range []string{"TryLock", "InteractionsSince", "RequestsSince", "Resets", "Deactivated", "DeleteUsers", "ActiveFeatures", "Save", "Publish"} {
		h.fail[op] = true
		_, err := h.svc.Recompute(ctx)
		if !errors.Is(err, errBoom) {
			t.Errorf("%s: %v", op, err)
		}
		if h.lockOwner != "" {
			t.Errorf("%s: el candado debía soltarse", op)
		}
		h.fail[op] = false
	}
	// Si el candado venció y otro lo tomó, no publica.
	h.svc.d.NewID = func() string { h.lockOwner = "intruso"; return "x" }
	if _, err := h.svc.Recompute(ctx); err == nil || !strings.Contains(err.Error(), "venció") {
		t.Errorf("candado perdido: %v", err)
	}
	// No borrar el modelo anterior solo se registra.
	h.svc.d.NewID = newHarness(t).svc.d.NewID
	h.lockOwner, h.lockUntil = "", time.Time{}
	h.fail["DeleteOtherBuilds"] = true
	if res, err := h.svc.Recompute(ctx); err != nil || !res.Built {
		t.Errorf("DeleteOtherBuilds: %+v %v", res, err)
	}
}

// CA-08: varios procesos encuentran el modelo vencido a la vez; solo uno
// recalcula y ninguna petición lo espera.
func TestConcurrentRecomputeCA08(t *testing.T) {
	h := newHarness(t)
	h.addItem(oid(1), "dueño", "active", "tools", "Tula")
	var n atomic.Int64
	h.svc.d.NewID = func() string { return "p" + string(rune('a'+n.Add(1)%26)) + time.Now().String() }
	var built atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := h.svc.Recompute(ctx); err == nil && res.Built {
				built.Add(1)
			}
		}()
	}
	wg.Wait()
	if built.Load() < 1 {
		t.Fatal("ninguno recalculó")
	}
	// El recálculo perezoso se lanza aparte: la petición no lo ejecuta.
	h.now = now0.Add(domain.ModelMaxAge + time.Hour)
	before := h.calls["InteractionsSince"]
	if _, err := h.svc.Recommend(ctx, "u", "", ""); err != nil || h.launched != 1 || h.calls["InteractionsSince"] != before {
		t.Errorf("la petición no debe recalcular: %v, lanzados %d", err, h.launched)
	}
}

func TestRecomputeInBackground(t *testing.T) {
	h := newHarness(t)
	h.addItem(oid(1), "dueño", "active", "tools", "Tula")
	h.svc.recomputeInBackground()
	if h.current == "" {
		t.Fatal("debía publicar un modelo")
	}
	h.fail["TryLock"] = true
	h.svc.recomputeInBackground() // solo registra el error
}

func TestEvaluateBeatsPopularity(t *testing.T) {
	cfg := synth.Default()
	items := synth.Items(cfg)
	events := synth.Interactions(items, cfg)
	r := Evaluate(events, items, 10)
	if r != Evaluate(events, items, 10) {
		t.Fatal("la evaluación no es reproducible")
	}
	t.Logf("k=%d personas=%d híbrido=%+v cf=%+v contenido=%+v popularidad=%+v", r.K, r.Users, r.Hybrid, r.CF, r.Content, r.Popularity)
	if r.Users < 250 || r.K != 10 {
		t.Errorf("informe = %+v", r)
	}
	if r.Hybrid.HitAt <= r.Popularity.HitAt {
		t.Errorf("el híbrido no supera a la popularidad: %.3f vs %.3f", r.Hybrid.HitAt, r.Popularity.HitAt)
	}
}
