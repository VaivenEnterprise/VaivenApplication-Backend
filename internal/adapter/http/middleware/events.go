package middleware

import (
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
)

// securityEvent registra un evento de seguridad estructurado (spec 018, C-02;
// propuesta no aprobada). Campos: event, result, severity, category,
// requestId y subject (seudónimo). Nunca lleva tokens, contraseñas, correos,
// cabeceras Authorization ni cookies; las IP salen truncadas (/24 en IPv4,
// /48 en IPv6). La marca temporal la pone slog.
func securityEvent(r *http.Request, log *slog.Logger, event, result, severity, category, key string) {
	if log == nil {
		return
	}
	log.LogAttrs(r.Context(), slog.LevelWarn, "security_event",
		slog.String("event", event),
		slog.String("result", result),
		slog.String("severity", severity),
		slog.String("category", category),
		slog.String("requestId", respond.RequestID(r.Context())),
		slog.String("subject", pseudonymize(key)),
	)
}

// limitCategory nombra el límite por su prefijo («global» si no tiene).
func limitCategory(prefix string) string {
	if c := strings.TrimSuffix(prefix, ":"); c != "" {
		return c
	}
	return "global"
}

// pseudonymize recibe la clave sin prefijo («user:<id>» o «ip:<dir>»): deja el
// id de usuario (seudónimo) y trunca la IP; cualquier otra cosa se oculta.
func pseudonymize(key string) string {
	if id, ok := strings.CutPrefix(key, "user:"); ok {
		return "user:" + id
	}
	if ip, ok := strings.CutPrefix(key, "ip:"); ok {
		a, err := netip.ParseAddr(ip)
		if err != nil {
			return "ip:invalid"
		}
		a = a.Unmap()
		bits := 24
		if a.Is6() {
			bits = 48
		}
		p, _ := a.Prefix(bits) // bits es válido para la familia
		return "ip:" + p.String()
	}
	return "other"
}
