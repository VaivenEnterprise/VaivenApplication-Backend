package http

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// CertReloadInterval es cada cuánto se comprueba si el certificado cambió en
// disco (RF-S7: ≤ 1 min). certbot lo renueva y la API lo usa sin reiniciarse.
const CertReloadInterval = 30 * time.Second

// ConfigureTLS prepara srv para servir solo HTTPS con el par certFile/keyFile y
// recargarlo cuando cambie su fecha de modificación (plan P-S8, Art. XIV.4).
// Falla si el par no se puede cargar al arrancar. Después se sirve con
// srv.ListenAndServeTLS("", "").
func ConfigureTLS(srv *http.Server, certFile, keyFile string) error {
	r, err := newCertReloader(certFile, keyFile, CertReloadInterval, time.Now)
	if err != nil {
		return err
	}
	srv.TLSConfig = &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: r.getCertificate,
	}
	return nil
}

// certReloader entrega el certificado vigente y lo relee si cambió en disco.
type certReloader struct {
	certFile, keyFile string
	interval          time.Duration
	now               func() time.Time

	mu        sync.Mutex
	cert      *tls.Certificate
	certMod   time.Time
	keyMod    time.Time
	lastCheck time.Time
}

func newCertReloader(certFile, keyFile string, interval time.Duration, now func() time.Time) (*certReloader, error) {
	r := &certReloader{certFile: certFile, keyFile: keyFile, interval: interval, now: now}
	if err := r.load(); err != nil {
		return nil, err
	}
	r.lastCheck = now()
	return r, nil
}

// load lee el par y guarda sus fechas de modificación. Los errores nombran los
// archivos, nunca su contenido.
func (r *certReloader) load() error {
	certMod, keyMod, err := r.modTimes()
	if err != nil {
		return err
	}
	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return fmt.Errorf("tls: no se pudo cargar el certificado %s: %w", r.certFile, err)
	}
	r.cert, r.certMod, r.keyMod = &cert, certMod, keyMod
	return nil
}

func (r *certReloader) modTimes() (time.Time, time.Time, error) {
	c, err := os.Stat(r.certFile)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("tls: certificado: %w", err)
	}
	k, err := os.Stat(r.keyFile)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("tls: clave: %w", err)
	}
	return c.ModTime(), k.ModTime(), nil
}

// getCertificate es tls.Config.GetCertificate. Como mucho una vez por
// intervalo comprueba las fechas; si cambiaron, recarga. Si la recarga falla
// (p. ej. archivos a medio escribir) sigue sirviendo el certificado anterior y
// lo reintenta en el siguiente intervalo.
func (r *certReloader) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if now := r.now(); now.Sub(r.lastCheck) >= r.interval {
		r.lastCheck = now
		if certMod, keyMod, err := r.modTimes(); err == nil &&
			(!certMod.Equal(r.certMod) || !keyMod.Equal(r.keyMod)) {
			_ = r.load()
		}
	}
	return r.cert, nil
}
