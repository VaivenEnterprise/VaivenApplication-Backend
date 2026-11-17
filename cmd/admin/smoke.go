package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
)

// smoke es la prueba de humo que ESCRIBE datos (spec 013, T007, CA-03): crea
// una cuenta desechable, sube una foto con una firma real, publica un objeto,
// lo lee, lo borra y da de baja la cuenta. Siempre limpia lo que creó, aunque
// un paso falle. Se niega a correr contra producción: allí se usa
// scripts/smoke.sh, que solo lee.
const smokeTimeout = 2 * time.Minute

// smokeEmailDomain es un dominio reservado (RFC 2606): nunca recibe correo.
const smokeEmailDomain = "vaiven.invalid"

// smokePhotoDestroyer borra la foto subida (el borrado de un objeto es lógico
// y no toca Cloudinary).
type smokePhotoDestroyer interface {
	Destroy(ctx context.Context, ref imagesdomain.Ref) error
}

type smokeDeps struct {
	client *http.Client // para la API y para la subida a Cloudinary
	photos smokePhotoDestroyer
	random io.Reader
}

type smokeFactory func(cfg config.Config, connectTo, apiHost string) (smokeDeps, error)

// newSmokeDeps arma el cliente HTTP real. Con --connect-to, las conexiones al
// host de la API van a esa dirección, pero el TLS sigue validando el nombre
// real (como curl --resolve): sirve para correrlo dentro de la red de Docker.
func newSmokeDeps(cfg config.Config, connectTo, apiHost string) (smokeDeps, error) {
	if cfg.Cloudinary.CloudName == "" {
		return smokeDeps{}, errors.New("faltan las credenciales de Cloudinary para borrar la foto de prueba")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if connectTo != "" && addr == apiHost {
			addr = connectTo
		}
		return dialer.DialContext(ctx, network, addr)
	}
	photos := newCloudinaryPhotos(nil, cfg.Cloudinary.CloudName, cfg.Cloudinary.APIKey, cfg.Cloudinary.APISecret.Reveal())
	return smokeDeps{client: &http.Client{Timeout: 30 * time.Second, Transport: tr}, photos: photos, random: rand.Reader}, nil
}

// runSmoke es el subcomando completo. Devuelve 0 si todo pasó y se limpió, 1 si
// algo falló y 2 ante un uso inválido o un entorno prohibido.
func runSmoke(args []string, stdout, stderr io.Writer, cfg config.Config, open smokeFactory) int {
	fs := flag.NewFlagSet("smoke", flag.ContinueOnError)
	fs.SetOutput(stderr)
	baseURL := fs.String("base-url", "", "URL base de la API (https://host:puerto)")
	connectTo := fs.String("connect-to", "", "opcional: dirección host:puerto a la que conectar en lugar del host de --base-url")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintln(stderr, "smoke: argumentos de más")
		return 2
	}
	if cfg.AppEnv == config.EnvProduction {
		_, _ = fmt.Fprintln(stderr, "smoke: no se ejecuta con APP_ENV=production (usa scripts/smoke.sh)")
		return 2
	}
	u, err := url.Parse(strings.TrimRight(*baseURL, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "https" && (u.Scheme != "http" || cfg.AppEnv != config.EnvDevelopment)) {
		_, _ = fmt.Fprintln(stderr, "smoke: --base-url debe ser https://host[:puerto]")
		return 2
	}
	apiHost := u.Host
	if u.Port() == "" {
		apiHost = net.JoinHostPort(u.Hostname(), map[string]string{"https": "443", "http": "80"}[u.Scheme])
	}
	deps, err := open(cfg, *connectTo, apiHost)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "smoke: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), smokeTimeout)
	defer cancel()
	s := &smokeRun{base: u.String(), deps: deps, out: stdout}
	return s.run(ctx)
}

type smokeRun struct {
	base   string
	deps   smokeDeps
	out    io.Writer
	token  string
	failed bool
}

func (s *smokeRun) step(name string, err error) bool {
	if err != nil {
		s.failed = true
		_, _ = fmt.Fprintf(s.out, "FALLA    %-22s %v\n", name, err)
		return false
	}
	_, _ = fmt.Fprintf(s.out, "OK       %s\n", name)
	return true
}

