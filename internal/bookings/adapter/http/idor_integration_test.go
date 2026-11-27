//go:build integration

package http

import (
	"fmt"
	"testing"
)

// denied indica que la petición se rechazó por dueño: 403 o 404 según el
// contrato (el 404 evita revelar que el recurso existe).
func denied(code int) bool { return code == 403 || code == 404 }

// TestIntegrationBookingIDOR (spec 020, T004; RF-15): una tercera persona, y
// quien solicita en las acciones de quien presta, no pueden leer ni cambiar
// una reserva ajena; la reserva queda intacta.
func TestIntegrationBookingIDOR(t *testing.T) {
	h, u, _ := router(t)
	own, req, other := u["own"], u["req"], u["r3"]

	code, item := call(t, h, own, "POST", "/items", `{"title":"Taladro","description":"x","category":"hogar","condition":"good",
		"modality":"rent","declaredValueCents":100000,"pricePerDayCents":3000,"priceSource":"manual",
		"zone":{"municipality":"Coyoacán"},"photos":[{"publicId":"p1"}]}`)
	if code != 201 {
		t.Fatalf("crear objeto = %d %v", code, item)
	}
	s, e := day(3), day(5)
	code, b := call(t, h, req, "POST", "/bookings", fmt.Sprintf(
		`{"itemId":%q,"startDate":%q,"endDate":%q,"paymentMode":"split_50_50","acceptedTotalCents":9000}`, item["id"], s, e))
	if code != 201 {
		t.Fatalf("solicitar = %d %v", code, b)
	}
	path := "/bookings/" + b["id"].(string)

	attacks := []struct{ name, method, path, body string }{
		{"leer", "GET", path, ""},
		{"aceptar", "POST", path + "/accept", `{"meetingPoint":{"description":"Metro"}}`},
		{"rechazar", "POST", path + "/reject", `{"reason":"no"}`},
		{"cancelar", "POST", path + "/cancel", `{"reason":"no"}`},
		{"punto de encuentro", "POST", path + "/meeting-point", `{"description":"Metro"}`},
	}
	for _, a := range attacks {
		t.Run("tercera persona "+a.name, func(t *testing.T) {
			if code, out := call(t, h, other, a.method, a.path, a.body); !denied(code) {
				t.Errorf("%s %s por una tercera persona = %d %v; debía ser 403 o 404", a.method, a.path, code, out)
			}
		})
	}
	// Quien solicita no puede hacer lo que solo hace quien presta.
	for _, a := range attacks[1:3] {
		t.Run("solicitante intenta "+a.name, func(t *testing.T) {
			if code, out := call(t, h, req, a.method, a.path, a.body); !denied(code) {
				t.Errorf("%s %s por quien solicita = %d %v; debía ser 403 o 404", a.method, a.path, code, out)
			}
		})
	}
	// Nada de lo anterior cambió el estado.
	if code, got := call(t, h, own, "GET", path, ""); code != 200 || got["status"] != "pending" {
		t.Errorf("la reserva cambió: %d %v", code, got["status"])
	}
	// No aparece en las listas de la tercera persona.
	_, mine := call(t, h, other, "GET", "/me/bookings", "")
	if items, _ := mine["items"].([]any); len(items) != 0 {
		t.Errorf("la tercera persona ve reservas ajenas: %v", items)
	}
}
