package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	authusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	bookingsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	bookingsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	imagescloudinary "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/adapter/cloudinary"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	itemsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/usecase"
)

// testPassword devuelve una contraseña ficticia distinta por cuenta.
func testPassword(i int) string { return fmt.Sprintf("Pw-ficticia-%02d-xyz", i) }

// writeCreds escribe un archivo de credenciales válido en un directorio
// temporal de la prueba (fuera del repositorio).
func writeCreds(t *testing.T, mutate func(lines []string) []string) string {
	t.Helper()
	var lines []string
	for i, e := range seedRequiredEmails() {
		lines = append(lines, e+"="+testPassword(i))
	}
	if mutate != nil {
		lines = mutate(lines)
	}
	p := filepath.Join(t.TempDir(), "creds.txt")
	if err := os.WriteFile(p, []byte("# credenciales de prueba\n\n"+strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadSeedCredentials(t *testing.T) {
	creds, err := loadSeedCredentials(writeCreds(t, nil), seedRequiredEmails())
	if err != nil || len(creds) != 7 {
		t.Fatalf("válido: %d cuentas, err %v", len(creds), err)
	}
	if _, err := loadSeedCredentials("", seedRequiredEmails()); err == nil || !strings.Contains(err.Error(), seedCredentialsEnv) {
		t.Fatalf("sin ruta: err = %v", err)
	}
	if _, err := loadSeedCredentials(filepath.Join(t.TempDir(), "no-existe"), seedRequiredEmails()); err == nil {
		t.Fatal("archivo inexistente: want error")
	}
	cases := map[string]func([]string) []string{
		"sin igual":           func(l []string) []string { return append(l, "linea-sin-separador") },
		"cuenta desconocida":  func(l []string) []string { return append(l, "otra@vaiven.example="+testPassword(99)) },
		"cuenta repetida":     func(l []string) []string { return append(l, l[0]) },
		"contraseña corta":    func(l []string) []string { l[1] = strings.SplitN(l[1], "=", 2)[0] + "=corta"; return l },
		"falta una cuenta":    func(l []string) []string { return l[1:] },
		"contraseña repetida": func(l []string) []string { l[2] = strings.SplitN(l[2], "=", 2)[0] + "=" + testPassword(3); return l },
	}
	for name, mutate := range cases {
		_, err := loadSeedCredentials(writeCreds(t, mutate), seedRequiredEmails())
		if err == nil {
			t.Errorf("%s: want error", name)
			continue
		}
		for i := 0; i < 8; i++ {
			if strings.Contains(err.Error(), testPassword(i)) {
				t.Errorf("%s: el error muestra una contraseña: %v", name, err)
			}
		}
	}
}

func TestRunSeedGuardsWriteNothing(t *testing.T) {
	never := func(context.Context, config.Config, bool) (seedDeps, error) {
		t.Fatal("no debe conectarse")
		return seedDeps{}, nil
	}
	env := func(string) string { return writeCreds(t, nil) }
	var errb bytes.Buffer
	if code := runSeed(nil, io.Discard, &errb, config.Config{AppEnv: config.EnvProduction}, env, never); code != 1 || !strings.Contains(errb.String(), "production") {
		t.Fatalf("production: código %d, stderr %q", code, errb.String())
	}
	if code := runSeed([]string{"--scale", "3"}, io.Discard, io.Discard, config.Config{AppEnv: config.EnvStaging}, env, never); code != 1 {
		t.Fatalf("--scale en staging: código %d, want 1", code)
	}
	if code := runSeed([]string{"--scale", "-1"}, io.Discard, io.Discard, config.Config{AppEnv: config.EnvDevelopment}, env, never); code != 2 {
		t.Fatalf("--scale negativo: código %d, want 2", code)
	}
	if code := runSeed([]string{"extra"}, io.Discard, io.Discard, config.Config{AppEnv: config.EnvDevelopment}, env, never); code != 2 {
		t.Fatalf("argumento extra: código %d, want 2", code)
	}
	noCreds := func(string) string { return "" }
	if code := runSeed(nil, io.Discard, io.Discard, config.Config{AppEnv: config.EnvStaging}, noCreds, never); code != 1 {
		t.Fatalf("sin credenciales: código %d, want 1", code)
	}
}

func TestRunSubcommandSeedRefusesProduction(t *testing.T) {
	load := func() (config.Config, error) { return config.Config{AppEnv: config.EnvProduction}, nil }
	if code := run([]string{"seed"}, io.Discard, io.Discard, load, nil, nil, nil, nil); code != 1 {
		t.Fatalf("código %d, want 1", code)
	}
	if code := run([]string{"indexes", "extra"}, io.Discard, io.Discard, load, nil, nil, nil, nil); code != 2 {
		t.Fatalf("argumento extra en otro subcomando: código %d, want 2", code)
	}
}

// --- Fakes de los casos de uso -------------------------------------------

type fakeSeedWorld struct {
	users      map[string]authdomain.User
	items      map[string][]itemsusecase.ItemView // por dueño
	bookings   []bookingsusecase.BookingView
	registered int
	accepted   []string
	cancelled  []string
	uploaded   []imagesdomain.Ref
	destroyed  []imagesdomain.Ref
	failCreate error
	failDelete error
	seq        int
}

func newFakeWorld() *fakeSeedWorld {
	return &fakeSeedWorld{users: map[string]authdomain.User{}, items: map[string][]itemsusecase.ItemView{}}
}

func (w *fakeSeedWorld) id(prefix string) string { w.seq++; return fmt.Sprintf("%s%d", prefix, w.seq) }

func (w *fakeSeedWorld) Register(_ context.Context, in authusecase.RegisterInput) (authusecase.AuthResult, error) {
	w.registered++
	u := authdomain.User{ID: w.id("u"), Email: in.Email, Role: authdomain.RoleUser}
	w.users[in.Email] = u
	return authusecase.AuthResult{User: u}, nil
}

func (w *fakeSeedWorld) ByEmail(_ context.Context, email string) (authdomain.User, error) {
	if u, ok := w.users[email]; ok {
		return u, nil
	}
	return authdomain.User{}, authdomain.ErrUserNotFound
}

func (w *fakeSeedWorld) Create(_ context.Context, v itemsusecase.Viewer, in itemsusecase.ItemInput) (itemsusecase.ItemView, error) {
	if w.failCreate != nil {
		return itemsusecase.ItemView{}, w.failCreate
	}
	var view itemsusecase.ItemView
	view.Item.ID, view.Item.OwnerID, view.Item.Title = w.id("i"), v.ID, in.Title
	w.items[v.ID] = append(w.items[v.ID], view)
	return view, nil
}

func (w *fakeSeedWorld) ListMine(_ context.Context, v itemsusecase.Viewer, _ itemsusecase.ListQuery) (itemsusecase.Page, error) {
	return itemsusecase.Page{Items: w.items[v.ID]}, nil
}

type fakeBookings struct{ w *fakeSeedWorld }

func (b fakeBookings) Quote(_ context.Context, _ bookingsusecase.Viewer, in bookingsusecase.QuoteInput) (bookingsdomain.Quote, error) {
	return bookingsdomain.Quote{ItemID: in.ItemID, TotalCents: 1000}, nil
}

func (b fakeBookings) Create(_ context.Context, v bookingsusecase.Viewer, in bookingsusecase.CreateInput) (bookingsusecase.BookingView, error) {
	var view bookingsusecase.BookingView
	view.Booking = bookingsdomain.Booking{ID: b.w.id("b"), ItemID: in.ItemID, RequesterID: v.ID, Status: bookingsdomain.StatusPending}
	b.w.bookings = append(b.w.bookings, view)
	return view, nil
}

func (b fakeBookings) set(id string, st bookingsdomain.Status) {
	for i := range b.w.bookings {
		if b.w.bookings[i].Booking.ID == id {
			b.w.bookings[i].Booking.Status = st
		}
	}
}

func (b fakeBookings) Accept(_ context.Context, _ bookingsusecase.Viewer, id string, _ *string) (bookingsusecase.BookingView, error) {
	b.w.accepted = append(b.w.accepted, id)
	b.set(id, bookingsdomain.StatusAccepted)
	return bookingsusecase.BookingView{}, nil
}

func (b fakeBookings) Cancel(_ context.Context, _ bookingsusecase.Viewer, id string, _ *string) (bookingsusecase.BookingView, error) {
	b.w.cancelled = append(b.w.cancelled, id)
	b.set(id, bookingsdomain.StatusCancelled)
	return bookingsusecase.BookingView{}, nil
}

func (b fakeBookings) List(_ context.Context, v bookingsusecase.Viewer, _ bookingsusecase.ListQuery) (bookingsusecase.Page, error) {
	var out []bookingsusecase.BookingView
	for _, x := range b.w.bookings {
		if x.Booking.RequesterID == v.ID {
			out = append(out, x)
		}
	}
	return bookingsusecase.Page{Items: out}, nil
}

type fakePhotos struct{ w *fakeSeedWorld }

func (p fakePhotos) Upload(_ context.Context, owner string, data []byte) (imagesdomain.Ref, error) {
	if len(data) == 0 || len(data) > 100<<10 {
		return imagesdomain.Ref{}, fmt.Errorf("foto de %d bytes", len(data))
	}
	r := imagesdomain.Ref{PublicID: "vaiven/test/item_photo/" + owner + "/" + p.w.id("p")}
	p.w.uploaded = append(p.w.uploaded, r)
	return r, nil
}

func (p fakePhotos) Destroy(_ context.Context, r imagesdomain.Ref) error {
	p.w.destroyed = append(p.w.destroyed, r)
	return p.w.failDelete
}

type fakeAdminStore struct{ w *fakeSeedWorld }

func (s fakeAdminStore) Create(_ context.Context, u authdomain.User) (authdomain.User, error) {
	u.ID = s.w.id("a")
	s.w.users[u.Email] = u
	return u, nil
}

func fakeDeps(w *fakeSeedWorld) seedDeps {
	loc, _ := bookingsdomain.Location()
	return seedDeps{
		auth: w, users: w, admins: adminDeps{store: fakeAdminStore{w}, hasher: fakeHasher{}},
		items: w, bookings: fakeBookings{w}, photos: fakePhotos{w},
		today: func() bookingsdomain.Date { return bookingsdomain.Today(time.Now(), loc) },
		now:   time.Now, close: func() {},
	}
}

func seedOnce(t *testing.T, w *fakeSeedWorld) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	open := func(context.Context, config.Config, bool) (seedDeps, error) { return fakeDeps(w), nil }
	path := writeCreds(t, nil)
	code := runSeed(nil, &out, &errb, config.Config{AppEnv: config.EnvStaging}, func(string) string { return path }, open)
	return code, out.String(), errb.String()
}

// CA-02: dos ejecuciones no duplican nada ni tocan contraseñas.
func TestSeedIsIdempotent(t *testing.T) {
	w := newFakeWorld()
	code, out, errs := seedOnce(t, w)
	if code != 0 {
		t.Fatalf("primera: código %d\n%s%s", code, out, errs)
	}
	if w.registered != 6 || len(w.users) != 7 || len(w.uploaded) != 15 || len(w.bookings) != 3 {
		t.Fatalf("primera: %d registros, %d cuentas, %d fotos, %d reservas", w.registered, len(w.users), len(w.uploaded), len(w.bookings))
	}
	if len(w.accepted) != 1 || len(w.cancelled) != 1 {
		t.Fatalf("transiciones: %d aceptadas, %d canceladas", len(w.accepted), len(w.cancelled))
	}
	code, out, _ = seedOnce(t, w)
	if code != 0 || w.registered != 6 || len(w.uploaded) != 15 || len(w.bookings) != 3 || len(w.accepted) != 1 {
		t.Fatalf("segunda ejecución creó algo: código %d, %d registros, %d fotos, %d reservas", code, w.registered, len(w.uploaded), len(w.bookings))
	}
	if !strings.Contains(out, "personas 0 nuevas/7 existentes, objetos 0/15, reservas 0/3") {
		t.Fatalf("resumen inesperado:\n%s", out)
	}
	for i := 0; i < 8; i++ {
		if strings.Contains(out, testPassword(i)) {
			t.Fatal("la salida muestra una contraseña")
		}
	}
}

// Una ejecución interrumpida (reserva creada sin su transición) se completa;
// una reserva que ya avanzó no se toca.
func TestSeedCompletesPendingTransition(t *testing.T) {
	w := newFakeWorld()
	if code, _, errs := seedOnce(t, w); code != 0 {
		t.Fatal(errs)
	}
	accepted := w.accepted[0]
	fakeBookings{w}.set(accepted, bookingsdomain.StatusPending)
	if code, _, errs := seedOnce(t, w); code != 0 {
		t.Fatal(errs)
	}
	if len(w.accepted) != 2 || w.accepted[1] != accepted || len(w.cancelled) != 1 {
		t.Fatalf("aceptadas %v, canceladas %v", w.accepted, w.cancelled)
	}
}

func TestSeedAdminWithoutRoleIsAnError(t *testing.T) {
	w := newFakeWorld()
	w.users[authdomain.NormalizeEmail(seedAdminEmail)] = authdomain.User{ID: "x", Role: authdomain.RoleUser}
	if code, _, errs := seedOnce(t, w); code != 1 || !strings.Contains(errs, "sin rol admin") {
		t.Fatalf("código %d, stderr %q", code, errs)
	}
}

// CA-09: si crear el objeto falla, la foto subida se borra.
func TestSeedDestroysPhotoWhenItemFails(t *testing.T) {
	w := newFakeWorld()
	w.failCreate = errors.New("moderación rechazó el texto")
	code, _, errs := seedOnce(t, w)
	if code != 1 || len(w.uploaded) != 1 || len(w.destroyed) != 1 || w.destroyed[0] != w.uploaded[0] {
		t.Fatalf("código %d, subidas %v, borradas %v, stderr %q", code, w.uploaded, w.destroyed, errs)
	}
	w2 := newFakeWorld()
	w2.failCreate, w2.failDelete = errors.New("falla al crear"), errors.New("cloudinary caído")
	if code, _, errs := seedOnce(t, w2); code != 1 || !strings.Contains(errs, "no se pudo borrar la foto subida") {
		t.Fatalf("doble falla: código %d, stderr %q", code, errs)
	}
}

func TestSeedAssetsAreSmallJPEGs(t *testing.T) {
	for _, it := range seedItems {
		b, err := seedAssets.ReadFile("assets/" + it.Photo)
		if err != nil {
			t.Fatalf("%s: %v", it.Photo, err)
		}
		if len(b) > 100<<10 || !bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}) {
			t.Errorf("%s: %d bytes o no es JPEG", it.Photo, len(b))
		}
	}
}