func (s *smokeRun) run(ctx context.Context) (code int) {
	var meta struct {
		Environment string `json:"environment"`
		Version     string `json:"version"`
	}
	if _, err := s.call(ctx, http.MethodGet, "/api/v1/meta", nil, http.StatusOK, &meta); !s.step("meta", err) {
		return 1
	}
	if meta.Environment == config.EnvProduction || meta.Environment == "" {
		_, _ = fmt.Fprintf(s.out, "FALLA    %-22s la API dice environment=%q: smoke no escribe en producción\n", "entorno", meta.Environment)
		return 2
	}
	_, _ = fmt.Fprintf(s.out, "         entorno %s, versión %s\n", meta.Environment, meta.Version)

	suffix, err := s.randomHex(6)
	if err != nil {
		s.step("preparar cuenta", err)
		return 1
	}
	password, err := s.randomHex(16)
	if err != nil {
		s.step("preparar cuenta", err)
		return 1
	}
	email := "smoke-" + suffix + "@" + smokeEmailDomain

	// Limpieza en orden inverso, pase lo que pase.
	var itemID, publicID string
	registered := false
	defer func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if itemID != "" {
			_, err := s.call(cctx, http.MethodDelete, "/api/v1/items/"+url.PathEscape(itemID), nil, http.StatusNoContent, nil)
			s.step("limpieza: objeto", err)
		}
		if publicID != "" {
			s.step("limpieza: foto", s.deps.photos.Destroy(cctx, imagesdomain.Ref{PublicID: publicID}))
		}
		if registered && s.token != "" {
			_, err := s.call(cctx, http.MethodDelete, "/api/v1/me", map[string]any{"password": password}, http.StatusNoContent, nil)
			s.step("limpieza: baja de la cuenta", err)
		}
		// Una limpieza fallida también hace fallar la prueba.
		if s.failed && code == 0 {
			code = 1
		}
	}()

	var auth struct {
		Tokens struct {
			AccessToken string `json:"accessToken"`
		} `json:"tokens"`
	}
	_, err = s.call(ctx, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": email, "password": password, "displayName": "Prueba de humo",
		"acceptTerms": true, "isAdult": true,
	}, http.StatusCreated, &auth)
	if !s.step("registro", err) {
		return 1
	}
	registered, s.token = true, auth.Tokens.AccessToken

	_, err = s.call(ctx, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": email, "password": password}, http.StatusOK, &auth)
	if !s.step("inicio de sesión", err) {
		return 1
	}
	s.token = auth.Tokens.AccessToken

	var sig struct {
		UploadURL      string `json:"uploadUrl"`
		APIKey         string `json:"apiKey"`
		Timestamp      int64  `json:"timestamp"`
		Signature      string `json:"signature"`
		PublicID       string `json:"publicId"`
		Type           string `json:"type"`
		AllowedFormats string `json:"allowedFormats"`
		Overwrite      string `json:"overwrite"`
	}
	_, err = s.call(ctx, http.MethodPost, "/api/v1/uploads/signature", map[string]any{"purpose": "item_photo"}, http.StatusOK, &sig)
	if !s.step("firma de subida", err) {
		return 1
	}
	photo, err := seedAssets.ReadFile("assets/objeto-01.jpg")
	if err == nil {
		err = s.upload(ctx, sig.UploadURL, [][2]string{
			{"api_key", sig.APIKey}, {"timestamp", strconv.FormatInt(sig.Timestamp, 10)},
			{"signature", sig.Signature}, {"public_id", sig.PublicID}, {"type", sig.Type},
			{"allowed_formats", sig.AllowedFormats}, {"overwrite", sig.Overwrite},
		}, photo)
	}
	if !s.step("subida a Cloudinary", err) {
		// La subida pudo haberse guardado aunque la respuesta fallara.
		publicID = sig.PublicID
		return 1
	}
	publicID = sig.PublicID

	var item struct {
		ID string `json:"id"`
	}
	_, err = s.call(ctx, http.MethodPost, "/api/v1/items", map[string]any{
		"title": "Prueba de humo", "description": "Objeto temporal de la prueba de humo; se borra al terminar.",
		"category": "otros", "condition": "good", "modality": "loan",
		"declaredValueCents": 5000, "pricePerDayCents": 0, "priceSource": "manual",
		"zone":   map[string]any{"municipality": "Monterrey"},
		"photos": []map[string]any{{"publicId": publicID}},
	}, http.StatusCreated, &item)
	if err == nil && item.ID == "" {
		err = errors.New("la respuesta no trae id")
	}
	if !s.step("publicar objeto", err) {
		return 1
	}
	itemID = item.ID

	_, err = s.call(ctx, http.MethodGet, "/api/v1/items/"+url.PathEscape(itemID), nil, http.StatusOK, nil)
	if !s.step("leer objeto", err) {
		return 1
	}
	_, err = s.call(ctx, http.MethodDelete, "/api/v1/items/"+url.PathEscape(itemID), nil, http.StatusNoContent, nil)
	if !s.step("borrar objeto", err) {
		return 1
	}
	itemID = ""
	return 0
}

func (s *smokeRun) randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(s.deps.random, b); err != nil {
		return "", errors.New("no se pudo generar un valor aleatorio")
	}
	return hex.EncodeToString(b), nil
}

// call hace una petición JSON a la API. Los errores solo incluyen el método,
// la ruta y el estado HTTP: nunca el cuerpo (podría traer datos de la cuenta).
func (s *smokeRun) call(ctx context.Context, method, path string, body any, want int, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, rd)
	if err != nil {
		return 0, fmt.Errorf("%s %s: petición inválida", method, path)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	res, err := s.deps.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s %s: sin respuesta", method, path)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != want {
		return res.StatusCode, fmt.Errorf("%s %s: HTTP %d (se esperaba %d)", method, path, res.StatusCode, want)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return res.StatusCode, fmt.Errorf("%s %s: respuesta no es JSON válido", method, path)
		}
	}
	return res.StatusCode, nil
}

// upload sube la foto a Cloudinary con los campos de la firma, como la app.
func (s *smokeRun) upload(ctx context.Context, uploadURL string, fields [][2]string, data []byte) error {
	u, err := url.Parse(uploadURL)
	if err != nil || u.Scheme != "https" {
		return errors.New("uploadUrl inválida")
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, f := range fields {
		if err := w.WriteField(f[0], f[1]); err != nil {
			return err
		}
	}
	fw, err := w.CreateFormFile("file", "smoke.jpg")
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), &body)
	if err != nil {
		return errors.New("no se pudo preparar la subida")
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := s.deps.client.Do(req)
	if err != nil {
		return errors.New("no se pudo contactar a Cloudinary")
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("respuesta de Cloudinary: HTTP %d", res.StatusCode)
	}
	return nil
}
