// Package imagefetch descarga la miniatura que construyó el servidor para
// enviarla al proveedor de IA (spec 014, RF-02). Lo comparten los adaptadores
// de proveedores para no duplicar código (Art. XII.6).
//
// Nunca recibe una URL de la persona: el caso de uso la arma con
// images.Service. Aun así se acota: solo https (salvo pruebas), como máximo
// 5 MB y solo imágenes.
package imagefetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
)

// MaxBytes es el tamaño máximo de la miniatura (POL-13).
const MaxBytes = 5 << 20

// Image es una imagen descargada.
type Image struct {
	MIMEType string
	Data     []byte
}

// Fetch descarga rawURL. allowHTTP solo debe ser true en pruebas.
func Fetch(ctx context.Context, client *http.Client, rawURL string, allowHTTP bool) (Image, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && (!allowHTTP || u.Scheme != "http")) {
		return Image{}, errors.New("imagefetch: la miniatura debe ser una URL https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Image{}, errors.New("imagefetch: no se pudo preparar la descarga")
	}
	res, err := client.Do(req)
	if err != nil {
		return Image{}, aidomain.ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return Image{}, fmt.Errorf("imagefetch: miniatura: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, MaxBytes+1))
	if err != nil {
		return Image{}, aidomain.ErrUnavailable
	}
	if len(data) > MaxBytes {
		return Image{}, errors.New("imagefetch: la miniatura supera 5 MB")
	}
	mime := strings.TrimSpace(strings.Split(res.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(mime, "image/") {
		mime = http.DetectContentType(data)
		if !strings.HasPrefix(mime, "image/") {
			return Image{}, errors.New("imagefetch: la miniatura no es una imagen")
		}
	}
	return Image{MIMEType: mime, Data: data}, nil
}
