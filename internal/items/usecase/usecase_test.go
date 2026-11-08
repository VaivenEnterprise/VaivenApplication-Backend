package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	images "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/moderation"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// --- dobles ---

type memRepo struct {
	items     map[string]domain.Item
	seq       int
	failNext  error
	conflicts int // Update devuelve conflicto estas veces
}

func newMemRepo() *memRepo { return &memRepo{items: map[string]domain.Item{}} }

func (r *memRepo) Create(_ context.Context, it domain.Item) (domain.Item, error) {
	if err := r.takeFail(); err != nil {
		return domain.Item{}, err
	}
	r.seq++
	it.ID = fmt.Sprintf("%024d", r.seq)
	r.items[it.ID] = it
	return it, nil
}

func (r *memRepo) ByID(_ context.Context, id string) (domain.Item, error) {
	it, ok := r.items[id]
	if !ok {
		return domain.Item{}, domain.ErrNotFound
	}
	return it, nil
}

func (r *memRepo) Update(_ context.Context, it domain.Item) (domain.Item, error) {
	if err := r.takeFail(); err != nil {
		return domain.Item{}, err
	}
	if r.conflicts > 0 {
		r.conflicts--
		return domain.Item{}, domain.ErrVersionConflict
	}
	cur, ok := r.items[it.ID]
	if !ok || cur.Version != it.Version || cur.OwnerID != it.OwnerID {
		return domain.Item{}, domain.ErrVersionConflict
	}
	it.Version++
	r.items[it.ID] = it
	return it, nil
}

