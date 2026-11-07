package config

// TLS son las rutas del certificado y la clave para servir HTTPS (RF-S7).
type TLS struct {
	CertFile string
	KeyFile  string
}

// Enabled indica si la API debe servir HTTPS.
func (t TLS) Enabled() bool {
	return t.CertFile != "" && t.KeyFile != ""
}

// loadTLS exige el certificado en staging y production: ahí la API solo sirve
// HTTPS (Art. XIV.4). En desarrollo y pruebas es opcional, pero si se da uno
// hay que dar ambos.
func loadTLS(r *reader, appEnv string) TLS {
	if isDeployed(appEnv) {
		return TLS{CertFile: r.required("TLS_CERT_FILE"), KeyFile: r.required("TLS_KEY_FILE")}
	}
	t := TLS{CertFile: r.text("TLS_CERT_FILE", ""), KeyFile: r.text("TLS_KEY_FILE", "")}
	if (t.CertFile == "") != (t.KeyFile == "") {
		r.fail("TLS_CERT_FILE/TLS_KEY_FILE", "hay que definir ambas o ninguna")
	}
	return t
}
