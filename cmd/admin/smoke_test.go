package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// fakeSmokeAPI simula la API y el endpoint de subida de Cloudinary.
type fakeSmokeAPI struct {
	mu        sync.Mutex
	env       string
	calls     []string
	failOn    string // "METHOD /ruta" que responde 500
	srv       *httptest.Server
	itemAlive bool
	deletedMe bool
}

func newFakeSmokeAPI(t *testing.T, env string) *fakeSmokeAPI {
	t.Helper()
	f := &fakeSmokeAPI{env: env}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSmokeAPI) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := r.Method + " " + r.URL.Path
	if strings.HasPrefix(r.URL.Path, "/api/v1/items/") {
		key = r.Method + " /api/v1/items/{id}"
	}
	f.calls = append(f.calls, key)
	if key == f.failOn {
		http.Error(w, `{"error":{"code":"internal"}}`, http.StatusInternalServerError)
		return
	}
	authed := r.Header.Get("Authorization") == "Bearer tok-login" || r.Header.Get("Authorization") == "Bearer tok-reg"
	writeJSON := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch key {
	case "GET /api/v1/meta":
		writeJSON(200, map[string]string{"environment": f.env, "version": "abc1234"})
	case "POST /api/v1/auth/register":
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if !strings.HasSuffix(in["email"].(string), "@vaiven.invalid") || in["acceptTerms"] != true || in["isAdult"] != true {
			writeJSON(400, map[string]any{"error": map[string]string{"code": "validation"}})
			return
		}
		writeJSON(201, map[string]any{"tokens": map[string]string{"accessToken": "tok-reg"}})
	case "POST /api/v1/auth/login":
		writeJSON(200, map[string]any{"tokens": map[string]string{"accessToken": "tok-login"}})
	case "POST /api/v1/uploads/signature":
		if !authed {
			w.WriteHeader(401)
			return
		}
		writeJSON(200, map[string]any{
			"uploadUrl": f.srv.URL + "/cloudinary/upload", "apiKey": "k", "timestamp": 1,
			"signature": "s", "publicId": "vaiven/staging/item_photo/u1/abc", "type": "upload",
			"allowedFormats": "jpg", "overwrite": "false",
		})
	case "POST /cloudinary/upload":
		if err := r.ParseMultipartForm(1 << 20); err != nil || r.FormValue("public_id") == "" {
			w.WriteHeader(400)
			return
		}
		writeJSON(200, map[string]string{"public_id": r.FormValue("public_id")})
	case "POST /api/v1/items":
		if !authed {
			w.WriteHeader(401)
			return
		}
		f.itemAlive = true
		writeJSON(201, map[string]string{"id": "item1"})
	case "GET /api/v1/items/{id}":
		writeJSON(200, map[string]string{"id": "item1"})
	case "DELETE /api/v1/items/{id}":
		f.itemAlive = false
		w.WriteHeader(204)
	case "DELETE /api/v1/me":
		f.deletedMe = true
		w.WriteHeader(204)
	default:
		w.WriteHeader(404)
	}
}

type fakeDestroyer struct{ destroyed []string }

func (d *fakeDestroyer) Destroy(_ context.Context, ref imagesdomain.Ref) error {
	d.destroyed = append(d.destroyed, ref.PublicID)
	return nil
}

func smokeCfg(env string) config.Config {
	var c config.Config
	c.AppEnv = env
	return c
}

func runFakeSmoke(t *testing.T, f *fakeSmokeAPI, cfg config.Config, args ...string) (int, string, *fakeDestroyer) {
	t.Helper()
	d := &fakeDestroyer{}
	open := func(config.Config, string, string) (smokeDeps, error) {
		return smokeDeps{client: f.srv.Client(), photos: d, random: bytes.NewReader(bytes.Repeat([]byte{7}, 64))}, nil
	}
	var out, errOut bytes.Buffer
	if len(args) == 0 {
		args = []string{"--base-url", f.srv.URL}
	}
	code := runSmoke(args, &out, &errOut, cfg, open)
	return code, out.String() + errOut.String(), d
}

