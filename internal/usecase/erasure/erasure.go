// Package erasure declara el contrato compartido de la baja de cuenta (spec
// 021): cada módulo implementa Eraser sobre SUS colecciones y devuelve en
// EraseResult lo que debe hacerse después del commit. Es un paquete neutral,
// como cache o revocation: ningún adaptador de módulo importa el usecase de
// otro módulo (Art. III y XII). Solo importa la biblioteca estándar.
package erasure

import (
	"context"
	"time"
)

// ImageRef identifica una imagen que hay que borrar en el proveedor de
// imágenes.
type ImageRef struct {
	PublicID string
	Delivery string
	// NotBefore, si no es cero, es la fecha más temprana en que se borra (p. ej.
	// la evidencia de una incidencia se borra al purgar el documento).
	NotBefore time.Time
}

// Notice es un aviso para una contraparte de la baja. Los textos son genéricos:
// no llevan datos de la cuenta eliminada.
type Notice struct {
	UserID string
	Type   string
	Title  string
	Body   string
	Data   map[string]string
}

// EraseResult es lo que un Eraser deja para DESPUÉS del commit.
type EraseResult struct {
	Images  []ImageRef
	Notices []Notice
	// Counts son documentos tocados por colección, solo para pruebas y
	// métricas; nunca ids ni valores.
	Counts map[string]int64
}

// Eraser lo implementa cada módulo sobre SUS colecciones (Art. XII). Se llama
// dentro de la transacción de la baja con su ctx. Debe ser idempotente, sin
// estado local y sin efectos fuera de la base de datos (las fotos y los avisos
// salen en EraseResult).
type Eraser interface {
	Name() string
	Erase(ctx context.Context, userID string, at time.Time) (EraseResult, error)
}

// RetentionMonths son los meses que se conservan incidencias, denuncias y
// sanciones de una cuenta dada de baja (spec 021, D-4).
const RetentionMonths = 12

// PurgeAt es la fecha en que se purgan incidencias, denuncias y sanciones de
// una cuenta dada de baja en at: 12 meses de calendario (no 365 días), en UTC.
// at viene del reloj inyectable, así se prueba sin esperar (RNF-06).
func PurgeAt(at time.Time) time.Time {
	return at.UTC().AddDate(0, RetentionMonths, 0)
}
