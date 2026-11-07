// Package handler contiene los handlers HTTP de la API. Esta tarea aporta los de
// infraestructura: /healthz, /readyz y /api/v1/meta (spec 002, plan §4.4).
package handler

import (
	"context"
	"net/http"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
)

const messageUnavailable = "El servicio no está disponible por ahora. Intenta de nuevo en unos minutos."

// Pinger comprueba que una dependencia responde. *mongodb.Client lo cumple.
type Pinger interface {
	Ping(ctx context.Context) error
}

type statusResponse struct {
	Status string `json:"status"`
}

// Estados de Redis en /readyz (RF-05, CA-S5).
const (
	redisOK        = "ok"
	redisDegradado = "degradado"
)

type readyResponse struct {
	Status string `json:"status"`
	Redis  string `json:"redis"`
}

// Healthz responde 200 mientras el proceso vive, sin consultar ninguna
// dependencia (RF-04).
func Healthz(w http.ResponseWriter, _ *http.Request) {
	respond.JSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

// Readyz responde 503 service_unavailable solo si MongoDB no responde (RF-05).
// Redis nunca es fuente de verdad: si falla o no está configurado (nil), la API
// sigue lista y lo informa como "degradado" (Art. XIV.2, CA-S5). La causa del
// fallo no se expone en la respuesta.
func Readyz(mongo, redis Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := mongo.Ping(r.Context()); err != nil {
			respond.Failure(w, r, http.StatusServiceUnavailable, respond.CodeServiceUnavailable, messageUnavailable)
			return
		}
		redisState := redisOK
		if redis == nil || redis.Ping(r.Context()) != nil {
			redisState = redisDegradado
		}
		respond.JSON(w, http.StatusOK, readyResponse{Status: "ready", Redis: redisState})
	}
}