func (r *memRepo) ListByOwner(_ context.Context, owner string, sts []domain.Status, after *domain.Cursor, limit int) ([]domain.Item, error) {
	if err := r.takeFail(); err != nil {
		return nil, err
	}
	var out []domain.Item
	for _, it := range r.items {
		ok := it.OwnerID == owner
		ok = ok && containsStatus(sts, it.Status)
		if ok && after != nil {
			ok = it.CreatedAt.Before(after.CreatedAt) || (it.CreatedAt.Equal(after.CreatedAt) && it.ID < after.ID)
		}
		if ok {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func containsStatus(sts []domain.Status, s domain.Status) bool {
	for _, x := range sts {
		if x == s {
			return true
		}
	}
	return false
}

func (r *memRepo) takeFail() error {
	err := r.failNext
	r.failNext = nil
	return err
}

type fakeImages struct {
	resolveErr error
	urlErr     error
	calls      int
}

func (f *fakeImages) Signature(context.Context, string, images.Purpose) (images.UploadSignature, error) {
	return images.UploadSignature{}, nil
}

func (f *fakeImages) Resolve(_ context.Context, userID string, p images.Purpose, field string, refs []images.Ref) ([]images.Image, error) {
	f.calls++
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	if p != images.PurposeItemPhoto || field != "photos" {
		return nil, fmt.Errorf("propósito %s campo %s", p, field)
	}
	out := make([]images.Image, 0, len(refs))
	for _, r := range refs {
		out = append(out, images.Image{PublicID: r.PublicID, Format: "jpg", Delivery: images.DeliveryPublic, Width: len(userID)})
	}
	return out, nil
}

func (f *fakeImages) URLs(img images.Image) (images.URLs, error) {
	if f.urlErr != nil {
		return images.URLs{}, f.urlErr
	}
	return images.URLs{URL: "https://img/" + img.PublicID, ThumbURL: "https://thumb/" + img.PublicID}, nil
}

// fakeModeration rechaza los textos que contienen "prohibido".
type fakeModeration struct {
	err     error
	checked []string
}

func (f *fakeModeration) Check(_ context.Context, text string) (moderation.Result, error) {
	f.checked = append(f.checked, text)
	if f.err != nil {
		return moderation.Result{}, f.err
	}
	if strings.Contains(text, "prohibido") {
		return moderation.Result{Allowed: false, Reason: "contiene una palabra prohibida"}, nil
	}
	if strings.Contains(text, "raro") {
		return moderation.Result{Allowed: false}, nil
	}
	return moderation.Result{Allowed: true}, nil
}

type fakeUsers struct{ err error }

func (f fakeUsers) PublicUsers(_ context.Context, ids []string) (map[string]users.PublicUser, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]users.PublicUser{}
	for _, id := range ids {
		if id != "ghost" {
			out[id] = users.PublicUser{ID: id, DisplayName: "Ana " + id}
		}
	}
	return out, nil
}

type fakeBookings struct {
	busy bool
	err  error
}

func (f fakeBookings) HasActiveBookings(context.Context, string) (bool, error) { return f.busy, f.err }

type env struct {
	svc    *Service
	repo   *memRepo
	images *fakeImages
	mod    *fakeModeration
	now    time.Time
}

func newEnv(t *testing.T, mutate ...func(*Deps)) env {
	t.Helper()
	e := env{repo: newMemRepo(), images: &fakeImages{}, mod: &fakeModeration{}, now: time.Date(2026, 11, 8, 16, 0, 0, 123456789, time.UTC)}
	d := Deps{Items: e.repo, Images: e.images, Moderation: e.mod, Users: fakeUsers{}, Now: func() time.Time { return e.now }}
	for _, m := range mutate {
		m(&d)
	}
	svc, err := NewService(d)
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

func ptr[T any](v T) *T { return &v }

var owner = Viewer{ID: "owner1"}

func rentInput() ItemInput {
	return ItemInput{
		Title:              " Taladro percutor ",
		Description:        "Con estuche",
		Category:           "herramientas_electricas",
		Condition:          domain.ConditionGood,
		Modality:           domain.ModalityRent,
		DeclaredValueCents: 100_000,
		PricePerDayCents:   3_000,
		PriceSource:        domain.PriceSuggested,
		UsageRules:         ptr("Devolver limpio"),
		Zone:               domain.Zone{Municipality: "Coyoacán"},
		Location:           &domain.Location{Lat: 19.123456, Lng: -99.162301},
		Photos:             []images.Ref{{PublicID: "p1"}, {PublicID: "p2"}},
	}
}

func assertCode(t *testing.T, err error, code shared.Code) *shared.Error {
	t.Helper()
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
	return de
}

// --- pruebas ---

func TestNewServiceRequiresDeps(t *testing.T) {
	if _, err := NewService(Deps{}); err == nil {
		t.Error("NewService sin dependencias debe fallar")
	}
	s, err := NewService(Deps{Items: newMemRepo(), Images: &fakeImages{}, Moderation: &fakeModeration{}, Users: fakeUsers{}})
	if err != nil || s.d.Bookings == nil || s.now().IsZero() {
		t.Errorf("valores por defecto: %v", err)
	}
}

func TestCreate(t *testing.T) {
	e := newEnv(t)
	v, err := e.svc.Create(context.Background(), owner, rentInput())
	if err != nil {
		t.Fatal(err)
	}
	it := v.Item
	if it.ID == "" || it.OwnerID != "owner1" || it.Status != domain.StatusActive || it.Version != 1 {
		t.Errorf("objeto = %+v", it)
	}
	if it.Title != "Taladro percutor" {
		t.Errorf("título sin recortar: %q", it.Title)
	}
	// CA-03: coordenadas redondeadas a 2 decimales.
	if *it.Location != (domain.Location{Lat: 19.12, Lng: -99.16}) {
		t.Errorf("ubicación = %+v", *it.Location)
	}
	if !it.CreatedAt.Equal(e.now.Truncate(time.Millisecond)) {
		t.Errorf("CreatedAt = %v", it.CreatedAt)
	}
	if len(it.Photos) != 2 || it.Photos[0].PublicID != "p1" || len(v.PhotoURLs) != 2 || v.PhotoURLs[1].ThumbURL != "https://thumb/p2" {
		t.Errorf("fotos = %+v urls = %+v", it.Photos, v.PhotoURLs)
	}
	if v.Owner.DisplayName != "Ana owner1" || !v.ShowDeclaredValue {
		t.Errorf("vista = %+v", v)
	}
	if len(e.mod.checked) != 3 {
		t.Errorf("moderó %v, want título, descripción y reglas", e.mod.checked)
	}
}

func TestCreateLoanCA01(t *testing.T) {
	e := newEnv(t)
	in := rentInput()
	in.Modality, in.PriceSource = domain.ModalityLoan, domain.PriceManual
	de := assertCode(t, mustErr(e.svc.Create(context.Background(), owner, in)), shared.CodeValidation)
	if de.Details[0].Field != "pricePerDayCents" {
		t.Errorf("detalles = %+v", de.Details)
	}
	if e.images.calls != 0 {
		t.Error("no debe llamar a Cloudinary si los datos son inválidos")
	}
}

func mustErr(_ ItemView, err error) error { return err }

func TestCreateErrors(t *testing.T) {
	ctx := context.Background()

	e := newEnv(t)
	in := rentInput()
	in.Photos = nil
	assertCode(t, mustErr(e.svc.Create(ctx, owner, in)), shared.CodeValidation)

	// CA-04: moderación rechaza con el campo y el motivo.
	in = rentInput()
	in.Title = "algo prohibido"
	in.Description = "raro"
	de := assertCode(t, mustErr(e.svc.Create(ctx, owner, in)), shared.CodeValidation)
	if len(de.Details) != 2 || de.Details[0].Field != "title" || de.Details[0].Issue != "contiene una palabra prohibida" ||
		de.Details[1].Issue != "contiene contenido no permitido" {
		t.Errorf("detalles = %+v", de.Details)
	}

	e.mod.err = errors.New("caído")
	if err := mustErr(e.svc.Create(ctx, owner, rentInput())); err == nil {
		t.Error("error de moderación debe propagarse")
	}
	e.mod.err = nil

	e.images.resolveErr = shared.NewUnavailable("Cloudinary")
	assertCode(t, mustErr(e.svc.Create(ctx, owner, rentInput())), shared.CodeUnavailable)
	e.images.resolveErr = nil

	e.repo.failNext = errors.New("mongo")
	if err := mustErr(e.svc.Create(ctx, owner, rentInput())); err == nil {
		t.Error("error del repositorio debe propagarse")
	}

	e2 := newEnv(t, func(d *Deps) { d.Users = fakeUsers{err: errors.New("users")} })
	if err := mustErr(e2.svc.Create(ctx, owner, rentInput())); err == nil {
		t.Error("error al leer el dueño debe propagarse")
	}
	e.images.urlErr = errors.New("url")
	if err := mustErr(e.svc.Create(ctx, owner, rentInput())); err == nil {
		t.Error("error de URL debe propagarse")
	}
}

func TestGetVisibility(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	v, _ := e.svc.Create(ctx, owner, rentInput())
	id := v.Item.ID

	other := Viewer{ID: "otro"}
	got, err := e.svc.Get(ctx, other, id)
	if err != nil || got.ShowDeclaredValue {
		t.Errorf("otro ve el activo sin valor declarado: %+v %v", got.ShowDeclaredValue, err)
	}
	if _, err := e.svc.Pause(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	// CA-06: pausado → 404 para otros; visible para dueño y admin.
	if _, err := e.svc.Get(ctx, other, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("otro ve pausado: %v", err)
	}
	if got, err := e.svc.Get(ctx, Viewer{ID: "adm", Admin: true}, id); err != nil || !got.ShowDeclaredValue {
		t.Errorf("admin: %v", err)
	}
	if _, err := e.svc.Get(ctx, owner, id); err != nil {
		t.Errorf("dueño: %v", err)
	}
	if _, err := e.svc.Get(ctx, owner, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}
}

func TestGetDeletedOwner(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	v, _ := e.svc.Create(ctx, Viewer{ID: "ghost"}, rentInput())
	got, err := e.svc.Get(ctx, owner, v.Item.ID)
	if err != nil || got.Owner.DisplayName != DeletedOwnerName || got.Owner.ID != "ghost" {
		t.Errorf("dueño eliminado = %+v %v", got.Owner, err)
	}
}

func TestSuggestPriceAndCategories(t *testing.T) {
	e := newEnv(t)
	if got, err := e.svc.SuggestPrice(100_000); err != nil || got != 3_000 { // CA-02
		t.Errorf("SuggestPrice = %d %v", got, err)
	}
	de := assertCode(t, func() error { _, err := e.svc.SuggestPrice(10); return err }(), shared.CodeValidation)
	if de.Details[0].Field != "declaredValueCents" {
		t.Errorf("detalle = %+v", de.Details)
	}
	if len(e.svc.Categories()) != 7 {
		t.Error("categorías")
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	v, _ := e.svc.Create(ctx, owner, rentInput())
	id := v.Item.ID
	e.mod.checked = nil
	e.images.calls = 0

	got, err := e.svc.Update(ctx, owner, id, UpdateInput{
		Version: 1, PricePerDayCents: ptr[int64](3_500), PriceSource: ptr(domain.PriceManual),
		SetUsageRules: true, SetLocation: true, Zone: &domain.Zone{Municipality: "Tlalpan", Neighborhood: ptr(" ")},
	})
	if err != nil {
		t.Fatal(err)
	}
	it := got.Item
	if it.Version != 2 || it.PricePerDayCents != 3_500 || it.UsageRules != nil || it.Location != nil ||
		it.Zone.Municipality != "Tlalpan" || it.Zone.Neighborhood != nil || len(it.Photos) != 2 {
		t.Errorf("tras editar = %+v", it)
	}
	if e.images.calls != 0 || len(e.mod.checked) != 0 {
		t.Errorf("sin fotos ni textos nuevos no se llama a Cloudinary (%d) ni a moderación (%v)", e.images.calls, e.mod.checked)
	}

	got, err = e.svc.Update(ctx, owner, id, UpdateInput{
		Version: 2, Title: ptr("Taladro nuevo"), Description: ptr("otra"), Category: ptr("hogar"),
		Condition: ptr(domain.ConditionFair), Modality: ptr(domain.ModalityLoan), PricePerDayCents: ptr[int64](0),
		DeclaredValueCents: ptr[int64](50_000), UsageRules: ptr("sin reglas raras no"), SetUsageRules: false,
		SetLocation: true, Location: &domain.Location{Lat: 1.006, Lng: 2.444},
		Photos: []images.Ref{{PublicID: "p9"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	it = got.Item
	if it.Title != "Taladro nuevo" || it.Modality != domain.ModalityLoan || len(it.Photos) != 1 || it.Photos[0].PublicID != "p9" ||
		it.UsageRules != nil || *it.Location != (domain.Location{Lat: 1.01, Lng: 2.44}) || it.Condition != domain.ConditionFair {
		t.Errorf("tras editar 2 = %+v", it)
	}
	if e.images.calls != 1 || len(e.mod.checked) != 2 {
		t.Errorf("Cloudinary %d, moderación %v", e.images.calls, e.mod.checked)
	}
}

func TestUpdateErrors(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	v, _ := e.svc.Create(ctx, owner, rentInput())
	id := v.Item.ID

	// CA-05: otra persona → 403.
	assertCode(t, mustErr(e.svc.Update(ctx, Viewer{ID: "otro"}, id, UpdateInput{Version: 1, Title: ptr("x y z")})), shared.CodeForbidden)
	// Versión vieja → 409.
	if err := mustErr(e.svc.Update(ctx, owner, id, UpdateInput{Version: 7, Title: ptr("Nuevo")})); !errors.Is(err, domain.ErrVersionConflict) {
		t.Errorf("versión: %v", err)
	}
	// El resultado debe cumplir las reglas (préstamo con precio).
	assertCode(t, mustErr(e.svc.Update(ctx, owner, id, UpdateInput{Version: 1, Modality: ptr(domain.ModalityLoan)})), shared.CodeValidation)
	// Moderación del texto cambiado.
	assertCode(t, mustErr(e.svc.Update(ctx, owner, id, UpdateInput{Version: 1, SetUsageRules: true, UsageRules: ptr("algo prohibido")})), shared.CodeValidation)
	// Fotos inválidas.
	e.images.resolveErr = shared.NewValidation("fotos")
	assertCode(t, mustErr(e.svc.Update(ctx, owner, id, UpdateInput{Version: 1, Photos: []images.Ref{{PublicID: "x"}}})), shared.CodeValidation)
	e.images.resolveErr = nil
	// Conflicto al guardar (otra petición ganó).
	e.repo.conflicts = 1
	if err := mustErr(e.svc.Update(ctx, owner, id, UpdateInput{Version: 1, Title: ptr("Nuevo")})); !errors.Is(err, domain.ErrVersionConflict) {
		t.Errorf("conflicto al guardar: %v", err)
	}
	if err := mustErr(e.svc.Update(ctx, owner, "nope", UpdateInput{Version: 1})); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	// Bloqueado → 409.
	if err := e.svc.Block(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := mustErr(e.svc.Update(ctx, owner, id, UpdateInput{Version: 2, Title: ptr("Nuevo")})); !errors.Is(err, domain.ErrBlocked) {
		t.Errorf("bloqueado: %v", err)
	}
}
