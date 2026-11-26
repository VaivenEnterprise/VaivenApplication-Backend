// Command tlstunnel es un túnel TCP con TLS 1.3 mutuo para el piloto de dos
// VMs de la spec 018 (docs/seguridad/piloto-red-vm2.md). Solo usa la
// biblioteca estándar y va dentro de la imagen firmada de la API, así que
// hereda Trivy, el SBOM, la firma y el despliegue por digest.
//
//	tlstunnel server --listen 10.0.0.4:27018 --target mongo:27017 --cert s.pem --key s.key --ca ca.pem --allow-cn vaiven-pilot-vm2
//	tlstunnel client --listen 127.0.0.1:27017 --target 10.0.0.4:27018 --cert c.pem --key c.key --ca ca.pem --server-name vaiven-pilot-gw
//	tlstunnel files  --listen 10.0.0.4:8444 --dir /srv/relay --cert s.pem --key s.key --ca ca.pem --allow-cn vaiven-pilot-vm2
//
// MongoDB y Redis no publican puertos: solo el túnel escucha en la IP privada.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(ctx, os.Args[1:], os.Stderr, log); err != nil {
		log.Error("tlstunnel", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stderr io.Writer, log *slog.Logger) error {
	if len(args) == 0 {
		return errors.New("uso: tlstunnel server|client|files [opciones]")
	}
	mode := args[0]
	fs := flag.NewFlagSet(mode, flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "", "dirección de escucha")
	target := fs.String("target", "", "destino (server y client)")
	dir := fs.String("dir", "", "directorio a servir (files)")
	cert := fs.String("cert", "", "certificado PEM")
	key := fs.String("key", "", "llave privada PEM")
	ca := fs.String("ca", "", "CA del piloto PEM")
	allow := fs.String("allow-cn", "", "CN de clientes permitidos, separados por coma (server y files)")
	serverName := fs.String("server-name", "", "nombre esperado del servidor (client)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *listen == "" || *cert == "" || *key == "" || *ca == "" {
		return errors.New("faltan --listen, --cert, --key o --ca")
	}
	f := tlsFiles{Cert: *cert, Key: *key, CA: *ca}
	var allowCN []string
	for _, cn := range strings.Split(*allow, ",") {
		if cn = strings.TrimSpace(cn); cn != "" {
			allowCN = append(allowCN, cn)
		}
	}
	switch mode {
	case "server":
		if *target == "" {
			return errors.New("falta --target")
		}
		return runServer(ctx, *listen, *target, f, allowCN, log)
	case "client":
		if *target == "" {
			return errors.New("falta --target")
		}
		if !strings.HasPrefix(*listen, "127.0.0.1:") {
			return errors.New("el cliente solo escucha en 127.0.0.1")
		}
		return runClient(ctx, *listen, *target, f, *serverName, log)
	case "files":
		if *dir == "" {
			return errors.New("falta --dir")
		}
		return runFiles(ctx, *listen, *dir, f, allowCN, log)
	default:
		return fmt.Errorf("modo desconocido: %s", mode)
	}
}
