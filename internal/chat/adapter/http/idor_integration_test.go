//go:build integration

package http

import "testing"

// TestIntegrationChatIDOR (spec 020, T004; RF-15): una persona ajena a la
// conversación no la lista, no lee sus mensajes, no escribe ni la marca leída;
// y lo que intenta no deja rastro para las partes.
func TestIntegrationChatIDOR(t *testing.T) {
	w := newWorld(t)
	_, conv := w.request(t, "req", 3)
	path := "/conversations/" + conv

	if code, m := w.call(t, "req", "POST", path+"/messages", `{"body":"hola"}`); code != 201 {
		t.Fatalf("enviar = %d %v", code, m)
	}
	for _, a := range []struct{ name, method, path, body string }{
		{"leer mensajes", "GET", path + "/messages", ""},
		{"escribir", "POST", path + "/messages", `{"body":"intruso"}`},
		{"marcar leída", "POST", path + "/read", ""},
	} {
		t.Run(a.name, func(t *testing.T) {
			if code, out := w.call(t, "otro", a.method, a.path, a.body); code != 403 && code != 404 {
				t.Errorf("%s %s por una persona ajena = %d %v; debía ser 403 o 404", a.method, a.path, code, out)
			}
		})
	}
	if _, l := w.call(t, "otro", "GET", "/conversations", ""); len(l["items"].([]any)) != 0 {
		t.Errorf("la persona ajena lista conversaciones ajenas: %v", l)
	}
	if _, u := w.call(t, "otro", "GET", "/conversations/unread-count", ""); u["count"] != float64(0) {
		t.Errorf("no leídos de la persona ajena = %v", u)
	}
	// El intento no dejó un mensaje ni marcó nada como leído para las partes.
	_, hist := w.call(t, "own", "GET", path+"/messages", "")
	if n := len(hist["items"].([]any)); n != 1 {
		t.Errorf("mensajes = %d, debía quedar 1 (la persona ajena no escribe)", n)
	}
	if _, u := w.call(t, "own", "GET", "/conversations/unread-count", ""); u["count"] != float64(1) {
		t.Errorf("no leídos de quien presta = %v, debía seguir en 1", u)
	}
}
