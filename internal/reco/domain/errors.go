package domain

import shared "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/domain"

// Errores de las recomendaciones; la capa HTTP los traduce con el Art. IV.
var (
	// ErrItemNotFound: el evento apunta a un objeto que no existe.
	ErrItemNotFound = shared.NewNotFound("No encontramos ese objeto.")
	// ErrDisabled: RECO_ENABLED=false (RF-10).
	ErrDisabled = shared.NewUnavailable("Las recomendaciones no están disponibles por ahora.")
)
