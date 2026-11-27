package domain

import "time"

// JobKind es el tipo de paso posterior al commit de la baja (spec 021, plan §4).
type JobKind string

// Tipos de job de la bandeja de salida (outbox) de la baja.
const (
	// JobImage borra una imagen en el proveedor de imágenes (Cloudinary).
	JobImage JobKind = "image"
	// JobNotice avisa a la contraparte de una reserva cerrada por la baja.
	JobNotice JobKind = "notice"
)

// Valid indica si k es un tipo conocido.
func (k JobKind) Valid() bool { return k == JobImage || k == JobNotice }

// Reintentos de un job de la baja (plan §4): espera exponencial desde un minuto
// con tope de seis horas; tras MaxJobAttempts intentos fallidos el job queda
// marcado como fallido y se avisa a operación.
const (
	MaxJobAttempts = 20
	jobBackoffBase = time.Minute
	jobBackoffCap  = 6 * time.Hour
)

// JobBackoff devuelve cuánto esperar antes del siguiente intento, dado el
// número de intentos ya hechos (attempts ≥ 1): 1 min, 2 min, 4 min… con tope de
// 6 h. Es una función pura, sin reloj.
func JobBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	d := jobBackoffBase
	for i := 1; i < attempts; i++ {
		d *= 2
		if d >= jobBackoffCap {
			return jobBackoffCap
		}
	}
	return d
}

// JobExhausted indica si con attempts intentos ya hechos el job se rinde.
func JobExhausted(attempts int) bool { return attempts >= MaxJobAttempts }
