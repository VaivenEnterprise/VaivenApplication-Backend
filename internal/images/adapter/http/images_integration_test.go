//go:build integration

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	apihttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http"
	authhttp "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/adapter/http"
	shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/adapter/cloudinary"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/clock"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/security"
)

// Prueba de punta a punta de la spec 004 (T009, CA-01 a CA-05 y RF-08): router
// completo de la API, JWT real y un servidor que simula Cloudinary (subida
// firmada y API de administración). No necesita Docker ni credenciales reales.

const (
	itCloud     = "nube-de-prueba"
	itAPIKey    = "clave-publica-de-prueba"
	itAPISecret = "secreto-de-prueba"
	itBase      = "vaiven/test"
	itOwner     = "6730f1c2a9b3e4d5f6a7b8c9"
	itOther     = "6730f1c2a9b3e4d5f6a7b8d0"
)

var itNow = time.Date(2026, 11, 6, 18, 0, 0, 0, time.UTC)

// fakeResource es lo que el servidor simulado guarda de cada subida.
type fakeResource struct {
	PublicID string `json:"public_id"`
	Type     string `json:"type"`
	Format   string `json:"format"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    int64  `json:"bytes"`
}

// fakeCloudinary simula la subida firmada y la API de administración.
type fakeCloudinary struct {
	mu        sync.Mutex
	resources map[string]fakeResource // clave: type + ":" + public_id
	down      bool
}

func newFakeCloudinary(t *testing.T) (*fakeCloudinary, *httptest.Server) {
	t.Helper()
	f := &fakeCloudinary{resources: map[string]fakeResource{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1_1/{cloud}/image/upload", f.upload)
	mux.HandleFunc("GET /v1_1/{cloud}/resources/image/{type}/{id...}", f.resource)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeCloudinary) put(r fakeResource) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resources[r.Type+":"+r.PublicID] = r
}

func (f *fakeCloudinary) setDown(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = v
}

func fakeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": msg}})
}

// upload recalcula la firma como Cloudinary: si no coincide (p. ej. porque el
// cliente cambió el public_id) responde 401 y no guarda nada.
func (f *fakeCloudinary) upload(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("cloud") != itCloud {
		fakeError(w, http.StatusNotFound, "cloud desconocida")
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		fakeError(w, http.StatusBadRequest, "multipart inválido")
		return
	}
	params := map[string]string{
		"allowed_formats": r.FormValue("allowed_formats"),
		"public_id":       r.FormValue("public_id"),
		"timestamp":       r.FormValue("timestamp"),
		"type":            r.FormValue("type"),
	}
	if r.FormValue("api_key") != itAPIKey || r.FormValue("signature") != cloudinary.Sign(params, itAPISecret) {
		fakeError(w, http.StatusUnauthorized, "Invalid Signature")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		fakeError(w, http.StatusBadRequest, "falta el archivo")
		return
	}
	defer func() { _ = file.Close() }()
	n, _ := io.Copy(io.Discard, file)
	format := strings.TrimPrefix(strings.ToLower(path.Ext(hdr.Filename)), ".")
	if !strings.Contains(","+params["allowed_formats"]+",", ","+format+",") {
		fakeError(w, http.StatusBadRequest, "Image format not allowed")
		return
	}
	res := fakeResource{PublicID: params["public_id"], Type: params["type"], Format: format, Width: 800, Height: 600, Bytes: n}
	f.put(res)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// resource responde como GET /resources/image/<type>/<public_id> de la API de
// administración (autenticación básica con api_key y secreto).
func (f *fakeCloudinary) resource(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	down := f.down
	res, ok := f.resources[r.PathValue("type")+":"+r.PathValue("id")]
	f.mu.Unlock()
	if down {
		fakeError(w, http.StatusServiceUnavailable, "mantenimiento")
		return
	}
	if u, p, okAuth := r.BasicAuth(); !okAuth || u != itAPIKey || p != itAPISecret {
		fakeError(w, http.StatusUnauthorized, "credenciales")
		return
	}
	if r.PathValue("cloud") != itCloud || !ok {
		fakeError(w, http.StatusNotFound, "Resource not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

type itEnv struct {
	t      *testing.T
	api    *httptest.Server
	fake   *fakeCloudinary
	fakeTS *httptest.Server
	svc    images.Service
	jwt    *security.JWT
}

func newITEnv(t *testing.T) *itEnv {
	t.Helper()
	fake, fakeTS := newFakeCloudinary(t)
	svc, err := images.New(images.Config{
		CloudName: itCloud, APIKey: itAPIKey, APISecret: itAPISecret, BaseFolder: itBase,
		MaxBytes: domain.DefaultMaxBytes, Clock: clock.Fixed{T: itNow},
		AdminOptions: []cloudinary.AdminOption{cloudinary.WithAdminBaseURL(fakeTS.URL)},
	})
	if err != nil {
		t.Fatal(err)
	}
	jwt, err := security.NewJWT("clave-de-prueba-clave-de-prueba-0000", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mod, err := NewModule(ModuleDeps{
		Service: svc, Authenticate: authhttp.Authenticate(jwt), Log: log,
		Limiter: redis.NewMemoryLimiter(), LimiterFallback: redis.NewMemoryLimiter(), SignaturesPerHour: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	router := apihttp.NewRouter(apihttp.Deps{Log: log, Modules: []apihttp.Module{mod}})
	api := httptest.NewServer(router)
	t.Cleanup(api.Close)
	return &itEnv{t: t, api: api, fake: fake, fakeTS: fakeTS, svc: svc, jwt: jwt}
}

func (e *itEnv) token(userID string) string {
	e.t.Helper()
	tok, err := e.jwt.Issue(userID, "user")
	if err != nil {
		e.t.Fatal(err)
	}
	return tok
}

// requestSignature llama a POST /api/v1/uploads/signature.
func (e *itEnv) requestSignature(access, purpose string) (int, signatureResponse, string) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.api.URL+"/api/v1/uploads/signature",
		strings.NewReader(`{"purpose":"`+purpose+`"}`))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	res, err := e.api.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	var sig signatureResponse
	if res.StatusCode == http.StatusOK {
		if err := json.Unmarshal(raw, &sig); err != nil {
			e.t.Fatalf("respuesta de firma inválida: %s", raw)
		}
	}
	return res.StatusCode, sig, string(raw)
}

// upload sube un archivo al Cloudinary simulado con los campos de la firma,
// como lo haría el móvil. uploadUrl apunta a api.cloudinary.com: se cambia el
// host por el del servidor simulado y se conserva la ruta.
func (e *itEnv) upload(sig signatureResponse, filename string, content []byte) (int, string) {
	e.t.Helper()
	u, err := url.Parse(sig.UploadURL)
	if err != nil || u.Scheme != "https" || u.Host != "api.cloudinary.com" || u.Path != "/v1_1/"+itCloud+"/image/upload" {
		e.t.Fatalf("uploadUrl = %q", sig.UploadURL)
	}
	fakeURL, _ := url.Parse(e.fakeTS.URL)
	u.Scheme, u.Host = fakeURL.Scheme, fakeURL.Host

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range map[string]string{
		"api_key": sig.APIKey, "timestamp": strconv.FormatInt(sig.Timestamp, 10), "signature": sig.Signature,
		"public_id": sig.PublicID, "type": sig.Type, "allowed_formats": sig.AllowedFormats,
	} {
		if err := mw.WriteField(k, v); err != nil {
			e.t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		e.t.Fatal(err)
	}
	_, _ = fw.Write(content)
	if err := mw.Close(); err != nil {
		e.t.Fatal(err)
	}
	res, err := e.fakeTS.Client().Post(u.String(), mw.FormDataContentType(), &body)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(raw)
}

func errCode(err error) shared.Code {
	var de *shared.Error
	if errors.As(err, &de) {
		return de.Code
	}
	return ""
}

var smallJPEG = []byte("\xff\xd8\xff\xe0 imagen pequeña de prueba \xff\xd9")

func TestIntegrationUploadFlow(t *testing.T) {
	e := newITEnv(t)
	ctx := context.Background()

	// CA-05: sin sesión no hay firma.
	if status, _, body := e.requestSignature("", "item_photo"); status != http.StatusUnauthorized || !strings.Contains(body, `"unauthorized"`) {
		t.Fatalf("sin token = %d %s", status, body)
	}
	if status, _, _ := e.requestSignature("token-roto", "item_photo"); status != http.StatusUnauthorized {
		t.Fatalf("token inválido = %d", status)
	}

	// CA-01: firma de foto de artículo en la carpeta de la persona.
	status, sig, body := e.requestSignature(e.token(itOwner), "item_photo")
	if status != http.StatusOK {
		t.Fatalf("firma = %d %s", status, body)
	}
	if sig.Type != "upload" || !strings.HasPrefix(sig.PublicID, itBase+"/item_photo/"+itOwner+"/") ||
		sig.APIKey != itAPIKey || sig.Timestamp != itNow.Unix() || sig.AllowedFormats != domain.AllowedFormats {
		t.Fatalf("firma = %+v", sig)
	}
	if strings.Contains(body, itAPISecret) {
		t.Fatal("el API secret no debe salir del servidor (RNF-01)")
	}

	// La subida con los campos tal cual se acepta.
	if status, body := e.upload(sig, "foto.jpg", smallJPEG); status != http.StatusOK {
		t.Fatalf("subida = %d %s", status, body)
	}

	// Resolve devuelve la imagen con los metadatos que reporta Cloudinary.
	imgs, err := e.svc.Resolve(ctx, itOwner, domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: sig.PublicID}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := domain.Image{PublicID: sig.PublicID, Width: 800, Height: 600, Format: "jpg", Bytes: int64(len(smallJPEG)), Delivery: domain.DeliveryPublic}
	if len(imgs) != 1 || imgs[0] != want {
		t.Fatalf("Resolve = %+v, quiero %+v", imgs, want)
	}
	u, err := e.svc.URLs(imgs[0])
	if err != nil || !strings.HasPrefix(u.URL, "https://res.cloudinary.com/"+itCloud+"/image/upload/") || u.ThumbURL == u.URL {
		t.Fatalf("URLs = %+v, %v", u, err)
	}
}

func TestIntegrationTamperedUploadRejected(t *testing.T) { // CA-01, RF-03
	e := newITEnv(t)
	_, sig, _ := e.requestSignature(e.token(itOwner), "item_photo")

	tampered := sig
	tampered.PublicID = itBase + "/item_photo/" + itOther + "/ajena"
	if status, _ := e.upload(tampered, "foto.jpg", smallJPEG); status != http.StatusUnauthorized {
		t.Errorf("public_id alterado = %d", status)
	}
	if _, ev, _ := e.requestSignature(e.token(itOwner), "evidence"); ev.Type == "authenticated" {
		ev.Type = "upload" // intentar volver pública una evidencia
		if status, _ := e.upload(ev, "foto.jpg", smallJPEG); status != http.StatusUnauthorized {
			t.Errorf("type alterado = %d", status)
		}
	} else {
		t.Errorf("evidencia = %+v", ev)
	}
	formats := sig
	formats.AllowedFormats += ",gif"
	if status, _ := e.upload(formats, "foto.gif", []byte("GIF89a")); status != http.StatusUnauthorized {
		t.Errorf("allowed_formats alterado = %d", status)
	}
	if status, _ := e.upload(sig, "foto.gif", []byte("GIF89a")); status != http.StatusBadRequest {
		t.Errorf("gif con firma válida = %d", status)
	}
	// Nada de lo rechazado quedó guardado.
	_, err := e.svc.Resolve(context.Background(), itOther, domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: tampered.PublicID}})
	if errCode(err) != shared.CodeValidation {
		t.Errorf("la subida alterada no debe existir: %v", err)
	}
}

func TestIntegrationResolveRejects(t *testing.T) {
	e := newITEnv(t)
	ctx := context.Background()
	_, sig, _ := e.requestSignature(e.token(itOwner), "item_photo")
	if status, body := e.upload(sig, "foto.png", smallJPEG); status != http.StatusOK {
		t.Fatalf("subida = %d %s", status, body)
	}

	// CA-02: la imagen de otra persona no se acepta.
	_, err := e.svc.Resolve(ctx, itOther, domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: sig.PublicID}})
	var de *shared.Error
	if !errors.As(err, &de) || de.Code != shared.CodeValidation || len(de.Details) != 1 || de.Details[0].Field != "photos[0].publicId" {
		t.Fatalf("imagen ajena = %v", err)
	}
	// Tampoco con otro propósito.
	if _, err := e.svc.Resolve(ctx, itOwner, domain.PurposeEvidence, "evidence", []domain.Ref{{PublicID: sig.PublicID}}); errCode(err) != shared.CodeValidation {
		t.Errorf("otro propósito = %v", err)
	}

	// CA-03: 6 MB o gif guardados en Cloudinary se rechazan con detalle.
	prefix := itBase + "/item_photo/" + itOwner + "/"
	e.fake.put(fakeResource{PublicID: prefix + "grande", Type: "upload", Format: "jpg", Width: 4000, Height: 3000, Bytes: 6 << 20})
	e.fake.put(fakeResource{PublicID: prefix + "animada", Type: "upload", Format: "gif", Width: 10, Height: 10, Bytes: 1024})
	for name, id := range map[string]string{"6 MB": prefix + "grande", "gif": prefix + "animada", "no subida": prefix + "inexistente"} {
		_, err := e.svc.Resolve(ctx, itOwner, domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: sig.PublicID}, {PublicID: id}})
		if !errors.As(err, &de) || de.Code != shared.CodeValidation || len(de.Details) != 1 || de.Details[0].Field != "photos[1].publicId" {
			t.Errorf("%s = %v", name, err)
		}
	}

	// RF-08: Cloudinary caído → service_unavailable, sin lista parcial.
	e.fake.setDown(true)
	imgs, err := e.svc.Resolve(ctx, itOwner, domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: sig.PublicID}})
	if imgs != nil || errCode(err) != shared.CodeUnavailable {
		t.Errorf("Cloudinary caído = %v, %v", imgs, err)
	}
	e.fake.setDown(false)
	if _, err := e.svc.Resolve(ctx, itOwner, domain.PurposeItemPhoto, "photos", []domain.Ref{{PublicID: sig.PublicID}}); err != nil {
		t.Errorf("tras recuperarse = %v", err)
	}
}

func TestIntegrationEvidenceSignedURL(t *testing.T) { // CA-04
	e := newITEnv(t)
	status, sig, body := e.requestSignature(e.token(itOwner), "evidence")
	if status != http.StatusOK || sig.Type != "authenticated" || !strings.HasPrefix(sig.PublicID, itBase+"/evidence/"+itOwner+"/") {
		t.Fatalf("firma de evidencia = %d %s", status, body)
	}
	if status, body := e.upload(sig, "evidencia.webp", smallJPEG); status != http.StatusOK {
		t.Fatalf("subida = %d %s", status, body)
	}
	imgs, err := e.svc.Resolve(context.Background(), itOwner, domain.PurposeEvidence, "evidence", []domain.Ref{{PublicID: sig.PublicID}})
	if err != nil || len(imgs) != 1 || imgs[0].Delivery != domain.DeliveryAuthenticated || imgs[0].Format != "webp" {
		t.Fatalf("Resolve = %+v, %v", imgs, err)
	}
	urls, err := e.svc.URLs(imgs[0])
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(urls.URL)
	if err != nil || u.Host != "api.cloudinary.com" || u.Path != "/v1_1/"+itCloud+"/image/download" {
		t.Fatalf("URL de evidencia = %q", urls.URL)
	}
	q := u.Query()
	exp, _ := strconv.ParseInt(q.Get("expires_at"), 10, 64)
	ts, _ := strconv.ParseInt(q.Get("timestamp"), 10, 64)
	if ts != itNow.Unix() || exp-ts != 600 {
		t.Errorf("expires_at - timestamp = %d (timestamp %d)", exp-ts, ts)
	}
	params := map[string]string{}
	for k := range q {
		if k != "signature" && k != "api_key" {
			params[k] = q.Get(k)
		}
	}
	if q.Get("signature") != cloudinary.Sign(params, itAPISecret) || q.Get("type") != "authenticated" || q.Get("public_id") != sig.PublicID {
		t.Errorf("firma de la URL de descarga inválida: %q", urls.URL)
	}
	if strings.Contains(urls.URL, itAPISecret) {
		t.Error("el API secret no debe aparecer en la URL")
	}
}
