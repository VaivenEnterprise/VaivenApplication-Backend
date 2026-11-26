package http

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/contract"
)

func TestLab018HealthzContractWithGzip(t *testing.T) {
	t.Setenv(contract.CompressEnv, "1")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := contract.Wrap(t, NewRouter(Deps{Log: log}), "")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz: HTTP %d", rec.Code)
	}
	if encoding := rec.Header().Get("Content-Encoding"); encoding != "" {
		t.Errorf("la respuesta pequeña debe conservar identity: %q", encoding)
	}
}
