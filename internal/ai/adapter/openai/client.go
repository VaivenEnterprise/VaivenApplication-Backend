// Package openai llama a un proveedor con API compatible con OpenAI
// (POST /chat/completions) usando net/http, sin SDK (spec 014, RF-03 y T016;
// Art. VIII.4). Sirve para Groq en staging y para Ollama en local; implementa
// Generate, el método que declaran como puerto los casos de uso.
//
// Seguridad: la clave viaja en Authorization: Bearer (nunca en la URL) y
// ningún error incluye el texto de la conversación, la imagen ni la respuesta
// del modelo (RF-07 y RF-18).
package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/adapter/imagefetch"
	aidomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/ai/domain"
)

const (
	httpTimeout  = 20 * time.Second // tope por intento; el caso de uso pone 10–12 s
	retryDelay   = 500 * time.Millisecond
	maxRespBytes = 1 << 20
	temperature  = 0.4
	maxOutTokens = 4096 // los modelos que razonan gastan tokens antes de responder
)

// Config es lo que el cliente necesita de config.AI.
type Config struct {
	BaseURL string // p. ej. https://api.groq.com/openai/v1 o http://localhost:11434/v1
	Model   string
	APIKey  string // vacía solo con Ollama local
}

// Client es el cliente compatible con OpenAI.
type Client struct {
	cfg             Config
	http            *http.Client
	sleep           func(context.Context, time.Duration) error
	allowHTTPImages bool // solo pruebas
}

// New crea el cliente.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" || cfg.Model == "" {
		return nil, errors.New("openai: faltan AI_BASE_URL o AI_MODEL")
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

type imageURL struct {
	URL string `json:"url"`
}

type part struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string o []part (con imagen)
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []message       `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

func (c *Client) buildBody(ctx context.Context, req aidomain.Request) ([]byte, error) {
	if len(req.Turns) == 0 {
		return nil, errors.New("openai: la petición no tiene mensajes")
	}
	cr := chatRequest{Model: c.cfg.Model, Temperature: temperature, MaxTokens: maxOutTokens}
	if sys := systemText(req); sys != "" {
		cr.Messages = append(cr.Messages, message{Role: "system", Content: sys})
	}
	for i, t := range req.Turns {
		role := "user"
		if t.Role == aidomain.RoleModel {
			role = "assistant"
		}
		if i == len(req.Turns)-1 && req.ImageURL != "" {
			img, err := imagefetch.Fetch(ctx, c.http, req.ImageURL, c.allowHTTPImages)
			if err != nil {
				return nil, err
			}
			dataURL := "data:" + img.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
			cr.Messages = append(cr.Messages, message{Role: role, Content: []part{
				{Type: "text", Text: t.Text},
				{Type: "image_url", ImageURL: &imageURL{URL: dataURL}},
			}})
			continue
		}
		cr.Messages = append(cr.Messages, message{Role: role, Content: t.Text})
	}
	// El modo JSON no siempre se admite junto con imágenes: solo se pide sin
	// imagen. En los dos casos el JSON se extrae del texto (extractJSON).
	if req.ImageURL == "" {
		cr.ResponseFormat = &responseFormat{Type: "json_object"}
	}
	return json.Marshal(cr)
}

// systemText une las instrucciones fijas con el esquema JSON esperado: estos
// proveedores no reciben el esquema como parámetro aparte.
func systemText(req aidomain.Request) string {
	var b strings.Builder
	b.WriteString(req.System)
	if req.Schema != nil {
		schema, err := json.Marshal(req.Schema)
		if err == nil {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString("Responde SOLO con un objeto JSON válido, sin texto antes ni después y sin bloques de código, que cumpla este esquema (tipos en mayúsculas):\n")
			b.Write(schema)
		}
	}
	return b.String()
}

// --- Respuesta ---

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// call hace un intento. retry indica si conviene reintentar.
func (c *Client) call(ctx context.Context, body []byte) (out []byte, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, false, errors.New("openai: no se pudo preparar la petición")
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
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
		// Solo el estado: el cuerpo podría repetir la petición.
		return nil, false, fmt.Errorf("%w (HTTP %d)", aidomain.ErrUnavailable, res.StatusCode)
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil || len(cr.Choices) == 0 {
		return nil, false, aidomain.ErrUnavailable
	}
	ch := cr.Choices[0]
	if ch.FinishReason == "content_filter" {
		return nil, false, aidomain.ErrBlocked
	}
	text := extractJSON(ch.Message.Content)
	if text == "" {
		return nil, false, aidomain.ErrUnavailable
	}
	return []byte(text), false, nil
}

// extractJSON devuelve el objeto JSON de la respuesta, aunque el modelo lo
// envuelva en un bloque ```json o agregue texto alrededor.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
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
