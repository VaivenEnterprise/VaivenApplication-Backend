// Package mongodb crea y verifica la conexión con MongoDB (spec 002, plan §4.5).
// Es infraestructura genérica: no conoce colecciones ni reglas de negocio.
package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// PingTimeout es el tiempo máximo de Ping, pensado para /readyz.
const PingTimeout = 2 * time.Second

// ErrInvalidParams indica que la URI o el timeout no son válidos, antes de
// intentar cualquier conexión.
var ErrInvalidParams = errors.New("parámetros de conexión inválidos")

// Client envuelve el cliente del driver para exponer solo lo que usa la API.
type Client struct {
	mc *mongo.Client
}

// Connect valida los parámetros, crea el cliente y hace ping dentro de timeout.
// Si el ping falla, cierra el cliente antes de devolver el error.
func Connect(ctx context.Context, uri string, timeout time.Duration) (*Client, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil, fmt.Errorf("mongodb: %w: URI vacía", ErrInvalidParams)
	}
	if !strings.HasPrefix(uri, "mongodb://") && !strings.HasPrefix(uri, "mongodb+srv://") {
		return nil, fmt.Errorf("mongodb: %w: la URI debe empezar con mongodb:// o mongodb+srv://", ErrInvalidParams)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("mongodb: %w: el timeout debe ser mayor que cero", ErrInvalidParams)
	}

	opts := options.Client().ApplyURI(uri).SetServerSelectionTimeout(timeout)
	mc, err := mongo.Connect(opts)
	if err != nil {
		// El driver valida la URI aquí; nunca se incluye la URI en el error
		// porque puede llevar la contraseña (Art. VI).
		return nil, fmt.Errorf("mongodb: %w: %w", ErrInvalidParams, err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := mc.Ping(pingCtx, nil); err != nil {
		_ = mc.Disconnect(context.Background())
		return nil, fmt.Errorf("mongodb: ping: %w", err)
	}
	return &Client{mc: mc}, nil
}

// Ping comprueba que MongoDB responde, con un máximo de PingTimeout.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, PingTimeout)
	defer cancel()
	if err := c.mc.Ping(ctx, nil); err != nil {
		return fmt.Errorf("mongodb: ping: %w", err)
	}
	return nil
}

// Database devuelve el manejador de la base indicada.
func (c *Client) Database(name string) *mongo.Database {
	return c.mc.Database(name)
}

// Disconnect cierra las conexiones; se llama en el cierre ordenado (plan §4.6).
func (c *Client) Disconnect(ctx context.Context) error {
	if err := c.mc.Disconnect(ctx); err != nil {
		return fmt.Errorf("mongodb: disconnect: %w", err)
	}
	return nil
}