// --- Subida y borrado firmados contra un Cloudinary falso ------------------

type sigImages struct{ uploadURL string }

func (s sigImages) Signature(_ context.Context, userID string, p imagesdomain.Purpose) (imagesdomain.UploadSignature, error) {
	return imagesdomain.UploadSignature{
		UploadURL: s.uploadURL, APIKey: "key-ficticia", Timestamp: 1700000000, Signature: "firma-ficticia",
		PublicID: "vaiven/test/" + string(p) + "/" + userID + "/abc", Type: "upload",
		AllowedFormats: imagesdomain.AllowedFormats, Overwrite: "false",
	}, nil
}

func (sigImages) Resolve(context.Context, string, imagesdomain.Purpose, string, []imagesdomain.Ref) ([]imagesdomain.Image, error) {
	return nil, nil
}

func (sigImages) URLs(imagesdomain.Image) (imagesdomain.URLs, error) { return imagesdomain.URLs{}, nil }

func TestCloudinaryPhotosUploadAndDestroy(t *testing.T) {
	const secret = "secreto-ficticio"
	var got map[string]string
	var file []byte
	status, destroyResult := http.StatusOK, `{"result":"ok"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = map[string]string{}
		if strings.HasSuffix(r.URL.Path, "/upload") {
			mr, err := r.MultipartReader()
			if err != nil {
				t.Errorf("multipart: %v", err)
				return
			}
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				b, _ := io.ReadAll(part)
				if part.FormName() == "file" {
					file = b
				} else {
					got[part.FormName()] = string(b)
				}
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid Signature firma-ficticia"}}`))
			return
		}
		_ = r.ParseForm()
		for k := range r.PostForm {
			got[k] = r.PostForm.Get(k)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(destroyResult))
	}))
	defer srv.Close()

	p := newCloudinaryPhotos(sigImages{uploadURL: srv.URL + "/image/upload"}, "demo", "key-ficticia", secret)
	p.destroyURL = srv.URL + "/image/destroy"
	p.now = func() time.Time { return time.Unix(1700000100, 0) }
	data := []byte{0xFF, 0xD8, 0xFF, 0x00}

	ref, err := p.Upload(context.Background(), "u1", data)
	if err != nil || ref.PublicID != "vaiven/test/item_photo/u1/abc" || !bytes.Equal(file, data) {
		t.Fatalf("upload: ref %v err %v", ref, err)
	}
	for k, want := range map[string]string{"api_key": "key-ficticia", "signature": "firma-ficticia", "public_id": ref.PublicID, "overwrite": "false", "type": "upload", "timestamp": "1700000000"} {
		if got[k] != want {
			t.Errorf("campo %s = %q, want %q", k, got[k], want)
		}
	}

	if err := p.Destroy(context.Background(), ref); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	wantSig := imagescloudinary.Sign(map[string]string{"public_id": ref.PublicID, "timestamp": "1700000100", "type": "upload"}, secret)
	if got["signature"] != wantSig || got["api_key"] != "key-ficticia" {
		t.Fatalf("destroy firmó mal: %v", got)
	}
	destroyResult = `{"result":"not found"}`
	if err := p.Destroy(context.Background(), ref); err != nil {
		t.Fatalf("not found cuenta como borrada: %v", err)
	}

	status = http.StatusUnauthorized
	_, err = p.Upload(context.Background(), "u1", data)
	if err == nil || strings.Contains(err.Error(), "firma-ficticia") || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "401") {
		t.Fatalf("error de subida = %v", err)
	}
	destroyResult = `{"result":"error"}`
	status = http.StatusOK
	if err := p.Destroy(context.Background(), ref); err == nil {
		t.Fatal("destroy con result=error: want error")
	}
}

func TestLocalImagesOnlyAcceptOwnFolder(t *testing.T) {
	ref, err := localPhotos{baseFolder: "vaiven/test"}.Upload(context.Background(), "u1", nil)
	if err != nil {
		t.Fatal(err)
	}
	li := localImages{baseFolder: "vaiven/test"}
	if imgs, err := li.Resolve(context.Background(), "u1", imagesdomain.PurposeItemPhoto, "photos", []imagesdomain.Ref{ref}); err != nil || len(imgs) != 1 {
		t.Fatalf("propia: %v %v", imgs, err)
	}
	if _, err := li.Resolve(context.Background(), "u2", imagesdomain.PurposeItemPhoto, "photos", []imagesdomain.Ref{ref}); err == nil {
		t.Fatal("ajena: want error")
	}
	if _, err := li.Signature(context.Background(), "u1", imagesdomain.PurposeItemPhoto); err == nil {
		t.Fatal("local no firma")
	}
	if u, _ := li.URLs(imagesdomain.Image{PublicID: "x"}); !strings.HasPrefix(u.URL, "https://example.invalid/") {
		t.Fatalf("URL local = %q", u.URL)
	}
}
