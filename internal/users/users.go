// Package users declara los puertos compartidos sobre las cuentas (spec 003,
// T008; ROADMAP §5): Directory para leer perfiles públicos por lote (specs 005,
// 006, 007, 008 y 010) y Admin para suspender y calificar (specs 010 y 011).
// La implementación vive en el módulo auth (MongoDB); quien consume solo
// depende de estas interfaces (Art. III.3 y XII.4).
package users

import (
	"context"
	"time"
)

// PublicUser es el perfil público de una persona. Nunca lleva correo ni
// teléfono (Art. VI.4). DisplayName es «Cuenta eliminada» tras la baja.
type PublicUser struct {
	ID            string
	DisplayName   string
	RatingAverage *float64 // nil sin calificaciones
	RatingCount   int64
	MemberSince   time.Time
}

// Directory lee perfiles públicos.
type Directory interface {
	// PublicUsers devuelve los perfiles de ids indexados por id. Los ids que
	// no existen (o mal formados) no aparecen en el mapa; no es un error.
	PublicUsers(ctx context.Context, ids []string) (map[string]PublicUser, error)
}

// Admin cambia datos de la cuenta que deciden otros módulos.
type Admin interface {
	// Suspend suspende la cuenta hasta until (RF-13); una fecha pasada la
	// reactiva.
	Suspend(ctx context.Context, userID string, until time.Time) error
	// AddRating suma una calificación de 1 a 5 (ratingSum y ratingCount).
	AddRating(ctx context.Context, userID string, score int) error
}
