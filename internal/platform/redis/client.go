// Package redis implementa los puertos ratelimit.Limiter y cache.Cache con
// Redis y en memoria (RF-S5, plan P-S5). Es el único paquete que importa
// go-redis (Art. XIV.3). Redis nunca es fuente de verdad.
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// PingTimeout es el tiempo máximo de Ping, pensado para /readyz.
const PingTimeout = 2 * time.Second

// ErrInvalidURL indica una REDIS_URL que no se puede interpretar.
var ErrInvalidURL = errors.New("REDIS_URL inválida")

// Client envuelve la conexión y antepone el prefijo vaiven:<APP_ENV>: a las claves.
type Client struct {
	rdb    *goredis.Client
	prefix string
}

// NewClient crea el cliente sin conectarse todavía (go-redis conecta a demanda).
// El error nunca incluye la URL porque lleva la contraseña.
func NewClient(url, appEnv string) (*Client, error) {
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: %w", ErrInvalidURL)
	}
	return &Client{rdb: goredis.NewClient(opts), prefix: Prefix(appEnv)}, nil
}

// Prefix devuelve el prefijo de claves de un entorno: vaiven:<APP_ENV>:.
func Prefix(appEnv string) string {
	return "vaiven:" + appEnv + ":"
}

// Ping comprueba que Redis responde, con un máximo de PingTimeout.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, PingTimeout)
	defer cancel()
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: ping: %w", err)
	}
	return nil
}

// Close libera las conexiones.
func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) key(k string) string {
	return c.prefix + k
}
