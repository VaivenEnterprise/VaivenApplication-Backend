package middleware

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// bigJSON es un listado paginado de unos 6 KiB.
func bigJSON() []byte {
	type item struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	page := struct {
		Items      []item  `json:"items"`
		NextCursor *string `json:"nextCursor"`
	}{}
	for i := 0; i < 100; i++ {
		page.Items = append(page.Items, item{ID: strings.Repeat("a", 24), Title: "Taladro percutor en buen estado"})
	}
	c := "eyJrIjoxfQ"
	page.NextCursor = &c
	b, _ := json.Marshal(page)
	return b
}

func jsonOut(status int, body []byte, extra map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", "999999") // debe desaparecer si se comprime
		for k, v := range extra {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
}

func do(h http.Handler, method, path, accept string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if accept != "" {
		req.Header.Set("Accept-Encoding", accept)
	}
	rec := httptest.NewRecorder()
	Compress(DefaultCompressConfig())(h).ServeHTTP(rec, req)
	return rec
}

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("no es gzip: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip inválido: %v", err)
	}
	return out
}

func sameJSON(t *testing.T, a, b []byte) {
	t.Helper()
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil || !reflect.DeepEqual(x, y) {
		t.Fatalf("el JSON cambió")
	}
}

func TestCompressWithoutAcceptEncodingIsIdentity(t *testing.T) {
	body := bigJSON()
	rec := do(jsonOut(200, body, nil), "GET", "/api/v1/items", "")
	if rec.Header().Get("Content-Encoding") != "" || !bytes.Equal(rec.Body.Bytes(), body) {
		t.Fatal("sin Accept-Encoding no se comprime")
	}
	if rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary = %q", rec.Header().Get("Vary"))
	}
}

func TestCompressGzipPaginatedJSON(t *testing.T) {
	body := bigJSON()
	rec := do(jsonOut(200, body, nil), "GET", "/api/v1/items?limit=100", "gzip, deflate, br")
	h := rec.Header()
	if h.Get("Content-Encoding") != "gzip" || h.Get("Vary") != "Accept-Encoding" || h.Get("Content-Length") != "" {
		t.Fatalf("cabeceras = %v", h)
	}
	if rec.Code != 200 || h.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("código o tipo cambiaron: %d %q", rec.Code, h.Get("Content-Type"))
	}
	plain := gunzip(t, rec.Body.Bytes())
	sameJSON(t, plain, body)
	if !bytes.Equal(plain, body) {
		t.Error("los bytes descomprimidos difieren")
	}
	if rec.Body.Len() >= len(body)/2 {
		t.Errorf("poca reducción: %d → %d", len(body), rec.Body.Len())
	}
}

func TestCompressErrorsKeepStatus(t *testing.T) {
	body := bigJSON() // un 400 grande con muchos detalles
	rec := do(jsonOut(400, body, nil), "POST", "/api/v1/items", "gzip")
	if rec.Code != 400 || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("400 = %d %v", rec.Code, rec.Header())
	}
	sameJSON(t, gunzip(t, rec.Body.Bytes()), body)
}

func TestCompressSmallAndEmptyStayPlain(t *testing.T) {
	small := []byte(`{"status":"ok"}`)
	rec := do(jsonOut(200, small, nil), "GET", "/healthz", "gzip")
	if rec.Header().Get("Content-Encoding") != "" || !bytes.Equal(rec.Body.Bytes(), small) {
		t.Error("lo pequeño sale en claro")
	}
	noContent := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	rec = do(noContent, "DELETE", "/api/v1/items/x", "gzip")
	if rec.Code != 204 || rec.Body.Len() != 0 || rec.Header().Get("Content-Encoding") != "" {
		t.Errorf("204 = %d %q", rec.Code, rec.Body.String())
	}
	nothing := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	rec = do(nothing, "GET", "/api/v1/x", "gzip")
	if rec.Code != 200 || rec.Body.Len() != 0 {
		t.Errorf("vacío = %d", rec.Code)
	}
}

