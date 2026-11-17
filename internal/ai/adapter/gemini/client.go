// Package gemini llama a la API de Gemini con net/http, sin SDK (spec 014,
// RF-03; plan P-05 y P-06; Art. VIII.4). Implementa Generate, el método que
// declaran como puerto los casos de uso de sugerencias y del asistente.
//
// Seguridad: la clave viaja en la cabecera x-goog-api-key (nunca en la URL,
// para que no termine en registros) y ningún error incluye el texto de la
// conversación, la imagen ni la respuesta del modelo (RF-07 y RF-18).
package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
)

// Límites del cliente.
const (
	httpTimeout   = 20 * time.Second // tope por intento; el caso de uso pone 10–12 s con su contexto
	retryDelay    = 500 * time.Millisecond
	maxImageBytes = 5 << 20 // POL-13
	maxRespBytes  = 1 << 20
	temperature   = 0.4
	maxOutTokens  = 2048
)

// Config es lo que el cliente necesita de config.AI.
type Config struct {
	BaseURL string // https://generativelanguage.googleapis.com
	Model   string // p. ej. gemini-2.5-flash
	APIKey  string
}

// Client es el cliente de Gemini.
type Client struct {
	cfg             Config
	http            *http.Client
	sleep           func(context.Context, time.Duration) error
	allowHTTPImages bool // solo pruebas: las miniaturas reales son https
}

// New crea el cliente. Falla si falta algún dato de la configuración.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" || cfg.Model == "" || cfg.APIKey == "" {
		return nil, errors.New("gemini: faltan AI_BASE_URL, AI_MODEL o AI_API_KEY")
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: httpTimeout}, sleep: sleepCtx}, nil
}

// Generate envía la petición y devuelve el JSON que produjo el modelo (aún sin
// validar: lo valida internal/ai/domain). Reintenta una vez ante tiempos de
// espera, 429 y 5xx.
func (c *Client) Generate(ctx context.Context, req aidomain.Request) ([]byte, error) {
	body, err := c.buildBody(ctx, req)
	if err != nil {
		return nil, err
	}
	out, retry, err := c.call(ctx, body)
	if err == nil || !retry {
		return out, err
	}
	if c.sleep(ctx, retryDelay) != nil {
		return nil, aidomain.ErrUnavailable
	}
	out, _, err = c.call(ctx, body)
	return out, err
}

// --- Petición ---

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inlineData,omitempty"`
}

type inlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type generationConfig struct {
	ResponseMIMEType string           `json:"responseMimeType"`
	ResponseSchema   *aidomain.Schema `json:"responseSchema,omitempty"`
	Temperature      float64          `json:"temperature"`
	MaxOutputTokens  int              `json:"maxOutputTokens"`
}

type generateRequest struct {
	SystemInstruction *content         `json:"systemInstruction,omitempty"`
	Contents          []content        `json:"contents"`
	GenerationConfig  generationConfig `json:"generationConfig"`
}

func (c *Client) buildBody(ctx context.Context, req aidomain.Request) ([]byte, error) {
	if len(req.Turns) == 0 {
		return nil, errors.New("gemini: la petición no tiene mensajes")
	}
	gr := generateRequest{
		GenerationConfig: generationConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   req.Schema,
			Temperature:      temperature,
			MaxOutputTokens:  maxOutTokens,
		},
	}
	if req.System != "" {
		gr.SystemInstruction = &content{Parts: []part{{Text: req.System}}}
	}
	for i, t := range req.Turns {
		role := aidomain.RoleUser
		if t.Role == aidomain.RoleModel {
			role = aidomain.RoleModel
		}
		ct := content{Role: role, Parts: []part{{Text: t.Text}}}
		// La imagen acompaña al último mensaje de la persona (parte A).
		if i == len(req.Turns)-1 && req.ImageURL != "" {
			img, err := c.fetchImage(ctx, req.ImageURL)
			if err != nil {
				return nil, err
			}
			ct.Parts = append(ct.Parts, part{InlineData: img})
		}
		gr.Contents = append(gr.Contents, ct)
	}
	return json.Marshal(gr)
}

// fetchImage descarga la miniatura que construyó el servidor (nunca una URL de
// la persona; spec 014, RF-02) con tamaño y tipo acotados.
func (c *Client) fetchImage(ctx context.Context, raw string) (*inlineData, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && (!c.allowHTTPImages || u.Scheme != "http")) || u.Host == "" {
		return nil, errors.New("gemini: la miniatura debe ser una URL https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("gemini: no se pudo preparar la descarga de la miniatura")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, aidomain.ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: miniatura: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxImageBytes+1))
	if err != nil {
		return nil, aidomain.ErrUnavailable
	}
	if len(data) > maxImageBytes {
		return nil, errors.New("gemini: la miniatura supera 5 MB")
	}
	mime := strings.TrimSpace(strings.Split(res.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(mime, "image/") {
		mime = http.DetectContentType(data)
		if !strings.HasPrefix(mime, "image/") {
			return nil, errors.New("gemini: la miniatura no es una imagen")
		}
	}
	return &inlineData{MIMEType: mime, Data: base64.StdEncoding.EncodeToString(data)}, nil
}

// --- Respuesta ---

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

// call hace un intento. retry indica si conviene reintentar.
func (c *Client) call(ctx context.Context, body []byte) (out []byte, retry bool, err error) {
	endpoint := c.cfg.BaseURL + "/v1beta/models/" + url.PathEscape(c.cfg.Model) + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, errors.New("gemini: no se pudo preparar la petición")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.cfg.APIKey)
	res, err := c.http.Do(req)
	if err != nil {
		// Tiempo de espera o red: se reintenta salvo que el contexto ya venció.
		return nil, ctx.Err() == nil, aidomain.ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxRespBytes))
	if err != nil {
		return nil, true, aidomain.ErrUnavailable
	}
	switch {
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
		return nil, true, aidomain.ErrUnavailable
	case res.StatusCode != http.StatusOK:
		// 400/401/403/404: configuración o petición inválida; reintentar no sirve.
		// Solo se informa el estado: el cuerpo podría repetir la petición.
		return nil, false, fmt.Errorf("%w (HTTP %d)", aidomain.ErrUnavailable, res.StatusCode)
	}
	var gr generateResponse
	if err := json.Unmarshal(raw, &gr); err != nil {
		return nil, false, aidomain.ErrUnavailable
	}
	if gr.PromptFeedback.BlockReason != "" {
		return nil, false, aidomain.ErrBlocked
	}
	if len(gr.Candidates) == 0 {
		return nil, false, aidomain.ErrUnavailable
	}
	cand := gr.Candidates[0]
	if cand.FinishReason == "SAFETY" || cand.FinishReason == "PROHIBITED_CONTENT" || cand.FinishReason == "BLOCKLIST" {
		return nil, false, aidomain.ErrBlocked
	}
	var sb strings.Builder
	for _, p := range cand.Content.Parts {
		sb.WriteString(p.Text)
	}
	text := strings.TrimSpace(sb.String())
	if text == "" {
		return nil, false, aidomain.ErrUnavailable
	}
	return []byte(text), false, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