func TestSmokeHappyPathCleansUp(t *testing.T) {
	f := newFakeSmokeAPI(t, "staging")
	code, out, d := runFakeSmoke(t, f, smokeCfg(config.EnvStaging))
	if code != 0 {
		t.Fatalf("código %d, salida:\n%s", code, out)
	}
	if f.itemAlive || !f.deletedMe || len(d.destroyed) != 1 {
		t.Fatalf("no limpió todo: objeto vivo=%v baja=%v fotos borradas=%v", f.itemAlive, f.deletedMe, d.destroyed)
	}
	for _, want := range []string{"OK       registro", "OK       publicar objeto", "OK       borrar objeto", "OK       limpieza: foto", "OK       limpieza: baja de la cuenta"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
	if strings.Contains(out, "smoke-") || strings.Contains(out, "tok-") {
		t.Errorf("la salida expone el correo o el token:\n%s", out)
	}
}

func TestSmokeRefusesProduction(t *testing.T) {
	f := newFakeSmokeAPI(t, "production")
	code, out, _ := runFakeSmoke(t, f, smokeCfg(config.EnvStaging))
	if code != 2 {
		t.Fatalf("código %d, se esperaba 2:\n%s", code, out)
	}
	for _, c := range f.calls {
		if c != "GET /api/v1/meta" {
			t.Fatalf("escribió contra producción: %v", f.calls)
		}
	}
}

func TestSmokeRefusesProductionConfig(t *testing.T) {
	f := newFakeSmokeAPI(t, "staging")
	code, _, _ := runFakeSmoke(t, f, smokeCfg(config.EnvProduction))
	if code != 2 || len(f.calls) != 0 {
		t.Fatalf("código %d, llamadas %v", code, f.calls)
	}
}

func TestSmokeFailureStillCleansUp(t *testing.T) {
	f := newFakeSmokeAPI(t, "staging")
	f.failOn = "GET /api/v1/items/{id}"
	code, out, d := runFakeSmoke(t, f, smokeCfg(config.EnvStaging))
	if code != 1 {
		t.Fatalf("código %d:\n%s", code, out)
	}
	if f.itemAlive || !f.deletedMe || len(d.destroyed) != 1 {
		t.Fatalf("no limpió tras el fallo: objeto vivo=%v baja=%v fotos=%v\n%s", f.itemAlive, f.deletedMe, d.destroyed, out)
	}
	if !strings.Contains(out, "FALLA    leer objeto") {
		t.Errorf("no informa el paso fallido:\n%s", out)
	}
}

func TestSmokeRejectsBadArgs(t *testing.T) {
	f := newFakeSmokeAPI(t, "staging")
	for _, args := range [][]string{
		{"--base-url", "http://insegura.example"},
		{"--base-url", ""},
		{"--base-url", f.srv.URL, "extra"},
		{"--desconocida"},
	} {
		if code, _, _ := runFakeSmoke(t, f, smokeCfg(config.EnvStaging), args...); code != 2 {
			t.Errorf("args %v: código %d, se esperaba 2", args, code)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("hizo peticiones con argumentos inválidos: %v", f.calls)
	}
}

// --connect-to: conecta a otra dirección pero valida el certificado con el
// nombre de --base-url (como curl --resolve).
func TestSmokeConnectToKeepsTLSName(t *testing.T) {
	f := newFakeSmokeAPI(t, "staging")
	addr := strings.TrimPrefix(f.srv.URL, "https://")
	_, port, _ := strings.Cut(addr, ":")
	cfg := smokeCfg(config.EnvStaging)
	cfg.Cloudinary.CloudName = "nube"
	deps, err := newSmokeDeps(cfg, addr, "example.com:"+port)
	if err != nil {
		t.Fatal(err)
	}
	tr := deps.client.Transport.(*http.Transport)
	tr.TLSClientConfig = &tls.Config{RootCAs: f.srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs, MinVersion: tls.VersionTLS12}
	res, err := deps.client.Get("https://example.com:" + port + "/api/v1/meta")
	if err != nil {
		t.Fatalf("no conectó por --connect-to con el nombre del certificado: %v", err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("estado %d", res.StatusCode)
	}
	if _, err := url.Parse(f.srv.URL); err != nil {
		t.Fatal(err)
	}
}

func TestSmokeNeedsCloudinaryForCleanup(t *testing.T) {
	if _, err := newSmokeDeps(smokeCfg(config.EnvStaging), "", "h:443"); err == nil {
		t.Fatal("sin Cloudinary no podría borrar la foto: debe negarse")
	}
}
