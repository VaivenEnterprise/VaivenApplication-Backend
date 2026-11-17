package http

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeSelfSigned escribe un par autofirmado para 127.0.0.1 con el número de
// serie dado y fija la fecha de modificación de ambos archivos.
func writeSelfSigned(t *testing.T, dir string, serial int64, mod time.Time) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "vaiven-test"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile := filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{certFile, keyFile} {
		if err := os.Chtimes(f, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	return certFile, keyFile
}

// servedSerial abre una conexión TLS y devuelve el número de serie servido.
func servedSerial(t *testing.T, addr string) int64 {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- certificado autofirmado de prueba
	if err != nil {
		t.Fatalf("tls.Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	return conn.ConnectionState().PeerCertificates[0].SerialNumber.Int64()
}

// CA-S4: el certificado renovado en disco se sirve sin reiniciar el servidor.
func TestTLSReloadsRenewedCertificate(t *testing.T) {
	dir := t.TempDir()
	start := time.Now().Add(-time.Minute).Truncate(time.Second)
	certFile, keyFile := writeSelfSigned(t, dir, 1, start)

	clock := start
	r, err := newCertReloader(certFile, keyFile, 30*time.Second, func() time.Time { return clock })
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
		ReadHeaderTimeout: time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: r.getCertificate},
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.ServeTLS(ln, "", "") }()
	t.Cleanup(func() { _ = srv.Close() })
	addr := ln.Addr().String()

	if got := servedSerial(t, addr); got != 1 {
		t.Fatalf("serie inicial = %d, want 1", got)
	}

	// certbot renueva: nuevo par con otra fecha de modificación.
	writeSelfSigned(t, dir, 2, start.Add(time.Minute))
	clock = start.Add(10 * time.Second) // antes del intervalo: aún el anterior
	if got := servedSerial(t, addr); got != 1 {
		t.Errorf("antes del intervalo = %d, want 1", got)
	}
	clock = start.Add(31 * time.Second) // pasó el intervalo: recarga
	if got := servedSerial(t, addr); got != 2 {
		t.Errorf("tras el intervalo = %d, want 2 (recarga sin reiniciar)", got)
	}
}

func TestTLSKeepsOldCertificateIfReloadFails(t *testing.T) {
	dir := t.TempDir()
	start := time.Now().Add(-time.Minute).Truncate(time.Second)
	certFile, keyFile := writeSelfSigned(t, dir, 7, start)
	clock := start
	r, err := newCertReloader(certFile, keyFile, time.Second, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}

	// Archivo a medio escribir: contenido inválido con fecha nueva.
	if err := os.WriteFile(certFile, []byte("roto"), 0o600); err != nil {
		t.Fatal(err)
	}
	clock = start.Add(2 * time.Second)
	c, err := r.getCertificate(nil)
	if err != nil || c == nil {
		t.Fatalf("getCertificate: %v", err)
	}
	if leaf, _ := x509.ParseCertificate(c.Certificate[0]); leaf.SerialNumber.Int64() != 7 {
		t.Errorf("debe seguir sirviendo el certificado anterior, serie = %d", leaf.SerialNumber.Int64())
	}

	// Si el archivo desaparece, también conserva el anterior.
	_ = os.Remove(keyFile)
	clock = start.Add(4 * time.Second)
	if c, _ := r.getCertificate(nil); c == nil {
		t.Error("sin la clave en disco debe conservar el certificado cargado")
	}
}

func TestConfigureTLS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeSelfSigned(t, dir, 3, time.Now())

	srv := &http.Server{ReadHeaderTimeout: time.Second}
	if err := ConfigureTLS(srv, certFile, keyFile); err != nil {
		t.Fatalf("ConfigureTLS: %v", err)
	}
	if srv.TLSConfig == nil || srv.TLSConfig.MinVersion != tls.VersionTLS12 || srv.TLSConfig.GetCertificate == nil {
		t.Errorf("TLSConfig = %+v", srv.TLSConfig)
	}
}

func TestConfigureTLSRejectsMissingOrInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeSelfSigned(t, dir, 4, time.Now())
	bad := filepath.Join(dir, "malo.pem")
	if err := os.WriteFile(bad, []byte("no es pem"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct{ name, cert, key, want string }{
		{"sin certificado", filepath.Join(dir, "no-existe.pem"), keyFile, "certificado"},
		{"sin clave", certFile, filepath.Join(dir, "no-existe.pem"), "clave"},
		{"contenido inválido", bad, keyFile, "no se pudo cargar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ConfigureTLS(&http.Server{ReadHeaderTimeout: time.Second}, tt.cert, tt.key)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want que contenga %q", err, tt.want)
			}
			if err != nil && strings.Contains(err.Error(), "PRIVATE KEY") {
				t.Errorf("el error muestra contenido de la clave: %v", err)
			}
		})
	}
}
