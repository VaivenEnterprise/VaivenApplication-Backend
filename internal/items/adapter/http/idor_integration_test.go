//go:build integration

package http

import "testing"

// TestIntegrationItemIDOR (spec 020, T004; RF-15): otra persona no puede
// editar, eliminar, pausar ni reanudar un objeto ajeno, y el objeto no cambia.
func TestIntegrationItemIDOR(t *testing.T) {
	h, _ := newIntegrationRouter(t)
	const ana, beto = "ana", "beto"

	code, item := call(t, h, ana, "POST", "/items", `{"title":"Taladro","description":"x","category":"hogar","condition":"good",
		"modality":"rent","declaredValueCents":100000,"pricePerDayCents":3000,"priceSource":"manual",
		"zone":{"municipality":"Coyoacán"},"photos":[{"publicId":"p1"}]}`)
	if code != 201 {
		t.Fatalf("crear objeto = %d %v", code, item)
	}
	path := "/items/" + item["id"].(string)

	for _, a := range []struct{ name, method, path, body string }{
		{"editar", "PATCH", path, `{"version":1,"title":"Mío"}`},
		{"eliminar", "DELETE", path, ""},
		{"pausar", "POST", path + "/pause", ""},
		{"reanudar", "POST", path + "/resume", ""},
	} {
		t.Run(a.name, func(t *testing.T) {
			if code, out := call(t, h, beto, a.method, a.path, a.body); code != 403 && code != 404 {
				t.Errorf("%s %s por otra persona = %d %v; debía ser 403 o 404", a.method, a.path, code, out)
			}
		})
	}
	code, got := call(t, h, ana, "GET", path, "")
	if code != 200 || got["title"] != "Taladro" || got["status"] != "active" || got["version"] != float64(1) {
		t.Errorf("el objeto cambió: %d %v", code, got)
	}
	// «Mis objetos» de otra persona no lista los de Ana.
	_, mine := call(t, h, beto, "GET", "/me/items", "")
	if items, _ := mine["items"].([]any); len(items) != 0 {
		t.Errorf("Beto ve objetos de Ana: %v", items)
	}
}
