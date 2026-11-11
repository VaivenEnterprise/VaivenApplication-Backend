package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	imagescloudinary "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/adapter/cloudinary"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	imgport "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/port"
)

// seedHTTPTimeout limita cada llamada a Cloudinary.
const seedHTTPTimeout = 30 * time.Second

// cloudinaryPhotos sube fotos como la app: pide la firma a images.Service y
// envía el archivo desde memoria (sin archivos temporales). Items.Create
// valida después la foto con Resolve. Ningún error incluye la firma, el API
// key ni el secreto.
type cloudinaryPhotos struct {
	images     imgport.Service
	apiKey     string
	secret     string
	destroyURL string // https://api.cloudinary.com/v1_1/<cloud>/image/destroy
	client     *http.Client
	now        func() time.Time
}

func newCloudinaryPhotos(images imgport.Service, cloudName, apiKey, secret string) *cloudinaryPhotos {
	return &cloudinaryPhotos{
		images:     images,
		apiKey:     apiKey,
		secret:     secret,
		destroyURL: "https://api.cloudinary.com/v1_1/" + url.PathEscape(cloudName) + "/image/destroy",
		client:     &http.Client{Timeout: seedHTTPTimeout},
		now:        time.Now,
	}
}

// Upload sube data con una firma de un solo uso (overwrite=false).
func (p *cloudinaryPhotos) Upload(ctx context.Context, ownerID string, data []byte) (imagesdomain.Ref, error) {
	sig, err := p.images.Signature(ctx, ownerID, imagesdomain.PurposeItemPhoto)
	if err != nil {
		return imagesdomain.Ref{}, fmt.Errorf("firma de subida: %w", err)
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fields := [][2]string{
		{"api_key", sig.APIKey},
		{"timestamp", strconv.FormatInt(sig.Timestamp, 10)},
		{"signature", sig.Signature},
		{"public_id", sig.PublicID},
		{"type", sig.Type},
		{"allowed_formats", sig.AllowedFormats},
		{"overwrite", sig.Overwrite},
	}
	for _, f := range fields {
		if err := w.WriteField(f[0], f[1]); err != nil {
			return imagesdomain.Ref{}, err
		}
	}
	fw, err := w.CreateFormFile("file", "foto.jpg")
	if err != nil {
		return imagesdomain.Ref{}, err
	}
	if _, err := fw.Write(data); err != nil {
		return imagesdomain.Ref{}, err
	}
	if err := w.Close(); err != nil {
		return imagesdomain.Ref{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sig.UploadURL, &body)
	if err != nil {
		return imagesdomain.Ref{}, errors.New("cloudinary: no se pudo preparar la subida")
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if err := p.send(req, func(int, []byte) bool { return true }); err != nil {
		return imagesdomain.Ref{}, fmt.Errorf("cloudinary: subida: %w", err)
	}
	return imagesdomain.Ref{PublicID: sig.PublicID}, nil
}

// Destroy borra la foto con la API firmada de Cloudinary. «not found» cuenta
// como borrada.
func (p *cloudinaryPhotos) Destroy(ctx context.Context, ref imagesdomain.Ref) error {
	ts := strconv.FormatInt(p.now().Unix(), 10)
	typ := imagesdomain.PurposeItemPhoto.Delivery().CloudinaryType()
	form := url.Values{
		"public_id": {ref.PublicID},
		"timestamp": {ts},
		"type":      {typ},
		"api_key":   {p.apiKey},
		"signature": {imagescloudinary.Sign(map[string]string{"public_id": ref.PublicID, "timestamp": ts, "type": typ}, p.secret)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.destroyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("cloudinary: no se pudo preparar el borrado")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return p.send(req, func(_ int, body []byte) bool {
		var r struct {
			Result string `json:"result"`
		}
		return json.Unmarshal(body, &r) == nil && (r.Result == "ok" || r.Result == "not found")
	})
}

// send hace la petición y solo informa el estado HTTP: el cuerpo de error de
// Cloudinary puede repetir la cadena firmada.
func (p *cloudinaryPhotos) send(req *http.Request, ok func(status int, body []byte) bool) error {
	res, err := p.client.Do(req)
	if err != nil {
		return errors.New("no se pudo contactar a Cloudinary")
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode != http.StatusOK || !ok(res.StatusCode, body) {
		return fmt.Errorf("respuesta inesperada (HTTP %d)", res.StatusCode)
	}
	return nil
}

// localPhotos y localImages se usan solo con --scale en development y test:
// generan referencias en la carpeta de cada persona sin tocar Cloudinary.
type localPhotos struct{ baseFolder string }

func (p localPhotos) Upload(_ context.Context, ownerID string, _ []byte) (imagesdomain.Ref, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return imagesdomain.Ref{}, err
	}
	return imagesdomain.Ref{PublicID: imagesdomain.NewPublicID(p.baseFolder, imagesdomain.PurposeItemPhoto, ownerID, hex.EncodeToString(b))}, nil
}

func (localPhotos) Destroy(context.Context, imagesdomain.Ref) error { return nil }

// localImages cumple images.Service sin red: Resolve acepta las referencias
// de la carpeta de la persona con metadatos fijos.
type localImages struct{ baseFolder string }

func (localImages) Signature(context.Context, string, imagesdomain.Purpose) (imagesdomain.UploadSignature, error) {
	return imagesdomain.UploadSignature{}, errors.New("seed local: no firma subidas")
}

func (l localImages) Resolve(_ context.Context, userID string, purpose imagesdomain.Purpose, _ string, refs []imagesdomain.Ref) ([]imagesdomain.Image, error) {
	out := make([]imagesdomain.Image, 0, len(refs))
	for _, r := range refs {
		if !imagesdomain.OwnedBy(r.PublicID, l.baseFolder, purpose, userID) {
			return nil, fmt.Errorf("seed local: %s no es de la persona", r.PublicID)
		}
		out = append(out, imagesdomain.Image{PublicID: r.PublicID, Width: 640, Height: 480, Format: "jpg", Bytes: 9000, Delivery: purpose.Delivery()})
	}
	return out, nil
}

func (localImages) URLs(img imagesdomain.Image) (imagesdomain.URLs, error) {
	u := "https://example.invalid/" + img.PublicID + ".jpg"
	return imagesdomain.URLs{URL: u, ThumbURL: u}, nil
}
