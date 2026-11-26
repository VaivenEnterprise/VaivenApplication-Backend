package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/suggest/usecase"
)

// T018 (spec 014): el registro lleva la causa; la respuesta no cambia.
func TestProviderFailureLogsCauseAndKeepsResponse(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantCause  string
		wantStatus string // vacío = sin upstreamStatus
	}{
		{"estado 429", aidomain.WithCause(aidomain.ErrUnavailable, aidomain.CauseHTTPStatus, 429), `"cause":"http_status"`, `"upstreamStatus":429`},
		{"tiempo", aidomain.WithCause(aidomain.ErrUnavailable, aidomain.CauseTimeout, 0), `"cause":"timeout"`, ""},
		{"vacío", aidomain.WithCause(aidomain.ErrUnavailable, aidomain.CauseEmptyResponse, 0), `"cause":"empty_response"`, ""},
		{"JSON malo", aidomain.WithCause(aidomain.ErrMalformed, aidomain.CauseMalformedJSON, 0), `"cause":"malformed_json"`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, logs := newRouter(t, &fakeSvc{err: usecase.ErrNoProposal.Wrap(c.err)})
			rec := post(h, `{"imagePublicId":"`+secretPhoto+`"}`, "u1")
			if rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "service_unavailable" {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			var body struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if body.Error.Message != usecase.ErrNoProposal.Message {
				t.Errorf("mensaje = %q", body.Error.Message)
			}
			out := logs.String()
			if !strings.Contains(out, c.wantCause) {
				t.Errorf("falta %s en %s", c.wantCause, out)
			}
			if c.wantStatus != "" && !strings.Contains(out, c.wantStatus) {
				t.Errorf("falta %s en %s", c.wantStatus, out)
			}
			if c.wantStatus == "" && strings.Contains(out, "upstreamStatus") {
				t.Errorf("no debe haber upstreamStatus: %s", out)
			}
			if strings.Contains(out, "foto-secreta") {
				t.Errorf("el publicId llegó a los registros: %s", out)
			}
		})
	}
}
