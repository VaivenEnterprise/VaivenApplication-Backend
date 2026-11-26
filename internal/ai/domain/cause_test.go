package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestCauseErrorKeepsSentinelAndExposesCause(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		base   error
		cause  Cause
		status int
		text   string
	}{
		{"estado", WithCause(ErrUnavailable, CauseHTTPStatus, 429), ErrUnavailable, CauseHTTPStatus, 429, "ai: proveedor no disponible (causa http_status, HTTP 429)"},
		{"tiempo", WithCause(ErrUnavailable, CauseTimeout, 0), ErrUnavailable, CauseTimeout, 0, "ai: proveedor no disponible (causa timeout)"},
		{"filtro envuelto", fmt.Errorf("x: %w", WithCause(ErrBlocked, CauseContentFilter, 0)), ErrBlocked, CauseContentFilter, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !errors.Is(c.err, c.base) {
				t.Errorf("debe seguir siendo %v", c.base)
			}
			cause, status, ok := CauseOf(c.err)
			if !ok || cause != c.cause || status != c.status {
				t.Errorf("CauseOf = %v %d %v", cause, status, ok)
			}
			if c.text != "" && c.err.Error() != c.text {
				t.Errorf("texto = %q", c.err.Error())
			}
		})
	}
	if _, _, ok := CauseOf(ErrUnavailable); ok {
		t.Error("un error sin causa no debe reportarla")
	}
}
