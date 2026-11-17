package config

import "strings"

// DefaultExpoPushURL es la API de envío de Expo Push (spec 011, plan P-02).
const DefaultExpoPushURL = "https://exp.host/--/api/v2/push/send"

// Push es la configuración de las notificaciones push (spec 011, plan P-09).
type Push struct {
	// Enabled activa el envío por Expo. Apagado, los avisos solo se guardan.
	Enabled bool
	// URL es el punto de envío de Expo (EXPO_PUSH_URL); en pruebas apunta a un
	// servidor simulado.
	URL string
	// AccessToken es el token de acceso de Expo (EXPO_ACCESS_TOKEN), opcional;
	// es un secreto y nunca se imprime.
	AccessToken Secret
}

// loadPush lee PUSH_ENABLED (activo por omisión solo en staging y production;
// en development y test queda apagado para no llamar a Expo), EXPO_PUSH_URL y
// EXPO_ACCESS_TOKEN.
func loadPush(r *reader, appEnv string) Push {
	p := Push{
		Enabled:     r.boolean("PUSH_ENABLED", isDeployed(appEnv)),
		URL:         r.text("EXPO_PUSH_URL", DefaultExpoPushURL),
		AccessToken: Secret(r.text("EXPO_ACCESS_TOKEN", "")),
	}
	if !strings.HasPrefix(p.URL, "https://") && !strings.HasPrefix(p.URL, "http://") {
		r.fail("EXPO_PUSH_URL", "debe empezar con https:// o http://")
	}
	if p.Enabled && isDeployed(appEnv) && !strings.HasPrefix(p.URL, "https://") {
		r.fail("EXPO_PUSH_URL", "en staging y production debe usar https://")
	}
	return p
}
