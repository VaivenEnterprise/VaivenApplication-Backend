package config

import "strings"

// Redis es la configuración del límite de tasa y la caché (RF-S5, Art. XIV).
// La URL lleva la contraseña, por eso es un Secret.
type Redis struct {
	URL Secret
}

// Enabled indica si hay Redis configurado; sin él se usa la versión en memoria.
func (r Redis) Enabled() bool {
	return r.URL != ""
}

// loadRedis exige REDIS_URL en staging y production; en desarrollo y pruebas
// es opcional.
func loadRedis(r *reader, appEnv string) Redis {
	var url string
	if isDeployed(appEnv) {
		url = r.required("REDIS_URL")
	} else {
		url = r.text("REDIS_URL", "")
	}
	if url != "" && !strings.HasPrefix(url, "redis://") && !strings.HasPrefix(url, "rediss://") {
		// No se muestra el valor: puede llevar la contraseña.
		r.fail("REDIS_URL", "debe empezar con redis:// o rediss://")
	}
	return Redis{URL: Secret(url)}
}
