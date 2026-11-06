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

// Healthz responde 200 mientras el proceso vive, sin consultar ninguna
// dependencia (RF-04).
func Healthz(w http.ResponseWriter, _ *http.Request) {
	respond.JSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

// Readyz responde 200 si pinger responde y 503 service_unavailable si no
// (RF-05). La causa del fallo no se expone en la respuesta.
func Readyz(pinger Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := pinger.Ping(r.Context()); err != nil {
			respond.Failure(w, r, http.StatusServiceUnavailable, respond.CodeServiceUnavailable, messageUnavailable)
			return
		}
		respond.JSON(w, http.StatusOK, statusResponse{Status: "ready"})
	}
}
