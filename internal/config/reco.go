package config

// Reco es la configuración de las recomendaciones con ML (spec 015).
type Reco struct {
	// Enabled activa «Recomendado para ti» (RECO_ENABLED, RF-10). Apagado por
	// defecto; apagado, las rutas responden service_unavailable.
	Enabled bool
}

// loadReco lee RECO_ENABLED. El resto de los parámetros del modelo son
// constantes documentadas en internal/reco/domain (spec 015, T002).
func loadReco(r *reader) Reco {
	return Reco{Enabled: r.boolean("RECO_ENABLED", false)}
}
