package handler

import (
	"net/http"
	"time"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/adapter/http/respond"
)

// MetaInfo son los datos fijos que expone /api/v1/meta.
type MetaInfo struct {
	Name        string
	Version     string
	Environment string
}

type metaResponse struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
	ServerTime  string `json:"serverTime"`
}

// Meta responde 200 con el nombre, la versión y el entorno del servicio y la
// hora del servidor en UTC (RFC 3339), para que el móvil compruebe conectividad
// (RF-06).
func Meta(info MetaInfo) http.HandlerFunc {
	return meta(info, time.Now)
}

// meta permite inyectar el reloj en las pruebas.
func meta(info MetaInfo, now func() time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		respond.JSON(w, http.StatusOK, metaResponse{
			Name:        info.Name,
			Version:     info.Version,
			Environment: info.Environment,
			ServerTime:  now().UTC().Format(time.RFC3339),
		})
	}
}