func TestCompressSecurityExclusions(t *testing.T) {
	body := bigJSON()
	for _, path := range []string{"/api/v1/auth/login", "/api/v1/auth/refresh", "/api/v1/me", "/api/v1/uploads/signature", "/api/v1/bookings/abc/delivery/qr", "/api/v1/bookings/abc/return/qr", "/docs", "/docs/openapi.yaml"} {
		rec := do(jsonOut(200, body, nil), "POST", path, "gzip")
		if rec.Header().Get("Content-Encoding") != "" || rec.Header().Get("Vary") != "" || !bytes.Equal(rec.Body.Bytes(), body) {
			t.Errorf("%s no debe comprimirse", path)
		}
	}
	// no-store en cualquier ruta: nunca se comprime.
	rec := do(jsonOut(200, body, map[string]string{"Cache-Control": "no-store"}), "GET", "/api/v1/items", "gzip")
	if rec.Header().Get("Content-Encoding") != "" {
		t.Error("no-store no debe comprimirse")
	}
	// /me/bookings no es /me: sí se comprime (listas propias, sin secretos).
	rec = do(jsonOut(200, body, nil), "GET", "/api/v1/me/bookings", "gzip")
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Error("/me/bookings debe comprimirse")
	}
}

func TestCompressSkipsBinaryAndPreEncoded(t *testing.T) {
	body := bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, 1000)
	rec := do(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}), "GET", "/x.png", "gzip")
	if rec.Header().Get("Content-Encoding") != "" || !bytes.Equal(rec.Body.Bytes(), body) {
		t.Error("binario no se comprime")
	}
	rec = do(jsonOut(200, bigJSON(), map[string]string{"Content-Encoding": "br"}), "GET", "/api/v1/items", "gzip")
	if rec.Header().Get("Content-Encoding") != "br" {
		t.Error("no se recomprime lo ya codificado")
	}
}

func TestCompressHeadAndRefusedGzip(t *testing.T) {
	for _, ae := range []string{"gzip;q=0", "*;q=0.5, gzip;q=0", "gzip;q=0, *", "identity", "br", "gzip;q=abc", "gzip;q=2"} {
		rec := do(jsonOut(200, bigJSON(), nil), "GET", "/api/v1/items", ae)
		if rec.Header().Get("Content-Encoding") != "" {
			t.Errorf("Accept-Encoding %q no acepta gzip", ae)
		}
	}
	rec := do(jsonOut(200, bigJSON(), nil), "HEAD", "/api/v1/items", "gzip")
	if rec.Header().Get("Content-Encoding") != "" {
		t.Error("HEAD no se comprime")
	}
	if !AcceptsGzip("br;q=1, *;q=0.5") || !AcceptsGzip("GZIP") {
		t.Error("* y mayúsculas aceptan gzip")
	}
}

func TestCompressManySmallWritesAndFlush(t *testing.T) {
	body := bigJSON()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for i := 0; i < len(body); i += 100 {
			end := min(i+100, len(body))
			_, _ = w.Write(body[i:end])
		}
		w.(http.Flusher).Flush()
	})
	rec := do(h, "GET", "/api/v1/items", "gzip")
	if !bytes.Equal(gunzip(t, rec.Body.Bytes()), body) {
		t.Error("escrituras en trozos")
	}
	early := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"a":`))
		w.(http.Flusher).Flush() // antes del umbral: sale en claro
		_, _ = w.Write([]byte(`1}`))
	})
	rec = do(early, "GET", "/api/v1/items", "gzip")
	if rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != `{"a":1}` {
		t.Errorf("flush temprano = %q", rec.Body.String())
	}
}

func TestCompressConcurrent(t *testing.T) {
	body := bigJSON()
	h := Compress(DefaultCompressConfig())(jsonOut(200, body, nil))
	srv := httptest.NewServer(h)
	defer srv.Close()
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// El cliente de Go pide gzip y descomprime solo, como un móvil.
			res, err := http.Get(srv.URL + "/api/v1/items")
			if err != nil {
				errs <- err.Error()
				return
			}
			defer res.Body.Close()
			got, _ := io.ReadAll(res.Body)
			if !res.Uncompressed || !bytes.Equal(got, body) {
				errs <- "respuesta distinta"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

func TestCompressConfigDefaults(t *testing.T) {
	h := Compress(CompressConfig{Level: 42})(jsonOut(200, bigJSON(), nil))
	req := httptest.NewRequest("GET", "/api/v1/items", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Error("valores por omisión")
	}
}

func TestCompressHijack(t *testing.T) {
	cw := &compressWriter{ResponseWriter: httptest.NewRecorder()}
	if _, _, err := cw.Hijack(); err == nil {
		t.Error("el recorder no permite hijack")
	}
	if cw.Unwrap() == nil {
		t.Error("Unwrap")
	}
}
