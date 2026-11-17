package config

import "strings"

// loadDocs decide si se sirve Swagger UI en /docs (DOCS_ENABLED). Solo se
// permite en development y staging, donde está activo por defecto; en test y
// production queda apagado y pedirlo es un error de configuración, para que
// producción nunca exponga la documentación interactiva.
func loadDocs(r *reader, appEnv string) bool {
	allowed := appEnv == EnvDevelopment || appEnv == EnvStaging
	enabled := r.boolean("DOCS_ENABLED", allowed)
	if enabled && !allowed {
		r.fail("DOCS_ENABLED", "solo se permite en development y staging")
		return false
	}
	return enabled
}

// boolean lee true/false (también 1/0); cualquier otro valor es un error.
func (r *reader) boolean(key string, def bool) bool {
	v, ok := r.value(key)
	if !ok {
		return def
	}
	switch strings.ToLower(v) {
	case "true", "1":
		return true
	case "false", "0":
		return false
	}
	r.fail(key, "debe ser true o false")
	return def
}
