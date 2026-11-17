// Package domain contiene las reglas de las recomendaciones con ML (spec 015):
// parámetros del modelo, eventos, motivos y validación. Solo importa la
// biblioteca estándar y otros paquetes de dominio (Art. III).
package domain

import "time"

// Parámetros del modelo (spec 015, RF-01 a RF-06; plan P-01). Los pesos y Alpha
// son provisionales: se ajustan una vez con la evaluación (RF-07) y el valor
// final se justifica en docs/ml/evaluacion.md.
const (
	// WeightRequest es el peso de una solicitud de reserva.
	WeightRequest = 3.0
	// WeightView es el peso de una vista del detalle de un objeto.
	WeightView = 1.0
	// Alpha mezcla el modelo colaborativo (Alpha) con el de contenido (1 − Alpha).
	Alpha = 0.7

	// Neighbors son los vecinos más parecidos que se guardan por objeto.
	Neighbors = 20
	// PopularSize es el tamaño de cada lista de populares.
	PopularSize = 50
	// HistorySize son las interacciones más recientes que pesan al recomendar.
	HistorySize = 20
	// MinInteractions es el mínimo para dejar el arranque en frío.
	MinInteractions = 2
	// MaxPerCategory limita los objetos de una misma categoría (diversidad).
	MaxPerCategory = 3

	// DefaultLimit y MaxLimit acotan GET /recommendations.
	DefaultLimit = 10
	MaxLimit     = 20

	// EventsPerMinute es el límite de tasa de POST /recommendations/events.
	EventsPerMinute = 60
	// MaxInteractionsRead acota la memoria del recálculo (plan P-11).
	MaxInteractionsRead = 200_000
)

// Tiempos del modelo.
const (
	// Window son los días de interacciones que entran al modelo y la vida de los
	// eventos (índice TTL, RF-09).
	Window = 90 * 24 * time.Hour
	// ViewBucket es el bloque en el que solo cuenta una vista por objeto.
	ViewBucket = 10 * time.Minute
	// ModelMaxAge es la edad a partir de la cual se recalcula el modelo.
	ModelMaxAge = 6 * time.Hour
	// LockTTL es la vida del candado del recálculo si el proceso muere.
	LockTTL = 2 * time.Minute
	// RecomputeTimeout es el tiempo máximo de un recálculo.
	RecomputeTimeout = 60 * time.Second
)

// Bandas de tarifa diaria del modelo de contenido (RF-03), en centavos.
const (
	BandLoan  = "loan"  // préstamo (sin tarifa)
	BandLe50  = "le50"  // hasta $50
	BandLe200 = "le200" // $50.01 a $200
	BandLe500 = "le500" // $200.01 a $500
	BandGt500 = "gt500" // más de $500
)

// PriceBand devuelve la banda de tarifa de un objeto. Un préstamo, o una
// tarifa de 0, es BandLoan.
func PriceBand(modality string, pricePerDayCents int64) string {
	switch {
	case modality == "loan" || pricePerDayCents <= 0:
		return BandLoan
	case pricePerDayCents <= 5_000:
		return BandLe50
	case pricePerDayCents <= 20_000:
		return BandLe200
	case pricePerDayCents <= 50_000:
		return BandLe500
	default:
		return BandGt500
	}
}
