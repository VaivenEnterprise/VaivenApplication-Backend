// Package port declara items.Moderation, la interfaz que usará la moderación
// de la spec 011 para bloquear objetos y revisar su contenido (spec 005, T006;
// Art. XII.4). Solo depende de la biblioteca estándar, así que los casos de uso
// de otros módulos lo pueden importar sin romper la regla de dependencia
// (Art. III). La implementación es items/usecase.Service.
package port

import "context"

// Snapshot es el contenido de un objeto que revisa moderación.
type Snapshot struct {
	ID          string
	OwnerID     string
	Title       string
	Description string
	UsageRules  string
	Status      string
}

// Moderation bloquea objetos y devuelve su contenido.
type Moderation interface {
	// Block pasa el objeto a blocked: deja de verse para otras personas y su
	// dueño ya no lo puede editar, pausar ni reanudar. Bloquear uno ya
	// bloqueado no cambia nada; uno eliminado devuelve not_found.
	Block(ctx context.Context, itemID string) error
	// Snapshot devuelve el contenido actual del objeto (incluso eliminado,
	// para revisar un reporte).
	Snapshot(ctx context.Context, itemID string) (Snapshot, error)
}
