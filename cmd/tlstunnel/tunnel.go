package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"
)

// tlsFiles son las rutas del material TLS (spec 018, piloto de dos VMs). Las
// llaves privadas se generan dentro de cada VM y nunca se registran.
type tlsFiles struct {
	Cert, Key, CA string
}

func loadPool(caFile string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(caFile) // #nosec G304 -- ruta de configuración del operador
	if err != nil {
		return nil, fmt.Errorf("leer CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("la CA no tiene certificados PEM")
	}
	return pool, nil
}

// serverConfig exige TLS 1.3 y un certificado de cliente firmado por la CA del
// piloto cuyo CN esté en allowCN.
func serverConfig(f tlsFiles, allowCN []string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(f.Cert, f.Key)
	if err != nil {
		return nil, fmt.Errorf("cargar certificado: %w", err)
	}
	pool, err := loadPool(f.CA)
	if err != nil {
		return nil, err
	}
	if len(allowCN) == 0 {
		return nil, errors.New("falta --allow-cn: ningún cliente sería aceptado")
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		// VerifyConnection corre en cada conexión, también en las reanudadas
		// (VerifyPeerCertificate no lo haría y un cliente podría saltarse el CN).
		VerifyConnection: func(cs tls.ConnectionState) error {
			chains := cs.VerifiedChains
			if len(chains) == 0 || len(chains[0]) == 0 || !slices.Contains(allowCN, chains[0][0].Subject.CommonName) {
				return errors.New("cliente no autorizado")
			}
			return nil
		},
	}, nil
}

// clientConfig presenta el certificado de cliente y verifica que el servidor
// tenga el nombre esperado y esté firmado por la CA del piloto.
func clientConfig(f tlsFiles, serverName string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(f.Cert, f.Key)
	if err != nil {
		return nil, fmt.Errorf("cargar certificado: %w", err)
	}
	pool, err := loadPool(f.CA)
	if err != nil {
		return nil, err
	}
	if serverName == "" {
		return nil, errors.New("falta --server-name")
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   serverName,
	}, nil
}

// proxy acepta conexiones en ln y las une con dial(). Termina al cerrar ln.
func proxy(ctx context.Context, ln net.Listener, dial func(context.Context) (net.Conn, error), log *slog.Logger) error {
	var wg sync.WaitGroup
	defer wg.Wait()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		in, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer in.Close()
			if tc, ok := in.(*tls.Conn); ok {
				hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := tc.HandshakeContext(hctx)
				cancel()
				if err != nil {
					// Sin detalles del par: solo el hecho (no se registran certificados).
					log.Warn("conexión rechazada en el saludo TLS")
					return
				}
			}
			dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			out, err := dial(dctx)
			cancel()
			if err != nil {
				log.Warn("no se pudo conectar con el destino")
				return
			}
			defer out.Close()
			pipe(in, out)
		}()
	}
}

// pipe copia en ambos sentidos hasta que uno cierre.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	<-done
}

// runServer: TLS mutuo en listen → TCP en claro a target (dentro de la red
// Docker de staging).
func runServer(ctx context.Context, listen, target string, f tlsFiles, allowCN []string, log *slog.Logger) error {
	cfg, err := serverConfig(f, allowCN)
	if err != nil {
		return err
	}
	ln, err := tls.Listen("tcp", listen, cfg)
	if err != nil {
		return err
	}
	log.Info("túnel servidor escuchando", "listen", listen)
	var d net.Dialer
	return proxy(ctx, ln, func(c context.Context) (net.Conn, error) { return d.DialContext(c, "tcp", target) }, log)
}

// runClient: TCP en claro en listen (solo 127.0.0.1) → TLS mutuo a target.
func runClient(ctx context.Context, listen, target string, f tlsFiles, serverName string, log *slog.Logger) error {
	cfg, err := clientConfig(f, serverName)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	log.Info("túnel cliente escuchando", "listen", listen)
	d := tls.Dialer{Config: cfg, NetDialer: &net.Dialer{}}
	return proxy(ctx, ln, func(c context.Context) (net.Conn, error) { return d.DialContext(c, "tcp", target) }, log)
}

// runFiles sirve dir en solo lectura por TLS mutuo (relevo de la imagen
// firmada y paquetes hacia VM2, sin salida a internet).
func runFiles(ctx context.Context, listen, dir string, f tlsFiles, allowCN []string, log *slog.Logger) error {
	cfg, err := serverConfig(f, allowCN)
	if err != nil {
		return err
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() { // #nosec G703 -- directorio que fija el operador en la VM
		return fmt.Errorf("directorio del relevo inválido: %s", dir)
	}
	srv := &http.Server{
		Addr:              listen,
		Handler:           filesOnly(http.FileServer(http.Dir(dir))),
		TLSConfig:         cfg,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Info("relevo de archivos escuchando", "listen", listen)
	if err := srv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// filesOnly solo admite GET y HEAD.
func filesOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
			return
		}
		h.ServeHTTP(w, r)
	})
}
