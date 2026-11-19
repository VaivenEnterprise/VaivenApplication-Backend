package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pki genera una CA y certificados de prueba en dir.
type pki struct {
	dir    string
	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey
}

func newPKI(t *testing.T) *pki {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "vaiven-pilot-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	ca, _ := x509.ParseCertificate(der)
	p := &pki{dir: t.TempDir(), caCert: ca, caKey: key}
	writePEM(t, filepath.Join(p.dir, "ca.pem"), "CERTIFICATE", der)
	return p
}

func writePEM(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

// issue emite un certificado firmado por la CA (o autofirmado si foreign).
func (p *pki) issue(t *testing.T, name, cn string, server, foreign bool) tlsFiles {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	if server {
		tpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tpl.DNSNames = []string{cn}
		tpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	} else {
		tpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	parent, signer := p.caCert, p.caKey
	if foreign {
		parent, signer = tpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, parent, &key.PublicKey, signer)
	if err != nil {
		t.Fatal(err)
	}
	kd, _ := x509.MarshalECPrivateKey(key)
	f := tlsFiles{Cert: filepath.Join(p.dir, name+".pem"), Key: filepath.Join(p.dir, name+".key"), CA: filepath.Join(p.dir, "ca.pem")}
	writePEM(t, f.Cert, "CERTIFICATE", der)
	writePEM(t, f.Key, "EC PRIVATE KEY", kd)
	return f
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// echoServer simula MongoDB o Redis: devuelve lo que recibe.
func echoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	return ln.Addr().String()
}

func waitListening(t *testing.T, addr string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if c, err := net.Dial("tcp", addr); err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s no escucha", addr)
}

func roundTrip(addr, msg string) (string, error) {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte(msg + "\n")); err != nil {
		return "", err
	}
	return bufio.NewReader(c).ReadString('\n')
}

func TestTunnelServerClientRoundTrip(t *testing.T) {
	p := newPKI(t)
	srv := p.issue(t, "gw", "vaiven-pilot-gw", true, false)
	cli := p.issue(t, "vm2", "vaiven-pilot-vm2", false, false)
	backend := echoServer(t)
	gw, local := freeAddr(t), freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runServer(ctx, gw, backend, srv, []string{"vaiven-pilot-vm2"}, quietLog()) }()
	waitListening(t, gw)
	go func() { _ = runClient(ctx, local, gw, cli, "vaiven-pilot-gw", quietLog()) }()
	waitListening(t, local)
	got, err := roundTrip(local, "PING")
	if err != nil || strings.TrimSpace(got) != "PING" {
		t.Fatalf("ida y vuelta = %q %v", got, err)
	}
}

func TestTunnelRejectsUnauthorizedClients(t *testing.T) {
	p := newPKI(t)
	srv := p.issue(t, "gw", "vaiven-pilot-gw", true, false)
	backend := echoServer(t)
	gw := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runServer(ctx, gw, backend, srv, []string{"vaiven-pilot-vm2"}, quietLog()) }()
	waitListening(t, gw)
	pool, _ := loadPool(srv.CA)

	try := func(certs []tls.Certificate) error {
		c, err := tls.Dial("tcp", gw, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "vaiven-pilot-gw", Certificates: certs})
		if err != nil {
			return err
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := c.Write([]byte("x\n")); err != nil {
			return err
		}
		_, err = bufio.NewReader(c).ReadString('\n')
		return err
	}
	load := func(f tlsFiles) []tls.Certificate {
		c, _ := tls.LoadX509KeyPair(f.Cert, f.Key)
		return []tls.Certificate{c}
	}
	if try(nil) == nil {
		t.Error("sin certificado de cliente debe rechazarse")
	}
	if try(load(p.issue(t, "otro", "vaiven-pilot-vm2", false, true))) == nil {
		t.Error("certificado de otra CA debe rechazarse")
	}
	if try(load(p.issue(t, "intruso", "otra-vm", false, false))) == nil {
		t.Error("CN no autorizado debe rechazarse")
	}
	if err := try(load(p.issue(t, "vm2", "vaiven-pilot-vm2", false, false))); err != nil {
		t.Errorf("cliente autorizado rechazado: %v", err)
	}
	// TLS 1.2 no se acepta.
	if c, err := tls.Dial("tcp", gw, &tls.Config{MaxVersion: tls.VersionTLS12, RootCAs: pool, ServerName: "vaiven-pilot-gw"}); err == nil {
		_ = c.Close()
		t.Error("TLS 1.2 debe rechazarse")
	}
}

