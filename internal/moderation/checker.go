// Package moderation declara el puerto compartido de moderación de texto (spec
// 003, RF-12; Art. XII.4). AllowAll permite todo y es la implementación vacía (pruebas); Filter
// (spec 011) es la real y no cambia a quien lo consume.
package moderation

import "context"

// Result es el veredicto sobre un texto. Si Allowed es false, Reason explica
// el motivo en es-MX (con la categoría) y se puede mostrar a la persona;
// Category es la categoría de POL-16 que coincidió ("sexual", "armas",
// "drogas" o "ilegal") y no se muestra tal cual.
type Result struct {
	Allowed  bool
	Reason   string
	Category string
}

// Checker revisa un texto escrito por una persona antes de publicarlo.
type Checker interface {
	Check(ctx context.Context, text string) (Result, error)
}

// AllowAll es la implementación vacía: permite cualquier texto.
type AllowAll struct{}

// Check siempre permite el texto.
func (AllowAll) Check(context.Context, string) (Result, error) {
	return Result{Allowed: true}, nil
}
