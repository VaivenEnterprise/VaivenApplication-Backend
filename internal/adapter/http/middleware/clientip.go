package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP corrige r.RemoteAddr cuando la conexión viene de un proxy de
// confianza (spec 018, prototipo de balanceo; propuesta no aprobada). Sin
// redes de confianza no hace nada: la API sigue usando la IP de la conexión,
// como hoy (Art. XIV.4).
//
// Con un proxy de confianza delante, recorre X-Forwarded-For de derecha a
// izquierda y toma la primera dirección que no es de confianza: es la última
// que agregó un proxy propio, así que un cliente no puede falsificarla
// anteponiendo valores. Si la conexión no viene de un proxy de confianza, la
// cabecera se ignora por completo. Todos los límites por IP (global, auth,
// imágenes) usan RemoteAddr, así que quedan bien sin cambiar cada módulo.
func ClientIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(trusted) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer, ok := addrOf(r.RemoteAddr)
			if ok && isTrusted(trusted, peer) {
				if client, found := fromForwarded(trusted, r.Header.Values("X-Forwarded-For")); found {
					r2 := r.Clone(r.Context())
					r2.RemoteAddr = net.JoinHostPort(client.String(), "0")
					r = r2
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func addrOf(remote string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	// Sin zona IPv6 (spec 020, hallazgo de fuzzing): netip.ParseAddr acepta
	// «::%[», y Prefix.Contains nunca considera de confianza una dirección con zona.
	return a.Unmap().WithZone(""), true
}

func isTrusted(trusted []netip.Prefix, a netip.Addr) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// fromForwarded devuelve la dirección más a la derecha que no es de confianza.
// Un valor mal formado corta la búsqueda: lo que está a su izquierda no es
// fiable.
func fromForwarded(trusted []netip.Prefix, values []string) (netip.Addr, bool) {
	var hops []string
	for _, v := range values {
		hops = append(hops, strings.Split(v, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return netip.Addr{}, false
		}
		a = a.Unmap().WithZone("") // la zona IPv6 no es parte de la identidad (spec 020)
		if !isTrusted(trusted, a) {
			return a, true
		}
	}
	return netip.Addr{}, false
}