func TestFilesRelayRequiresMTLSAndIsReadOnly(t *testing.T) {
	p := newPKI(t)
	srv := p.issue(t, "gw", "vaiven-pilot-gw", true, false)
	cli := p.issue(t, "vm2", "vaiven-pilot-vm2", false, false)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "image.tar.sha256"), []byte("abc  image.tar\n"), 0o600)
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runFiles(ctx, addr, dir, srv, []string{"vaiven-pilot-vm2"}, quietLog()) }()
	waitListening(t, addr)

	ccfg, _ := clientConfig(cli, "vaiven-pilot-gw")
	good := &http.Client{Transport: &http.Transport{TLSClientConfig: ccfg}, Timeout: 3 * time.Second}
	res, err := good.Get("https://" + addr + "/image.tar.sha256")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("GET con mTLS = %v %v", res, err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(b), "image.tar") {
		t.Errorf("contenido = %q", b)
	}
	if res, err := good.Post("https://"+addr+"/x", "text/plain", strings.NewReader("y")); err != nil || res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST = %v %v", res, err)
	}
	pool, _ := loadPool(cli.CA)
	anon := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "vaiven-pilot-gw"}}, Timeout: 3 * time.Second}
	if res, err := anon.Get("https://" + addr + "/image.tar.sha256"); err == nil {
		_ = res.Body.Close()
		t.Error("sin certificado de cliente el relevo debe rechazar")
	}
}

func TestRunValidatesArguments(t *testing.T) {
	p := newPKI(t)
	srv := p.issue(t, "gw", "vaiven-pilot-gw", true, false)
	ctx := context.Background()
	base := []string{"--cert", srv.Cert, "--key", srv.Key, "--ca", srv.CA}
	cases := map[string][]string{
		"sin modo":           {},
		"modo desconocido":   {"otro", "--listen", "127.0.0.1:1"},
		"faltan llaves":      {"server", "--listen", "127.0.0.1:1"},
		"server sin target":  append([]string{"server", "--listen", "127.0.0.1:1"}, base...),
		"client no loopback": append([]string{"client", "--listen", "0.0.0.0:1", "--target", "x:1", "--server-name", "gw"}, base...),
		"files sin dir":      append([]string{"files", "--listen", "127.0.0.1:1"}, base...),
		"server sin allow":   append([]string{"server", "--listen", "127.0.0.1:0", "--target", "x:1"}, base...),
		"client sin nombre":  append([]string{"client", "--listen", "127.0.0.1:0", "--target", "x:1"}, base...),
		"files dir inválido": append([]string{"files", "--listen", "127.0.0.1:0", "--dir", "/no/existe", "--allow-cn", "a"}, base...),
		"bandera inválida":   {"server", "--nope"},
	}
	for name, args := range cases {
		if err := run(ctx, args, io.Discard, quietLog()); err == nil {
			t.Errorf("%s: se esperaba error", name)
		}
	}
	if _, err := loadPool(filepath.Join(p.dir, "no.pem")); err == nil {
		t.Error("CA inexistente")
	}
	_ = os.WriteFile(filepath.Join(p.dir, "vacia.pem"), []byte("nada"), 0o600)
	if _, err := loadPool(filepath.Join(p.dir, "vacia.pem")); err == nil {
		t.Error("CA sin PEM")
	}
}
