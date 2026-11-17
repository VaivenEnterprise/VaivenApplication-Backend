// Package expo es el cliente mínimo de Expo Push (spec 011, T007; plan P-02):
// un POST JSON a la API de envío, sin SDK. Cumple worker.Sender.
package expo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MaxBatch es el máximo de mensajes por petición que admite Expo (RF-05).
const MaxBatch = 100

// DeviceNotRegistered es el error de ticket de un token que ya no sirve.
const DeviceNotRegistered = "DeviceNotRegistered"

// maxResponseBytes acota la respuesta que se lee de Expo.
const maxResponseBytes = 1 << 20

// Message es un mensaje push a un token.
type Message struct {
	To    string            `json:"to"`
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
	Sound string            `json:"sound,omitempty"`
}

// Ticket es el resultado de Expo para un mensaje, en el mismo orden en que se
// envió. Error vacío = aceptado.
type Ticket struct {
	Status string
	// Error es details.error de Expo (p. ej. DeviceNotRegistered); vacío si no
	// hay.
	Error string
}

// Client envía lotes de mensajes a Expo.
type Client struct {
	url   string
	token string
	http  *http.Client
}

// NewClient crea el cliente. token (opcional) se manda como Bearer; timeout 0
// usa 10 s (plan P-02).
func NewClient(url, accessToken string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{url: url, token: accessToken, http: &http.Client{Timeout: timeout}}
}

// Send envía hasta MaxBatch mensajes y devuelve un Ticket por mensaje. Un
// error de red o un estado distinto de 200 devuelve error (sin el cuerpo, que
// podría llevar datos): el lote completo se da por fallido.
func (c *Client) Send(ctx context.Context, msgs []Message) ([]Ticket, error) {
	if len(msgs) == 0 {
		return nil, nil
	}
	if len(msgs) > MaxBatch {
		return nil, fmt.Errorf("expo: el lote tiene %d mensajes, el máximo es %d", len(msgs), MaxBatch)
	}
	payload, err := json.Marshal(msgs)
	if err != nil {
		return nil, fmt.Errorf("expo: codificar lote: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("expo: crear petición: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// El error del cliente HTTP puede incluir la URL; no lleva datos de personas.
		return nil, fmt.Errorf("expo: enviar lote: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("expo: leer respuesta: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("expo: respuesta %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			Status  string `json:"status"`
			Details struct {
				Error string `json:"error"`
			} `json:"details"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("expo: respuesta no válida: %w", err)
	}
	if len(out.Data) != len(msgs) {
		return nil, fmt.Errorf("expo: %d tickets para %d mensajes", len(out.Data), len(msgs))
	}
	tickets := make([]Ticket, len(out.Data))
	for i, d := range out.Data {
		tickets[i] = Ticket{Status: d.Status, Error: d.Details.Error}
		if d.Status != "ok" && tickets[i].Error == "" {
			tickets[i].Error = "unknown"
		}
	}
	return tickets, nil
}
