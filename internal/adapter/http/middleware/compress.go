package middleware

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// CompressConfig configura la compresión gzip de respuestas (spec 018, C-08;
// propuesta, no aprobada: se activa solo con HTTP_COMPRESSION_ENABLED).
type CompressConfig struct {
	// MinBytes es el tamaño mínimo para comprimir; por debajo, gzip no ahorra.
	MinBytes int
	// Level es el nivel de gzip (1–9); 0 usa DefaultLevel.
	Level int
	// ExcludePrefixes son rutas que nunca se comprimen.
	ExcludePrefixes []string
	// ExcludeExact son rutas exactas que nunca se comprimen.
	ExcludeExact []string
	// ExcludeSuffixes son terminaciones de ruta que nunca se comprimen
	// (rutas con parámetros, como /bookings/{id}/delivery/qr).
	ExcludeSuffixes []string
}

// DefaultCompressConfig son los valores recomendados. Exclusiones (riesgo de
// BREACH: un secreto y texto que controla un tercero en la misma respuesta
// comprimida por TLS):
//   - /api/v1/auth/*: tokens de acceso y de renovación;
//   - /api/v1/me: datos personales de la cuenta;
//   - /api/v1/uploads/signature: firma de subida a Cloudinary;
//   - /delivery/qr y /return/qr: token de un solo uso del QR.
//   - /docs y /docs/*: documentación y recursos asociados.
//
// Además, nunca se comprime una respuesta con Cache-Control: no-store, que es
// como la API marca las respuestas sensibles.
func DefaultCompressConfig() CompressConfig {
	return CompressConfig{
		MinBytes:        1024,
		Level:           DefaultLevel,
		ExcludePrefixes: []string{"/api/v1/auth/", "/docs/"},
		ExcludeExact:    []string{"/api/v1/me", "/api/v1/uploads/signature", "/docs"},
		ExcludeSuffixes: []string{"/delivery/qr", "/return/qr"},
	}
}

// DefaultLevel equilibra CPU y tamaño para JSON.
const DefaultLevel = 5

// Compress comprime con gzip las respuestas elegibles. No cambia el cuerpo
// (descomprimido es idéntico), ni el código, ni las demás cabeceras; quita
// Content-Length porque deja de ser válido y agrega Vary: Accept-Encoding.
func Compress(cfg CompressConfig) func(http.Handler) http.Handler {
	if cfg.MinBytes <= 0 {
		cfg.MinBytes = 1024
	}
	if cfg.Level < gzip.BestSpeed || cfg.Level > gzip.BestCompression {
		cfg.Level = DefaultLevel
	}
	pool := &sync.Pool{New: func() any {
		zw, _ := gzip.NewWriterLevel(nil, cfg.Level) // el nivel ya se validó
		return zw
	}}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if excluded(cfg, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Add("Vary", "Accept-Encoding")
			if r.Method == http.MethodHead || !AcceptsGzip(r.Header.Get("Accept-Encoding")) {
				next.ServeHTTP(w, r)
				return
			}
			cw := &compressWriter{ResponseWriter: w, cfg: cfg, pool: pool, status: http.StatusOK}
			defer cw.close()
			next.ServeHTTP(cw, r)
		})
	}
}

func excluded(cfg CompressConfig, path string) bool {
	for _, p := range cfg.ExcludePrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	for _, p := range cfg.ExcludeExact {
		if path == p {
			return true
		}
	}
	for _, s := range cfg.ExcludeSuffixes {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}

// AcceptsGzip dice si Accept-Encoding acepta gzip (o *) con q > 0.
func AcceptsGzip(header string) bool {
	wildcard := false
	for _, part := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "gzip" && name != "*" {
			continue
		}
		q := 1.0
		for _, p := range strings.Split(params, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
			if ok && strings.EqualFold(strings.TrimSpace(k), "q") {
				f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
				if err != nil || f < 0 || f > 1 {
					f = 0
				}
				q = f
			}
		}
		if name == "gzip" {
			return q > 0
		}
		wildcard = q > 0
	}
	return wildcard
}

// compressWriter retiene hasta MinBytes para decidir; después escribe en claro
// o en gzip, sin volver a decidir.
type compressWriter struct {
	http.ResponseWriter
	cfg     CompressConfig
	pool    *sync.Pool
	status  int
	header  bool // WriteHeader del handler ya llegó
	decided bool
	zw      *gzip.Writer
	buf     bytes.Buffer
}

func (c *compressWriter) WriteHeader(status int) {
	if c.header {
		return
	}
	if status < 200 && status != http.StatusSwitchingProtocols {
		c.ResponseWriter.WriteHeader(status)
		return
	}
	c.header = true
	c.status = status
	// Estos estados no llevan cuerpo; todavía no hay bytes retenidos.
	if status == http.StatusSwitchingProtocols || status == http.StatusNoContent || status == http.StatusNotModified {
		c.decided = true
		c.ResponseWriter.WriteHeader(status)
	}
}

func (c *compressWriter) Write(p []byte) (int, error) {
	if !c.header {
		c.WriteHeader(http.StatusOK)
	}
	if c.decided {
		if c.zw != nil {
			return c.zw.Write(p)
		}
		return c.ResponseWriter.Write(p)
	}
	c.buf.Write(p)
	if c.buf.Len() >= c.cfg.MinBytes {
		if err := c.decide(c.eligible()); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// eligible decide por cabeceras: tipo comprimible, sin codificación previa y
// sin no-store.
func (c *compressWriter) eligible() bool {
	h := c.Header()
	if h.Get("Content-Encoding") != "" {
		return false
	}
	if strings.Contains(strings.ToLower(h.Get("Cache-Control")), "no-store") {
		return false
	}
	ct, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	return err == nil && (ct == "application/json" || ct == "application/problem+json")
}

// decide envía las cabeceras y lo retenido, comprimido o no.
func (c *compressWriter) decide(gz bool) error {
	if c.decided {
		return nil
	}
	c.decided = true
	if gz {
		h := c.Header()
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		c.zw = c.pool.Get().(*gzip.Writer)
		c.zw.Reset(c.ResponseWriter)
	}
	c.ResponseWriter.WriteHeader(c.status)
	if c.buf.Len() == 0 {
		return nil
	}
	var err error
	if c.zw != nil {
		_, err = c.zw.Write(c.buf.Bytes())
	} else {
		_, err = c.ResponseWriter.Write(c.buf.Bytes())
	}
	c.buf.Reset()
	return err
}

// close termina la respuesta: lo pequeño sale en claro y gzip se cierra.
func (c *compressWriter) close() {
	if !c.decided {
		if !c.header && c.buf.Len() == 0 {
			// El handler no escribió nada: net/http responderá 200 vacío.
			return
		}
		_ = c.decide(false)
	}
	if c.zw != nil {
		_ = c.zw.Close()
		c.zw.Reset(nil)
		c.pool.Put(c.zw)
		c.zw = nil
	}
}

// Flush entrega lo retenido (sin comprimir si no llegó al umbral).
func (c *compressWriter) Flush() {
	if !c.decided {
		_ = c.decide(c.buf.Len() >= c.cfg.MinBytes && c.eligible())
	}
	if c.zw != nil {
		_ = c.zw.Flush()
	}
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack no se permite a mitad de una respuesta comprimida.
func (c *compressWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := c.ResponseWriter.(http.Hijacker); ok && !c.decided {
		return h.Hijack()
	}
	return nil, nil, errors.New("compress: hijack no disponible")
}

// Unwrap permite a http.ResponseController llegar al writer original.
func (c *compressWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }
