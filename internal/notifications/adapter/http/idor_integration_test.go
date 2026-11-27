//go:build integration

package http

import (
	"context"
	"testing"
)

// TestIntegrationNotificationsIDOR (spec 020, T004; RF-15): los avisos y los
// tokens de push son de quien los recibe. Otra persona no los ve, no los marca
// como leídos y no borra el token de otra.
func TestIntegrationNotificationsIDOR(t *testing.T) {
	w := newPushWorld(t)
	if code, b := w.call(t, "own", "POST", "/me/push-tokens", `{"token":"`+ownerToken+`","platform":"android"}`); code != 204 {
		t.Fatalf("registrar token = %d %v", code, b)
	}
	w.request(t, 3) // genera un aviso para quien presta (own)

	_, list := w.call(t, "own", "GET", "/me/notifications", "")
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("avisos de own = %v", list)
	}
	noticeID := items[0].(map[string]any)["id"].(string)

	// req no ve el aviso de own.
	if _, mine := w.call(t, "req", "GET", "/me/notifications", ""); len(mine["items"].([]any)) != 0 {
		t.Errorf("req ve avisos ajenos: %v", mine)
	}
	// req intenta marcarlo (por id y con «all»): no cambia el de own.
	w.call(t, "req", "POST", "/me/notifications/read", `{"ids":["`+noticeID+`"]}`)
	w.call(t, "req", "POST", "/me/notifications/read", `{"all":true}`)
	if _, c := w.call(t, "own", "GET", "/me/notifications/unread-count", ""); c["count"] != float64(1) {
		t.Errorf("req marcó como leído el aviso de own: no leídos = %v", c)
	}
	// req intenta borrar el token de own: el token sigue registrado.
	w.call(t, "req", "DELETE", "/me/push-tokens", `{"token":"`+ownerToken+`"}`)
	if got, err := w.tokens.TokensOf(context.Background(), w.ids["own"]); err != nil || len(got) != 1 {
		t.Errorf("req borró el token de own: %v %v", got, err)
	}
}
