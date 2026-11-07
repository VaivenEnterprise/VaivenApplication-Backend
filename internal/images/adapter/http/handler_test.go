package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/middleware"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/platform/redis"
)

type fakeService struct {
	err        error
	gotUser    string
	gotPurpose domain.Purpose
}

func (f *fakeService) Signature(_ context.Context, userID string, p domain.Purpose) (domain.UploadSignature, error) {
	f.gotUser, f.gotPurpose = userID, p
	if f.err != nil {
		return domain.UploadSignature{}, f.err
	}
	if !p.Valid() {
		return domain.UploadSignature{}, domain.ErrInvalidPurpose
	}
	return domain.UploadSignature{
		UploadURL: "https://api.cloudinary.com/v1_1/nube/image/upload", APIKey: "clave", Timestamp: 1762480800,
		Signature: "firma", PublicID: "vaiven/test/" + string(p) + "/" + userID + "/x", Type: p.Delivery().CloudinaryType(),
		AllowedFormats: domain.AllowedFormats, Overwrite: "false",
	}, nil
}

func (*fakeService) Resolve(context.Context, string, domain.Purpose, string, []domain.Ref) ([]domain.Image, error) {
	return nil, nil
}

func (*fakeService) URLs(domain.Image) (domain.URLs, error) { return domain.URLs{}, nil }

// fakeAuth autentica si llega «Authorization: Bearer <userId>».
func fakeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || id == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), auth.Identity{UserID: id, Role: "user"})))
	})
}

func newRouter(t *testing.T, svc *fakeService, perHour int) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logBuf bytes.Buffer
	m, err := NewModule(ModuleDeps{
		Service: svc, Authenticate: fakeAuth, Log: slog.New(slog.NewJSONHandler(&logBuf, nil)),
		Limiter: redis.NewMemoryLimiter(), SignaturesPerHour: perHour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "images" {
		t.Error("Name")
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Route("/api/v1", m.Register)
	return r, &logBuf
}

func post(h http.Handler, body, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/signature", strings.NewReader(body))
	req.RemoteAddr = "10.0.0.1:1"
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func code(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct{ Code, RequestID string } `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.RequestID == "" {
		t.Fatalf("no es el envoltorio de error: %s", rec.Body.String())
	}
	return env.Error.Code
}

func TestSignatureOK(t *testing.T) { // CA-01
	svc := &fakeService{}
	h, _ := newRouter(t, svc, 30)
	rec := post(h, `{"purpose":"item_photo"}`, "u1")
	if rec.Code != http.StatusOK || svc.gotUser != "u1" || svc.gotPurpose != domain.PurposeItemPhoto {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	for _, k := range []string{"uploadUrl", "apiKey", "timestamp", "signature", "publicId", "type", "allowedFormats", "overwrite"} {
		if _, ok := got[k]; !ok {
			t.Errorf("falta %s en %v", k, got)
		}
	}
	if got["type"] != "upload" || !strings.Contains(got["publicId"].(string), "/u1/") || got["overwrite"] != "false" || len(got) != 8 {
		t.Errorf("respuesta = %v", got)
	}
	if rec := post(h, `{"purpose":"evidence"}`, "u1"); !strings.Contains(rec.Body.String(), `"type":"authenticated"`) {
		t.Errorf("evidencia = %s", rec.Body.String())
	}
}

func TestSignatureErrors(t *testing.T) {
	svc := &fakeService{}
	h, logBuf := newRouter(t, svc, 30)
	if rec := post(h, `{"purpose":"item_photo"}`, ""); rec.Code != http.StatusUnauthorized { // CA-05
		t.Errorf("sin sesión = %d", rec.Code)
	}
	for name, body := range map[string]string{"propósito inválido": `{"purpose":"avatar"}`, "sin propósito": `{}`, "JSON roto": `{`, "dos objetos": `{}{}`} {
		if rec := post(h, body, "u1"); rec.Code != http.StatusBadRequest || code(t, rec) != "validation_failed" {
			t.Errorf("%s = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	svc.err = errors.New("falla interna con datos")
	rec := post(h, `{"purpose":"item_photo"}`, "u1")
	if rec.Code != http.StatusInternalServerError || code(t, rec) != "internal_error" || strings.Contains(rec.Body.String(), "datos") {
		t.Errorf("falla interna = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logBuf.String(), "images: error interno") {
		t.Error("el error interno debe quedar en el log")
	}
	svc.err = domain.ErrUnavailable
	if rec := post(h, `{"purpose":"item_photo"}`, "u1"); rec.Code != http.StatusServiceUnavailable || code(t, rec) != "service_unavailable" {
		t.Errorf("Cloudinary caído = %d", rec.Code)
	}
}

func TestSignatureRateLimitPerUser(t *testing.T) { // RF-04
	h, _ := newRouter(t, &fakeService{}, 3)
	for i := 1; i <= 3; i++ {
		if rec := post(h, `{"purpose":"item_photo"}`, "u1"); rec.Code != http.StatusOK {
			t.Fatalf("firma %d = %d", i, rec.Code)
		}
	}
	rec := post(h, `{"purpose":"item_photo"}`, "u1")
	if rec.Code != http.StatusTooManyRequests || code(t, rec) != "rate_limited" || rec.Header().Get("Retry-After") != "3600" {
		t.Errorf("4.ª firma = %d (Retry-After %q)", rec.Code, rec.Header().Get("Retry-After"))
	}
	// El límite es por persona, no por IP.
	if rec := post(h, `{"purpose":"item_photo"}`, "u2"); rec.Code != http.StatusOK {
		t.Errorf("otra persona desde la misma IP = %d", rec.Code)
	}
}

func TestNewModuleRequiresDeps(t *testing.T) {
	if _, err := NewModule(ModuleDeps{}); err == nil {
		t.Error("NewModule sin dependencias debe fallar")
	}
}
